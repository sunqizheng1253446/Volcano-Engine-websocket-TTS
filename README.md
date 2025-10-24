# TTS WebSocket 中转服务

## 项目介绍

这是一个基于 Go 语言开发的 TTS（Text-to-Speech）Websocket 中转服务，用于将 OpenAI 风格的 TTS 请求转换为火山引擎（字节跳动）的 TTS WebSocket API 请求，并将响应流式返回给客户端。

## 功能特性

- 支持 OpenAI 风格的 WebSocket TTS API
- 自动转换为火山引擎 TTS WebSocket 协议
- 流式响应处理
- 并发连接和调用限制
- 健康检查端点
- 环境变量配置
- 完善的错误处理
- 服务运行状态监控

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

| 环境变量 | 类型 | 默认值 | 描述 |
|---------|------|-------|------|
| `LISTEN_ADDR` | string | `:8080` | 服务监听地址和端口 |
| `BYTEDANCE_TTS_ADDR` | string | `openspeech.bytedance.com` | 火山引擎 TTS 服务地址 |
| `BYTEDANCE_TTS_PATH` | string | `/api/v1/tts/ws_binary` | 火山引擎 TTS API 路径 |
| `BYTEDANCE_TTS_APP_ID` | string | (必需) | 火山引擎 App ID |
| `BYTEDANCE_TTS_BEARER_TOKEN` | string | (必需) | 火山引擎认证令牌 |
| `BYTEDANCE_TTS_CLUSTER` | string | (必需) | 火山引擎集群名称 |
| `BYTEDANCE_TTS_VOICE_TYPE` | string | (必需) | 火山引擎语音类型 |
| `MAX_CONNECTIONS` | int | 100 | 最大并发连接数 |
| `MAX_CONCURRENT_CALLS` | int | 10 | 最大并发调用数 |
| `LOG_LEVEL` | string | `info` | 日志级别（debug, info, warn, error） |
| `GIN_MODE` | string | `release` | Gin 框架模式 |

### 使用 .env 文件

你可以创建一个 `.env` 文件来管理环境变量。示例文件参见 `.env.example`。

## API 使用

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
  "voice": "alloy",
  "speed": 1.0,
  "response_format": "pcm"
}
```

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
  "active_connections": 5,
  "max_connections": 100,
  "current_calls": 3,
  "max_concurrent_calls": 10,
  "uptime_seconds": 120
}
```

## 语音映射

以下是 OpenAI 语音名称到火山引擎语音 ID 的映射：

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

健康检查端点提供以下监控指标：

- 活动连接数
- 最大连接数
- 当前并发调用数
- 最大并发调用数
- 服务运行时间（秒）

## 部署建议

- 使用环境变量或配置文件管理敏感信息
- 部署在支持 WebSocket 的环境中
- 配置适当的连接和调用限制以防止资源耗尽
- 考虑使用反向代理（如 Nginx）进行负载均衡

## 许可证

[MIT License](LICENSE)