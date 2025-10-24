package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config 应用程序配置
type Config struct {
	// 服务器配置
	ServerHost string
	ServerPort string

	// 字节跳动TTS配置
	ByteDanceAppID   string
	ByteDanceToken   string
	ByteDanceCluster string
	ByteDanceAddr    string
	ByteDancePath    string

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

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	cfg := &Config{
		// 服务器配置
		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnv("SERVER_PORT", "8080"),

		// 字节跳动TTS配置
		ByteDanceAppID:   getEnv("BYTE_DANCE_APPID", "XXX"),
		ByteDanceToken:   getEnv("BYTE_DANCE_TOKEN", "XXX"),
		ByteDanceCluster: getEnv("BYTE_DANCE_CLUSTER", "xxxx"),
		ByteDanceAddr:    getEnv("BYTE_DANCE_ADDR", "openspeech.bytedance.com"),
		ByteDancePath:    getEnv("BYTE_DANCE_PATH", "/api/v1/tts/ws_binary"),

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
	// 验证必要的字节跳动配置
	if c.ByteDanceAppID == "XXX" {
		return fmt.Errorf("BYTE_DANCE_APPID is not configured, please set the environment variable")
	}

	if c.ByteDanceToken == "XXX" {
		return fmt.Errorf("BYTE_DANCE_TOKEN is not configured, please set the environment variable")
	}

	if c.ByteDanceCluster == "xxxx" {
		return fmt.Errorf("BYTE_DANCE_CLUSTER is not configured, please set the environment variable")
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