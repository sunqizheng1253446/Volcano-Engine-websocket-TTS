package main

import (
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// Metrics 存储系统监控指标
type Metrics struct {
	mu                sync.RWMutex
	startTime         time.Time
	requestCount      int
	successCount      int
	errorCount        int
	activeConnections int
	currentCalls      int
	totalResponseTime int64
	errors            []ErrorRecord
}

// ErrorRecord 错误记录
type ErrorRecord struct {
	Timestamp string `json:"timestamp"`
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
}

// GlobalMetrics 全局监控实例
var GlobalMetrics *Metrics

// 初始化监控模块
// 注意：此函数在主程序的 main 函数中被调用
func initMetrics() {
	GlobalMetrics = &Metrics{
		startTime: time.Now(),
		errors:    make([]ErrorRecord, 0),
	}
}

// RecordRequest 记录请求
func (m *Metrics) RecordRequest(success bool, responseTimeMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestCount++
	m.totalResponseTime += responseTimeMs
	if success {
		m.successCount++
	} else {
		m.errorCount++
	}
}

// RecordError 记录错误
func (m *Metrics) RecordError(errorType, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record := ErrorRecord{
		Timestamp: time.Now().Format(time.RFC3339),
		ErrorType: errorType,
		Message:   message,
	}
	m.errors = append(m.errors, record)

	// 限制错误记录数量
	if len(m.errors) > 100 {
		m.errors = m.errors[len(m.errors)-100:]
	}
}

// IncActiveConnections 增加活动连接数
func (m *Metrics) IncActiveConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeConnections++
}

// DecActiveConnections 减少活动连接数
func (m *Metrics) DecActiveConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeConnections > 0 {
		m.activeConnections--
	}
}

// IncCurrentCalls 增加当前并发调用数
func (m *Metrics) IncCurrentCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentCalls++
}

// DecCurrentCalls 减少当前并发调用数
func (m *Metrics) DecCurrentCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentCalls > 0 {
		m.currentCalls--
	}
}

// GetUptime 获取运行时间（秒）
func (m *Metrics) GetUptime() int64 {
	return int64(time.Since(m.startTime).Seconds())
}

// GetSuccessRate 获取成功率
func (m *Metrics) GetSuccessRate() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.requestCount == 0 {
		return 100.0
	}
	return float64(m.successCount) / float64(m.requestCount) * 100
}

// GetAvgResponseTime 获取平均响应时间（毫秒）
func (m *Metrics) GetAvgResponseTime() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.requestCount == 0 {
		return 0
	}
	return int(m.totalResponseTime / int64(m.requestCount))
}

// GetErrorCount 获取错误数量
func (m *Metrics) GetErrorCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.errorCount
}

// GetActiveConnections 获取活动连接数
func (m *Metrics) GetActiveConnections() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeConnections
}

// GetCurrentCalls 获取当前并发调用数
func (m *Metrics) GetCurrentCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentCalls
}

// GetRequestCount 获取总请求数
func (m *Metrics) GetRequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requestCount
}

// GetErrorRecords 获取错误记录
func (m *Metrics) GetErrorRecords() []ErrorRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 返回副本以避免并发问题
	records := make([]ErrorRecord, len(m.errors))
	copy(records, m.errors)
	return records
}

// GetCPUsage 获取CPU使用率（实际值）
func GetCPUsage() float64 {
	// 获取CPU使用率，间隔时间为0表示立即返回
	cpuPercent, err := cpu.Percent(0, false)
	if err != nil || len(cpuPercent) == 0 {
		// 如果获取失败，返回默认值
		return 0.0
	}
	return cpuPercent[0]
}

// GetMemoryUsage 获取内存使用率（实际值）
func GetMemoryUsage() float64 {
	// 获取虚拟内存统计信息
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		// 如果获取失败，使用runtime包获取的值并转换为MB
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return float64(m.Alloc) / 1024 / 1024 // 转换为MB
	}

	// 返回内存使用率百分比
	return vmStat.UsedPercent
}
