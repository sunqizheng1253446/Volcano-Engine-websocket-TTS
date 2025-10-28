# TTS WebSocket 中转服务

## 项目介绍

这是一个基于 Go 语言开发的 TTS（Text-to-Speech）Websocket 中转服务，用于将 OpenAI 风格的 TTS 请求转换为火山引擎（字节跳动）的 TTS WebSocket API 请求，并将响应流式返回给客户端。

## 功能特性

- 支持 OpenAI 风格的 WebSocket TTS API
- 自动转换为火山引擎 TTS WebSocket 协议
- 流式响应处理
- 并发连接和调用限制
- 多维度监控 API 模块
- WebSocket 实时监控数据流
- 环境变量配置
- 完善的错误处理
- 详细的服务运行状态统计

## 快速开始

### 前提条件

- Go 1.16 或更高版本
- 火山引擎 TTS 服务的访问凭证

### 安装和运行

1. 克隆代码库

```bash
git clone https://github.com/yourusername/Volcano-Engine-websocket-TTS.git
cd Volcano-Engine-websocket-TTS
```

2. 安装依赖

```bash
go mod tidy
```

3. 配置环境变量（参见配置说明）

4. 构建和运行

```bash
go build
# 设置环境变量并运行
BYTEDANCE_TTS_APP_ID=your_app_id BYTEDANCE_TTS_BEARER_TOKEN=your_token BYTEDANCE_TTS_CLUSTER=your_cluster BYTEDANCE_TTS_VOICE_TYPE=your_voice_type go run .
```

或者直接运行构建好的二进制文件：

```bash
go build
BYTEDANCE_TTS_APP_ID=your_app_id BYTEDANCE_TTS_BEARER_TOKEN=your_token BYTEDANCE_TTS_CLUSTER=your_cluster BYTEDANCE_TTS_VOICE_TYPE=your_voice_type ./Volcano-Engine-websocket-TTS
```

## 配置说明

服务通过环境变量进行配置。以下是可用的配置项：

> **设计说明**：OpenAI TTS 请求中的 `voice` 参数在当前版本中被忽略，语音类型完全由环境变量 `BYTEDANCE_TTS_VOICE_TYPE` 决定。这是有意为之的设计决策，以确保服务使用统一的语音配置。

| 环境变量 | 类型 | 默认值 | 描述 |
|---------|------|-------|------|
| `LISTEN_ADDR` | string | `:8080` | 服务监听地址和端口 |
| `BYTEDANCE_TTS_APP_ID` | string | (必需) | 火山引擎 App ID |
| `BYTEDANCE_TTS_BEARER_TOKEN` | string | (必需) | 火山引擎认证令牌 |
| `BYTEDANCE_TTS_CLUSTER` | string | (必需) | 火山引擎集群名称 |
| `BYTEDANCE_TTS_VOICE_TYPE` | string | (必需) | 火山引擎语音类型 |
| `OPENAI_TTS_API_KEY` | string | (可选) | OpenAI TTS API 访问密钥，用于验证客户端请求 |
| `MAX_CONNECTIONS` | int | 100 | 最大并发连接数 |
| `MAX_CONCURRENT_CALLS` | int | 10 | 最大并发调用数 |
| `LOG_LEVEL` | string | `info` | 日志级别（debug, info, warn, error） |
| `GIN_MODE` | string | `release` | Gin 框架模式 |

### 使用 .env 文件

你可以创建一个 `.env` 文件来管理环境变量。示例文件参见 `.env.example`。

## API 使用

### 监控 API 接口

#### 健康检查端点

```
GET /health
```

返回服务状态信息：

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
  "version": "1.0.0",
  "last_check_time": "2025-10-29T10:30:00Z"
}
```

#### 监控模块端点

```
GET /monitoring/health
```

与 `/health` 端点功能相同，提供服务健康状态。

#### WebSocket 实时监控

```
GET /monitoring/ws
```

WebSocket 连接建立后，服务会每 10 秒推送一次实时监控数据，包含：

- 时间戳
- 活动连接数
- 当前并发调用数
- CPU 使用率
- 内存使用率

### OpenAI 风格 TTS WebSocket API

#### 连接端点

```
wss://your-server-host/tts/websocket
```

#### 请求格式

连接建立后，发送 JSON 格式的请求：

```json
{
  "model": "tts-1",
  "input": "这是一段需要转换为语音的文本",
  "voice": "alloy",  // 注意：此参数在当前版本中被忽略
  "speed": 1.0,
  "response_format": "pcm"
}
```

> **注意**：`voice` 参数在当前版本中被忽略，实际语音类型由环境变量 `BYTEDANCE_TTS_VOICE_TYPE` 确定。保留此参数是为了与 OpenAI TTS API 保持接口兼容性。

#### 响应格式

服务会流式返回二进制音频数据块，格式与火山引擎 TTS 服务保持一致。

### 健康检查端点

```
GET /health
```

返回服务状态信息：

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
  "version": "1.0.0",
  "last_check_time": "2025-10-29T10:30:00Z"
}
```

## 语音映射

> **重要说明**：OpenAI TTS 请求中的 `voice` 参数在当前版本中被忽略，不会影响实际的语音合成结果。
> 语音类型完全由环境变量 `BYTEDANCE_TTS_VOICE_TYPE` 决定。

以下是 OpenAI 语音名称到火山引擎语音 ID 的映射（仅供参考）：

| OpenAI 语音 | 火山引擎语音 ID |
|------------|--------------|
| alloy | zh_speaker_2 |
| echo | zh_speaker_1 |
| fable | zh_speaker_3 |
| onyx | zh_speaker_4 |
| nova | zh_speaker_5 |
| shimmer | zh_speaker_6 |

## 错误处理

服务会返回标准的 HTTP 错误码和错误信息：

- 400 Bad Request: 请求参数错误
- 403 Forbidden: 超出连接限制
- 500 Internal Server Error: 服务器内部错误
- 502 Bad Gateway: 火山引擎服务连接失败

## 监控指标

服务提供全面的监控指标，包括：

- **连接指标**：活动连接数、最大连接限制
- **调用指标**：当前并发调用数、最大并发调用限制
- **系统指标**：CPU 使用率、内存使用率
- **性能指标**：平均响应时间、请求成功率
- **时间指标**：服务运行时间、请求时间戳

这些指标可通过健康检查端点和 WebSocket 实时监控接口获取。

## 部署建议

- 使用环境变量或配置文件管理敏感信息
- 部署在支持 WebSocket 的环境中
- 配置适当的连接和调用限制以防止资源耗尽
- 考虑使用反向代理（如 Nginx）进行负载均衡
- 定期监控服务指标以确保稳定性
- 配置适当的日志级别以便故障排查

## 许可证

[MIT License](LICENSE)