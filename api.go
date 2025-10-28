package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WebSocket升级器配置
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 允许所有来源的WebSocket连接
		return true
	},
}

// HealthResponse 健康检查响应结构
type HealthResponse struct {
	Status            string  `json:"status"`
	UptimeSeconds     int64   `json:"uptime_seconds"`
	ActiveConnections int     `json:"active_connections"`
	CurrentCalls      int     `json:"current_calls"`
	AvgResponseTime   int     `json:"avg_response_time"`
	SuccessRate       float64 `json:"success_rate"`
	ErrorCount        int     `json:"error_count"`
	CPUUsage          float64 `json:"cpu_usage"`
	MemoryUsage       float64 `json:"memory_usage"`
	RequestCount      int     `json:"request_count"`
	Version           string  `json:"version"`
	LastCheckTime     string  `json:"last_check_time"`
}

// ErrorsResponse 错误信息响应结构
type ErrorsResponse struct {
	ErrorRecords []ErrorRecord `json:"error_records"`
	Count        int           `json:"count"`
}

// MetricsResponse 性能指标响应结构
type MetricsResponse struct {
	AvgResponseTime   int     `json:"avg_response_time"`
	SuccessRate       float64 `json:"success_rate"`
	UptimeSeconds     int64   `json:"uptime_seconds"`
	ActiveConnections int     `json:"active_connections"`
	CurrentCalls      int     `json:"current_calls"`
	CPUUsage          float64 `json:"cpu_usage"`
	MemoryUsage       float64 `json:"memory_usage"`
	RequestCount      int     `json:"request_count"`
}

// handleHealthCheck 处理健康检查请求
func handleHealthCheck(c *gin.Context) {
	// 验证API密钥（如果配置了）
	if !validateAPIKey(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 构建健康检查响应
	response := HealthResponse{
		Status:            "ok",
		UptimeSeconds:     GlobalMetrics.GetUptime(),
		ActiveConnections: GlobalMetrics.GetActiveConnections(),
		CurrentCalls:      GlobalMetrics.GetCurrentCalls(),
		AvgResponseTime:   GlobalMetrics.GetAvgResponseTime(),
		SuccessRate:       GlobalMetrics.GetSuccessRate(),
		ErrorCount:        GlobalMetrics.GetErrorCount(),
		CPUUsage:          GetCPUsage(),
		MemoryUsage:       GetMemoryUsage(),
		RequestCount:      GlobalMetrics.GetRequestCount(),
		Version:           "1.0.0",
		LastCheckTime:     time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}

// handleErrors 处理错误信息请求
func handleErrors(c *gin.Context) {
	// 验证API密钥（如果配置了）
	if !validateAPIKey(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 获取错误记录
	errorRecords := GlobalMetrics.GetErrorRecords()

	// 构建错误信息响应
	response := ErrorsResponse{
		ErrorRecords: errorRecords,
		Count:        len(errorRecords),
	}

	c.JSON(http.StatusOK, response)
}

// handleMetrics 处理性能指标请求
func handleMetrics(c *gin.Context) {
	// 验证API密钥（如果配置了）
	if !validateAPIKey(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 构建性能指标响应
	response := MetricsResponse{
		AvgResponseTime:   GlobalMetrics.GetAvgResponseTime(),
		SuccessRate:       GlobalMetrics.GetSuccessRate(),
		UptimeSeconds:     GlobalMetrics.GetUptime(),
		ActiveConnections: GlobalMetrics.GetActiveConnections(),
		CurrentCalls:      GlobalMetrics.GetCurrentCalls(),
		CPUUsage:          GetCPUsage(),
		MemoryUsage:       GetMemoryUsage(),
		RequestCount:      GlobalMetrics.GetRequestCount(),
	}

	c.JSON(http.StatusOK, response)
}

// handleWebSocket 处理WebSocket连接请求
func handleWebSocket(c *gin.Context) {
	// 验证API密钥（如果配置了）
	if !validateAPIKey(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// 升级HTTP连接为WebSocket连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		GlobalMetrics.RecordError("websocket", "Failed to upgrade connection: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to establish websocket connection"})
		return
	}
	defer conn.Close()

	// 增加活动连接计数
	GlobalMetrics.IncActiveConnections()
	defer GlobalMetrics.DecActiveConnections()

	// WebSocket连接建立后的处理
	// 示例：定期推送健康状态更新
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// 创建退出通道
	quit := make(chan struct{})
	defer close(quit)

	// 在单独的goroutine中处理接收消息
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				// 连接已关闭或发生错误
				close(quit)
				break
			}
			// 这里可以处理客户端发送的消息
		}
	}()

	// 主循环处理发送消息
	for {
		select {
		case <-ticker.C:
			// 构建实时数据
			liveData := gin.H{
				"timestamp":          time.Now().Format(time.RFC3339),
				"active_connections": GlobalMetrics.GetActiveConnections(),
				"current_calls":      GlobalMetrics.GetCurrentCalls(),
				"cpu_usage":          GetCPUsage(),
				"memory_usage":       GetMemoryUsage(),
			}

			// 发送数据
			err := conn.WriteJSON(liveData)
			if err != nil {
				// 连接已关闭
				return
			}
		case <-quit:
			// 客户端断开连接
			return
		}
	}
}

// validateAPIKey 验证API密钥
func validateAPIKey(c *gin.Context) bool {
	// 从环境变量获取API密钥
	apiToken := getEnv("TARGET_API_TOKEN", "")
	if apiToken == "" {
		// 未配置API密钥，允许所有请求
		return true
	}

	// 获取请求头中的Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return false
	}

	// 检查格式并验证
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:] == apiToken
	}

	return false
}

// setupMonitoringRoutes 设置监控相关路由
func setupMonitoringRoutes(router *gin.Engine) {
	// 监控API路由组
	monitoring := router.Group("/monitoring")
	{
		monitoring.GET("/health", handleHealthCheck)
		monitoring.GET("/ws", handleWebSocket)
	}

	// API路由组（符合规范要求）
	api := router.Group("/api")
	{
		api.GET("/health", handleHealthCheck)
		api.GET("/errors", handleErrors)
		api.GET("/metrics", handleMetrics)
	}

	// 保留原有健康检查端点的兼容性
	router.GET("/health", handleHealthCheck)
}
