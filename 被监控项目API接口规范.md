# 被监控项目API接口规范

本文档定义了被监控项目需要提供的API接口规范，以便健康监控系统能够正确获取服务状态信息。

## 1. 健康检查接口

### 接口地址
```
GET /api/health
```

### 请求头
```
Accept: application/json
```

### 响应格式
```json
{
  "status": "ok",
  "uptime_seconds": 3600,
  "active_connections": 10,
  "current_calls": 5,
  "avg_response_time": 120,
  "success_rate": 99.5,
  "error_count": 2,
  "cpu_usage": 45.5,
  "memory_usage": 60.2,
  "request_count": 12345,
  "version": "1.2.3",
  "last_check_time": "2025-10-29T10:30:00Z"
}
```

### 响应字段说明

| 字段名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| status | string | 是 | 服务状态，可选值：`ok`（正常）、`warning`（警告）、`error`（错误） |
| uptime_seconds | int64 | 是 | 服务运行时间（秒） |
| active_connections | int | 是 | 当前活动连接数 |
| current_calls | int | 是 | 当前并发调用数 |
| avg_response_time | int | 是 | 平均响应时间（毫秒） |
| success_rate | float64 | 是 | 请求成功率（百分比，0-100） |
| error_count | int | 是 | 错误数量 |
| cpu_usage | float64 | 是 | CPU使用率（百分比，0-100） |
| memory_usage | float64 | 是 | 内存使用率（百分比，0-100） |
| request_count | int | 是 | 总请求数 |
| version | string | 是 | 服务版本号 |
| last_check_time | string | 是 | 最后检查时间（ISO 8601格式） |

### 状态码
- `200` - 成功返回健康数据
- `503` - 服务不可用
- `500` - 服务器内部错误

## 2. 错误信息接口（可选）

### 接口地址
```
GET /api/errors
```

### 请求头
```
Accept: application/json
```

### 响应格式
```json
{
  "error_records": [
    {
      "timestamp": "2025-10-29T10:25:00Z",
      "error_type": "database_connection",
      "message": "Failed to connect to database"
    }
  ],
  "count": 1
}
```

### 错误记录字段说明

| 字段名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| timestamp | string | 是 | 错误发生时间（ISO 8601格式） |
| error_type | string | 是 | 错误类型 |
| message | string | 是 | 错误详细信息 |

## 3. 性能指标接口（可选）

### 接口地址
```
GET /api/metrics
```

### 请求头
```
Accept: application/json
```

### 响应格式
```json
{
  "avg_response_time": 120,
  "success_rate": 99.5,
  "uptime_seconds": 3600,
  "active_connections": 10,
  "current_calls": 5,
  "cpu_usage": 45.5,
  "memory_usage": 60.2,
  "request_count": 12345
}
```

## 4. WebSocket接口（可选）

### 接口地址
```
GET /api/websocket
```

### 说明
如果被监控服务支持WebSocket连接，可以通过此接口提供实时数据推送功能。

## 5. 认证方式

### 无认证（默认）
健康监控系统默认不使用认证访问被监控服务的API。

### API密钥认证（可选）
如果需要认证，可以在环境变量中配置API密钥：
```
TARGET_API_TOKEN=your_api_token_here
```

请求头需要添加：
```
Authorization: Bearer your_api_token_here
```

## 6. 错误处理

被监控服务应该：
1. 在服务正常运行时返回`200`状态码和健康数据
2. 在服务不可用时返回`503`状态码
3. 在服务器内部错误时返回`500`状态码
4. 在API不存在时返回`404`状态码

## 7. 实现建议

1. 健康检查接口应该轻量级，避免执行耗时操作
2. 接口响应时间应控制在1秒以内
3. 建议对接口进行缓存，避免频繁计算
4. 错误记录应限制数量，避免返回过多数据