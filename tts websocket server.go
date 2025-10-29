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
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
)

// Config 应用程序配置
type Config struct {
	// 服务器配置
	ServerHost string
	ServerPort string

	// 字节跳动TTS配置
	ByteDanceAppID     string
	ByteDanceToken     string
	ByteDanceCluster   string
	ByteDanceVoiceType string

	// OpenAI TTS认证配置
	OpenAITTSAPIKey string

	// 超时配置
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// 日志配置
	LogLevel string

	// 性能配置
	MaxConnections     int
	MaxRequestSizeMB   int
	MaxTextLength      int
	MaxConcurrentCalls int
}

// 应用程序配置
var appConfig *Config

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	cfg := &Config{
		// 服务器配置
		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnv("SERVER_PORT", "8080"),

		// 字节跳动TTS配置
		ByteDanceAppID:     getEnv("BYTEDANCE_TTS_APP_ID", "XXX"),
		ByteDanceToken:     getEnv("BYTEDANCE_TTS_BEARER_TOKEN", "XXX"),
		ByteDanceCluster:   getEnv("BYTEDANCE_TTS_CLUSTER", "xxxx"),
		ByteDanceVoiceType: getEnv("BYTEDANCE_TTS_VOICE_TYPE", ""),

		// OpenAI TTS认证配置
		OpenAITTSAPIKey: getEnv("OPENAI_TTS_API_KEY", ""),

		// 超时配置
		DialTimeout:  getEnvDuration("DIAL_TIMEOUT", 10*time.Second),
		ReadTimeout:  getEnvDuration("READ_TIMEOUT", 30*time.Second),
		WriteTimeout: getEnvDuration("WRITE_TIMEOUT", 30*time.Second),

		// 日志配置
		LogLevel: getEnv("LOG_LEVEL", "info"),

		// 性能配置
		MaxConnections:     getEnvInt("MAX_CONNECTIONS", 100),
		MaxRequestSizeMB:   getEnvInt("MAX_REQUEST_SIZE_MB", 5),
		MaxTextLength:      getEnvInt("MAX_TEXT_LENGTH", 5000),
		MaxConcurrentCalls: getEnvInt("MAX_CONCURRENT_CALLS", 10),
	}

	return cfg
}

// ValidateConfig 验证配置的有效性
func (c *Config) ValidateConfig() error {
	// 验证必要的字节跳动配置，收集所有缺失的环境变量
	var missingEnvs []string

	if c.ByteDanceAppID == "XXX" {
		missingEnvs = append(missingEnvs, "BYTEDANCE_TTS_APP_ID")
	}

	if c.ByteDanceToken == "XXX" {
		missingEnvs = append(missingEnvs, "BYTEDANCE_TTS_BEARER_TOKEN")
	}

	if c.ByteDanceCluster == "xxxx" {
		missingEnvs = append(missingEnvs, "BYTEDANCE_TTS_CLUSTER")
	}

	if c.ByteDanceVoiceType == "" {
		missingEnvs = append(missingEnvs, "BYTEDANCE_TTS_VOICE_TYPE")
	}

	// 如果有缺失的环境变量，返回详细的错误信息
	if len(missingEnvs) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missingEnvs)
	}

	// 验证超时设置
	if c.DialTimeout <= 0 {
		return fmt.Errorf("DIAL_TIMEOUT must be positive")
	}

	if c.ReadTimeout <= 0 {
		return fmt.Errorf("READ_TIMEOUT must be positive")
	}

	if c.WriteTimeout <= 0 {
		return fmt.Errorf("WRITE_TIMEOUT must be positive")
	}

	// 验证性能限制
	if c.MaxConnections <= 0 {
		return fmt.Errorf("MAX_CONNECTIONS must be positive")
	}

	if c.MaxRequestSizeMB <= 0 {
		return fmt.Errorf("MAX_REQUEST_SIZE_MB must be positive")
	}

	if c.MaxTextLength <= 0 {
		return fmt.Errorf("MAX_TEXT_LENGTH must be positive")
	}

	if c.MaxConcurrentCalls <= 0 {
		return fmt.Errorf("MAX_CONCURRENT_CALLS must be positive")
	}

	return nil
}

// 从环境变量获取字符串值，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// 从环境变量获取整数值，如果不存在或解析失败则返回默认值
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// 从环境变量获取时间段值，如果不存在或解析失败则返回默认值
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

var byteDanceURL *url.URL
var semaphore chan struct{} // 用于控制并发调用数量
var startTime time.Time     // 服务启动时间

// 协议相关常量
const (
	optSubmit string = "submit"
)

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
	ErrInvalidAPIKey       = errors.New("invalid API key format")
	ErrUnauthorized        = errors.New("unauthorized access")
)

// isValidAPIKey 验证API密钥格式是否合法
// 确保密钥不包含非法符号如括号等
func isValidAPIKey(key string) bool {
	// 检查密钥是否为空
	if key == "" {
		return false
	}

	// 检查是否包含非法字符（括号等）
	illegalChars := "[](){}<>()[]{}<>&^%$#@!~`\"'"
	for _, char := range key {
		for _, illegal := range illegalChars {
			if char == illegal {
				return false
			}
		}
	}

	return true
}

// 默认协议头部
var defaultHeader = []byte{0x11, 0x10, 0x11, 0x00}

// OpenAI TTS请求结构
type OpenAITTSRequest struct {
	Model          string  `json:"model" binding:"required"`
	Input          string  `json:"input" binding:"required"`
	Voice          string  `json:"voice" binding:"required"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
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
	appConfig = LoadConfig()

	// 初始化字节跳动URL - 直接硬编码完整URL
	byteDanceURL = &url.URL{
		Scheme: "wss",
		Host:   "openspeech.bytedance.com",
		Path:   "/api/v1/tts/ws_binary",
	}

	// 初始化并发控制信号量
	semaphore = make(chan struct{}, appConfig.MaxConcurrentCalls)
}

// 设置字节跳动TTS请求参数
// 注意：voice_type 参数来自环境变量 BYTEDANCE_TTS_VOICE_TYPE，忽略任何传入的 voiceType 值
func setupByteDanceInput(text, opt string, speed float64) ([]byte, error) {
	// 验证文本长度
	if len(text) > appConfig.MaxTextLength {
		return nil, fmt.Errorf("%w: text length %d exceeds maximum allowed %d",
			ErrTextTooLong, len(text), appConfig.MaxTextLength)
	}

	// 直接使用appConfig中的配置值
	// voice_type 参数来自环境变量 BYTEDANCE_TTS_VOICE_TYPE
	appID := appConfig.ByteDanceAppID
	token := appConfig.ByteDanceToken
	cluster := appConfig.ByteDanceCluster
	voiceType := appConfig.ByteDanceVoiceType

	reqID := uuid.NewV4().String()
	params := make(map[string]map[string]interface{})
	params["app"] = make(map[string]interface{})
	params["app"]["appid"] = appID
	params["app"]["token"] = token
	params["app"]["cluster"] = cluster
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
// 注意：voiceType 参数被忽略，实际使用的 voice_type 来自环境变量 BYTEDANCE_TTS_VOICE_TYPE
func streamSynthesize(text, voiceType string, speed float64) ([]byte, error) {
	// 明确忽略 voiceType 参数以消除静态分析警告
	_ = voiceType

	// 获取并发控制信号量
	select {
	case semaphore <- struct{}{}:
		// 增加当前并发调用计数
		GlobalMetrics.IncCurrentCalls()
		defer func() {
			<-semaphore
			// 减少当前并发调用计数
			GlobalMetrics.DecCurrentCalls()
		}()
	default:
		return nil, fmt.Errorf("%w: maximum concurrent calls (%d) reached",
			ErrTooManyConnections, appConfig.MaxConcurrentCalls)
	}

	// 设置输入参数
	// 注意：voiceType 参数被忽略，实际使用的 voice_type 来自环境变量 BYTEDANCE_TTS_VOICE_TYPE
	input, err := setupByteDanceInput(text, optSubmit, speed)
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
// 注意：此函数的返回值在当前实现中被忽略，仅用于保持接口兼容性
func mapOpenAIVoiceToByteDance(openAIVoice string) string {
	// 这里可以根据实际情况添加映射关系
	// 注意：实际使用的 voice_type 来自环境变量 BYTEDANCE_TTS_VOICE_TYPE
	voiceMap := map[string]string{
		"alloy":   "alloy", // 需要根据实际的字节跳动语音ID进行映射
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
	GlobalMetrics.IncActiveConnections()
	defer GlobalMetrics.DecActiveConnections()

	// 验证并发连接数
	currentConnections := GlobalMetrics.GetActiveConnections()
	if currentConnections > appConfig.MaxConnections {
		// 记录请求（失败）
		responseTime := time.Since(startTime).Milliseconds()
		GlobalMetrics.RecordRequest(false, responseTime)
		GlobalMetrics.RecordError("connection_limit", fmt.Sprintf("Too many concurrent connections, maximum is %d", appConfig.MaxConnections))

		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error:   "service_overloaded",
			Code:    http.StatusServiceUnavailable,
			Message: fmt.Sprintf("Too many concurrent connections, maximum is %d", appConfig.MaxConnections),
		})
		return
	}

	// API密钥验证
	apiKey := c.GetHeader("Authorization")
	// 移除可能的Bearer前缀
	if len(apiKey) > 7 && apiKey[:7] == "Bearer " {
		apiKey = apiKey[7:]
	}

	// 验证密钥格式
	if !isValidAPIKey(apiKey) {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "invalid_api_key",
			Code:    http.StatusUnauthorized,
			Message: "API key format is invalid, must not contain illegal characters",
		})
		return
	}

	// 如果服务器配置了API密钥，则验证客户端密钥是否匹配
	if appConfig.OpenAITTSAPIKey != "" {
		if apiKey != appConfig.OpenAITTSAPIKey {
			c.JSON(http.StatusUnauthorized, ErrorResponse{
				Error:   "unauthorized",
				Code:    http.StatusUnauthorized,
				Message: "Invalid API key",
			})
			return
		}
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

	// 映射语音类型（注意：此参数在实际调用中被忽略，仅用于保持接口兼容性）
	// 实际使用的 voice_type 来自环境变量 BYTEDANCE_TTS_VOICE_TYPE
	byteDanceVoice := mapOpenAIVoiceToByteDance(req.Voice)

	// 设置响应头
	c.Header("Content-Type", "audio/mpeg")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Content-Type-Options", "nosniff")

	// 创建流式合成并返回数据
	// 注意：byteDanceVoice 参数在 streamSynthesize 中被忽略
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
		case errors.Is(err, ErrInvalidAPIKey):
			statusCode = http.StatusUnauthorized
			errorType = "invalid_api_key"
		case errors.Is(err, ErrUnauthorized):
			statusCode = http.StatusUnauthorized
			errorType = "unauthorized"
		}

		c.JSON(statusCode, ErrorResponse{
			Error:   errorType,
			Code:    statusCode,
			Message: err.Error(),
		})
		// 记录请求（失败）
		responseTime := time.Since(startTime).Milliseconds()
		GlobalMetrics.RecordRequest(false, responseTime)
		GlobalMetrics.RecordError(errorType, err.Error())
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

	// 记录请求（成功）
	responseTime := time.Since(startTime).Milliseconds()
	GlobalMetrics.RecordRequest(true, responseTime)
}

// 移除旧的健康检查函数，使用监控模块中的实现

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

	// 保持原有的健康检查路由兼容性，但实际使用监控模块中的实现
	router.GET("/health", handleHealthCheck)

	// OpenAI TTS API兼容端点
	router.POST("/v1/audio/speech", handleOpenAITTSRequest)
}

// 主函数
func main() {
	// 初始化监控模块
	initMetrics()

	// 验证配置
	err := appConfig.ValidateConfig()
	if err != nil {
		fmt.Printf("Configuration validation failed: %v\n", err)
		fmt.Println("Please set the missing environment variables before starting the service.")
		fmt.Println("Example usage:")
		fmt.Println("  BYTEDANCE_TTS_APP_ID=your_app_id BYTEDANCE_TTS_BEARER_TOKEN=your_token BYTEDANCE_TTS_CLUSTER=your_cluster BYTEDANCE_TTS_VOICE_TYPE=your_voice_type ./tts_websocket_server")

		// 在容器环境中，为了避免无限重启循环，我们启动一个最小化的HTTP服务器来提供错误信息
		fmt.Println("Starting minimal HTTP server to provide error information...")
		startMinimalServer()
		return
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

	// 设置监控相关路由
	setupMonitoringRoutes(router)

	// 启动服务器
	serverAddr := fmt.Sprintf("%s:%s", appConfig.ServerHost, appConfig.ServerPort)
	fmt.Printf("Starting TTS Transit Service on %s\n", serverAddr)
	fmt.Printf("Health check: http://%s/health\n", serverAddr)
	fmt.Printf("TTS endpoint: http://%s/v1/audio/speech\n", serverAddr)
	fmt.Printf("Monitoring endpoints:\n")
	fmt.Printf("  - Health check: http://%s/api/health\n", serverAddr)
	fmt.Printf("  - Metrics: http://%s/api/metrics\n", serverAddr)
	fmt.Printf("  - Errors: http://%s/api/errors\n", serverAddr)
	fmt.Printf("  - WebSocket monitoring: ws://%s/api/ws\n", serverAddr)
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

// startMinimalServer 启动一个最小化的HTTP服务器来提供错误信息，避免容器无限重启
func startMinimalServer() {
	router := gin.New()

	// 健康检查端点，返回配置错误信息
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"message": "Service is not properly configured",
			"details": "Missing required environment variables. Check logs for details.",
		})
	})

	// 根API端点也返回错误信息
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"message": "Service is not properly configured",
			"details": "Missing required environment variables. Check logs for details.",
		})
	})

	// TTS端点也返回错误信息
	router.POST("/v1/audio/speech", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"message": "Service is not properly configured",
			"details": "Missing required environment variables. Check logs for details.",
		})
	})

	serverAddr := fmt.Sprintf("%s:%s", appConfig.ServerHost, appConfig.ServerPort)
	fmt.Printf("Starting minimal HTTP server on %s to report configuration error\n", serverAddr)

	// 启动服务器但不处理错误，因为这是在错误状态下运行的
	_ = router.Run(serverAddr)
}
