package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"time"

	"Volcano-Engine-websocket-TTS/config"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
)

// 应用程序配置
var appConfig *config.Config
var byteDanceURL *url.URL
var activeConnections atomic.Int32 // 使用原子计数器替代WaitGroup
var semaphore chan struct{} // 用于控制并发调用数量
var startTime time.Time // 服务启动时间

// 协议相关常量
const (
	optQuery  string = "query"
	optSubmit string = "submit"
)

// 默认协议头部
var defaultHeader = []byte{0x11, 0x10, 0x11, 0x00}

// 错误定义
var (
	ErrInvalidRequest      = errors.New("invalid request format")
	ErrTextTooLong         = errors.New("text too long")
	ErrTooManyConnections  = errors.New("too many concurrent connections")
	ErrWebSocketDialFailed = errors.New("websocket dial failed")
	ErrMessageWriteFailed  = errors.New("failed to write message")
	ErrMessageReadFailed   = errors.New("failed to read message")
	ErrResponseParseFailed = errors.New("failed to parse response")
	ErrAudioWriteFailed    = errors.New("failed to write audio data")
)

// 协议消息类型映射
var enumMessageType = map[byte]string{
	11: "audio-only server response",
	12: "frontend server response",
	15: "error message from server",
}

var enumMessageTypeSpecificFlags = map[byte]string{
	0: "no sequence number",
	1: "sequence number > 0",
	2: "last message from server (seq < 0)",
	3: "sequence number < 0",
}

// OpenAI TTS请求结构
type OpenAITTSRequest struct {
	Model         string  `json:"model" binding:"required"`
	Input         string  `json:"input" binding:"required"`
	Voice         string  `json:"voice" binding:"required"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed         float64 `json:"speed,omitempty"`
}

// 合成响应结构
type SynthResp struct {
	Audio  []byte
	IsLast bool
}

// 初始化函数
func init() {
	// 记录服务启动时间
	startTime = time.Now()
	
	// 初始化应用配置
	appConfig = config.LoadConfig()
	
	// 初始化字节跳动URL
	byteDanceURL = &url.URL{
		Scheme: "wss",
		Host:   appConfig.ByteDanceAddr,
		Path:   appConfig.ByteDancePath,
	}
	
	// 初始化并发控制信号量
	semaphore = make(chan struct{}, appConfig.MaxConcurrentCalls)
}

// 设置字节跳动TTS请求参数
func setupByteDanceInput(text, voiceType, opt string, speed float64) ([]byte, error) {
	// 验证文本长度
	if len(text) > appConfig.MaxTextLength {
		return nil, fmt.Errorf("%w: text length %d exceeds maximum allowed %d", 
			ErrTextTooLong, len(text), appConfig.MaxTextLength)
	}

	reqID := uuid.NewV4().String()
	params := make(map[string]map[string]interface{})
	params["app"] = make(map[string]interface{})
	params["app"]["appid"] = appConfig.ByteDanceAppID
	params["app"]["token"] = "access_token"
	params["app"]["cluster"] = appConfig.ByteDanceCluster
	params["user"] = make(map[string]interface{})
	params["user"]["uid"] = "uid"
	params["audio"] = make(map[string]interface{})
	params["audio"]["voice_type"] = voiceType
	params["audio"]["encoding"] = "mp3"
	params["audio"]["speed_ratio"] = speed
	params["audio"]["volume_ratio"] = 1.0
	params["audio"]["pitch_ratio"] = 1.0
	params["request"] = make(map[string]interface{})
	params["request"]["reqid"] = reqID
	params["request"]["text"] = text
	params["request"]["text_type"] = "plain"
	params["request"]["operation"] = opt
	
	resStr, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal TTS parameters: %w", err)
	}
	
	return resStr, nil
}

// GZIP压缩
func gzipCompress(input []byte) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(input)
	w.Close()
	return b.Bytes()
}

// GZIP解压缩
func gzipDecompress(input []byte) []byte {
	b := bytes.NewBuffer(input)
	r, err := gzip.NewReader(b)
	if err != nil {
		fmt.Printf("gzip decompress error: %v\n", err)
		return input
	}
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		fmt.Printf("read decompressed data error: %v\n", err)
		return input
	}
	return out
}

// 解析字节跳动TTS响应
func parseByteDanceResponse(res []byte) (resp SynthResp, err error) {
	if len(res) < 4 {
		return resp, errors.New("invalid response: too short")
	}

	// protoVersion := res[0] >> 4
	headSize := res[0] & 0x0f
	messageType := res[1] >> 4
	messageTypeSpecificFlags := res[1] & 0x0f
	// serializationMethod := res[2] >> 4
	messageCompression := res[2] & 0x0f
	// reserve := res[3]

	// 确保有足够的头部数据
	if int(headSize*4) > len(res) {
		return resp, errors.New("invalid response: header size exceeds data length")
	}

	// 提取负载数据
	payload := res[headSize*4:]

	// 处理不同类型的消息
	switch messageType {
	case 0xb: // audio-only server response
		// 无序列号作为确认
		if messageTypeSpecificFlags == 0 {
			// 空负载
		} else {
			// 有序列号的音频数据
			if len(payload) < 8 {
				return resp, errors.New("invalid audio response: insufficient data")
			}

			sequenceNumber := int32(binary.BigEndian.Uint32(payload[0:4]))
			// payloadSize := int32(binary.BigEndian.Uint32(payload[4:8]))
			audioData := payload[8:]

			resp.Audio = append(resp.Audio, audioData...)

			// 检查是否为最后一条消息
			if sequenceNumber < 0 {
				resp.IsLast = true
			}
		}

	case 0xf: // error message
		if len(payload) < 8 {
			return resp, errors.New("invalid error response: insufficient data")
		}

		code := int32(binary.BigEndian.Uint32(payload[0:4]))
		errorData := payload[8:]

		// 如果是压缩的错误信息，进行解压缩
		if messageCompression == 1 {
			errorData = gzipDecompress(errorData)
		}

		errorMsg := string(errorData)
		err = fmt.Errorf("server error (code: %d): %s", code, errorMsg)

	case 0xc: // frontend message
		if len(payload) < 4 {
			return resp, errors.New("invalid frontend message: insufficient data")
		}

		// 直接跳过msgSize
		frontendPayload := payload[4:]

		// 如果是压缩的前端消息，进行解压缩
		if messageCompression == 1 {
			frontendPayload = gzipDecompress(frontendPayload)
		}

		fmt.Printf("Frontend message: %s\n", string(frontendPayload))

	default:
		err = fmt.Errorf("unknown message type: %d", messageType)
	}

	return resp, err
}

// 实现流式合成并返回音频数据
func streamSynthesize(text, voiceType string, speed float64) ([]byte, error) {
	// 获取并发控制信号量
	select {
	case semaphore <- struct{}{}:
		defer func() { <-semaphore }()
	default:
		return nil, fmt.Errorf("%w: maximum concurrent calls (%d) reached", 
			ErrTooManyConnections, appConfig.MaxConcurrentCalls)
	}

	// 设置输入参数
	input, err := setupByteDanceInput(text, voiceType, optSubmit, speed)
	if err != nil {
		return nil, err
	}

	input = gzipCompress(input)

	// 构建请求
	payloadSize := len(input)
	payloadArr := make([]byte, 4)
	binary.BigEndian.PutUint32(payloadArr, uint32(payloadSize))
	clientRequest := make([]byte, len(defaultHeader))
	copy(clientRequest, defaultHeader)
	clientRequest = append(clientRequest, payloadArr...)
	clientRequest = append(clientRequest, input...)

	// 创建WebSocket连接配置
	dialer := websocket.Dialer{
		HandshakeTimeout: appConfig.DialTimeout,
		ReadBufferSize:   1024 * 1024, // 1MB
		WriteBufferSize:  1024 * 1024, // 1MB
	}

	// 创建WebSocket连接
	header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer;%s", appConfig.ByteDanceToken)}}
	c, _, err := dialer.Dial(byteDanceURL.String(), header)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWebSocketDialFailed, err)
	}

	// 设置连接超时
	c.SetReadDeadline(time.Now().Add(appConfig.ReadTimeout))
	c.SetWriteDeadline(time.Now().Add(appConfig.WriteTimeout))

	defer c.Close()

	// 发送请求
	err = c.WriteMessage(websocket.BinaryMessage, clientRequest)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMessageWriteFailed, err)
	}

	// 接收音频数据
	var audio []byte
	for {
		// 更新读取超时
		c.SetReadDeadline(time.Now().Add(appConfig.ReadTimeout))

		_, message, err := c.ReadMessage()
		if err != nil {
			// 如果是连接关闭错误且已收到一些音频数据，仍然返回已接收的音频
			if len(audio) > 0 && websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("Warning: connection closed with partial audio received: %v\n", err)
				return audio, nil
			}
			return nil, fmt.Errorf("%w: %v", ErrMessageReadFailed, err)
		}

		resp, err := parseByteDanceResponse(message)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrResponseParseFailed, err)
		}

		// 添加音频数据
		audio = append(audio, resp.Audio...)

		// 检查是否为最后一条消息
		if resp.IsLast {
			break
		}
	}

	return audio, nil
}

// 将OpenAI语音映射到字节跳动语音
func mapOpenAIVoiceToByteDance(openAIVoice string) string {
	// 这里可以根据实际情况添加映射关系
	voiceMap := map[string]string{
		"alloy":   "alloy",     // 需要根据实际的字节跳动语音ID进行映射
		"echo":    "echo",
		"fable":   "fable",
		"onyx":    "onyx",
		"nova":    "nova",
		"shimmer": "shimmer",
	}

	if mappedVoice, exists := voiceMap[openAIVoice]; exists {
		return mappedVoice
	}

	// 默认返回alloy
	return "alloy"
}

// 错误响应结构
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// 处理OpenAI TTS请求的处理函数
func handleOpenAITTSRequest(c *gin.Context) {
	// 增加活动连接计数
	activeConnections.Add(1)
	defer activeConnections.Add(-1)

	// 验证并发连接数
	currentConnections := activeConnections.Load()
	if currentConnections > int32(appConfig.MaxConnections) {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "service_overloaded",
			Code:    http.StatusServiceUnavailable,
			Message: fmt.Sprintf("Too many concurrent connections, maximum is %d", appConfig.MaxConnections),
		})
		return
	}

	// 解析请求体
	var req OpenAITTSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Invalid request format: %v", err),
		})
		return
	}

	// 验证请求参数
	if req.Input == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Code:    http.StatusBadRequest,
			Message: "Input text cannot be empty",
		})
		return
	}

	if req.Voice == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Code:    http.StatusBadRequest,
			Message: "Voice parameter cannot be empty",
		})
		return
	}

	// 设置默认值
	responseFormat := req.ResponseFormat
	if responseFormat == "" {
		responseFormat = "mp3"
	}

	speed := req.Speed
	if speed == 0 {
		speed = 1.0
	}

	// 限制速度范围
	if speed < 0.5 || speed > 2.0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Code:    http.StatusBadRequest,
			Message: "Speed must be between 0.5 and 2.0",
		})
		return
	}

	// 映射语音类型
	byteDanceVoice := mapOpenAIVoiceToByteDance(req.Voice)

	// 设置响应头
	c.Header("Content-Type", "audio/mpeg")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Content-Type-Options", "nosniff")

	// 创建流式合成并返回数据
	audioData, err := streamSynthesize(req.Input, byteDanceVoice, speed)
	if err != nil {
		// 根据错误类型返回适当的HTTP状态码
		statusCode := http.StatusInternalServerError
		errorType := "internal_error"
		
		// 处理已知错误类型
		switch {
		case errors.Is(err, ErrTextTooLong):
			statusCode = http.StatusBadRequest
			errorType = "invalid_request"
		case errors.Is(err, ErrTooManyConnections):
			statusCode = http.StatusServiceUnavailable
			errorType = "service_overloaded"
		case errors.Is(err, ErrWebSocketDialFailed):
			statusCode = http.StatusServiceUnavailable
			errorType = "upstream_service_unavailable"
		}

		c.JSON(statusCode, ErrorResponse{
			Error:   errorType,
			Code:    statusCode,
			Message: err.Error(),
		})
		return
	}

	// 写入音频数据
	_, err = c.Writer.Write(audioData)
	if err != nil {
		fmt.Printf("Error writing audio data: %v\n", err)
		return
	}

	// 刷新缓冲区
	c.Writer.Flush()
}

// 健康检查端点
func healthCheck(c *gin.Context) {
	// 获取当前活动连接数
	activeConns := activeConnections.Load()
	
	// 获取当前并发调用数
	currentCalls := len(semaphore)
	
	c.JSON(http.StatusOK, gin.H{
		"status":             "ok",
		"timestamp":          time.Now().Unix(),
		"timestamp_iso":      time.Now().Format(time.RFC3339),
		"service":            "TTS-Transit-Service",
		"version":            "1.0.0",
		"active_connections": activeConns,
		"max_connections":    appConfig.MaxConnections,
		"current_calls":      currentCalls,
		"max_concurrent_calls": appConfig.MaxConcurrentCalls,
		"uptime_seconds":     int(time.Since(startTime).Seconds()),
	})
}

// 设置路由
func setupRoutes(router *gin.Engine) {
	// 添加请求大小限制中间件
	router.Use(gin.HandlerFunc(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, int64(appConfig.MaxRequestSizeMB)*1024*1024)
		c.Next()
	}))

	// 添加CORS中间件
	router.Use(gin.HandlerFunc(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		
		c.Next()
	}))

	// 健康检查端点
	router.GET("/health", healthCheck)

	// OpenAI TTS API兼容端点
	router.POST("/v1/audio/speech", handleOpenAITTSRequest)
}

// 主函数
func main() {
	// 验证配置
	err := appConfig.ValidateConfig()
	if err != nil {
		fmt.Printf("Configuration validation failed: %v\n", err)
		fmt.Println("Please set the required environment variables before starting the service.")
		fmt.Println("Required environment variables:")
		fmt.Println("  - BYTE_DANCE_APPID: Your ByteDance application ID")
		fmt.Println("  - BYTE_DANCE_TOKEN: Your ByteDance authentication token")
		fmt.Println("  - BYTE_DANCE_CLUSTER: Your ByteDance cluster name")
		os.Exit(1)
	}

	// 设置Gin模式
	if appConfig.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin路由器
	router := gin.New()

	// 添加日志和恢复中间件
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// 设置路由
	setupRoutes(router)

	// 启动服务器
	serverAddr := fmt.Sprintf("%s:%s", appConfig.ServerHost, appConfig.ServerPort)
	fmt.Printf("Starting TTS Transit Service on %s\n", serverAddr)
	fmt.Printf("Health check: http://%s/health\n", serverAddr)
	fmt.Printf("TTS endpoint: http://%s/v1/audio/speech\n", serverAddr)
	fmt.Printf("Configuration:\n")
	fmt.Printf("  - Max Connections: %d\n", appConfig.MaxConnections)
	fmt.Printf("  - Max Concurrent Calls: %d\n", appConfig.MaxConcurrentCalls)
	fmt.Printf("  - Max Text Length: %d characters\n", appConfig.MaxTextLength)
	fmt.Printf("  - Read Timeout: %v\n", appConfig.ReadTimeout)
	fmt.Printf("  - Write Timeout: %v\n", appConfig.WriteTimeout)
	fmt.Printf("  - Dial Timeout: %v\n", appConfig.DialTimeout)

	err = router.Run(serverAddr)
	if err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}
}
