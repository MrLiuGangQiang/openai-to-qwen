# OpenAI → Qwen Token Plan 协议转换网关

高性能反向代理：对外暴露 **OpenAI 兼容协议**，把请求转发到 Qwen Token Plan；文本模型**纯字节级透传**，图像模型（Qwen 自定义 `multimodal-generation` 协议）做**请求/响应双向转换**。无状态、不落库、只做转换。

## 特性

- **文本透传**：`/v1/chat/completions`、`/v1/responses`、`/v1/embeddings`、`/v1/models` 及所有 `/v1/*` 非图像路径，零 JSON 解析，字节级反向代理，SSE 流式即时转发（`FlushInterval=-1`）。
- **图像转换**（重点）：`/v1/images/generations`（文生图）、`/v1/images/edits`（图生图 multipart）→ Qwen `multimodal-generation`；响应反向映射为 OpenAI 格式，支持 `url` / `b64_json`。
- **极致性能**：转换纯函数 + `sync.Pool` 复用；`url` 模式零下载；`b64_json` 模式有界并发下载（默认 4，单张上限 20MB）；全局连接池 + HTTP/2。
  - 实测（i5-1135G7）：文生图请求转换 ~1.5µs/op，响应映射 ~89ns/op。
- **环境变量配置**：Qwen Base URL / Qwen Key / 对外 Key 全部走环境变量，`.env.example` 提供模板。
- **Docker 部署**：多阶段构建、非 root 运行、HEALTHCHECK、docker-compose 一键启动。

## 快速开始

### 本地运行

```bash
export QWEN_API_KEY=sk-sp-xxxx            # Token Plan 专属 Key
export EXPOSED_API_KEY=sk-my-exposed     # 对外暴露的 Key（可选，留空则不鉴权）
go run ./cmd/server
```

### Docker 部署

```bash
cp .env.example .env                     # 填入 QWEN_API_KEY / EXPOSED_API_KEY
docker compose up -d
```

### 客户端接入（把网关当作 OpenAI 使用）

```python
from openai import OpenAI
client = OpenAI(
    base_url="http://localhost:8080/v1",   # 网关地址
    api_key="sk-my-exposed",               # EXPOSED_API_KEY
)

# 文本：直接透传
chat = client.chat.completions.create(
    model="qwen3.8-max",
    messages=[{"role": "user", "content": "你好"}],
)

# 图像：OpenAI 文生图 → Qwen multimodal-generation
img = client.images.generate(
    model="gpt-image-1",          # 会被映射为 QWEN_IMAGE_MODEL（默认 qwen-image-2.0）
    prompt="a cat on the moon",
    size="1024x1024",
    n=1,
    response_format="url",        # 或 "b64_json"
)
```

```bash
# curl 文生图
curl http://localhost:8080/v1/images/generations \
  -H "Authorization: Bearer sk-my-exposed" \
  -H "Content-Type: application/json" \
  -d '{"model":"dall-e-3","prompt":"a cat","size":"1024x1024"}'
```

## 接口列表

| 方法 | 路径 | 行为 |
|---|---|---|
| POST | `/v1/chat/completions` | 文本透传（含 SSE） |
| POST | `/v1/responses` | 文本透传 |
| POST | `/v1/embeddings` | 文本透传 |
| GET | `/v1/models` | 文本透传 |
| POST | `/v1/images/generations` | **转换**：文生图 |
| POST | `/v1/images/edits` | **转换**：图生图（multipart） |
| GET | `/healthz` | 健康检查（免鉴权） |
| * | `/v1/*` 其他 | 文本透传兜底 |

> `/v1/images/variations` 明确不实现，返回 404。

## 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `QWEN_API_KEY` | ✅ | — | Token Plan 专属 Key（`sk-sp-` 前缀） |
| `QWEN_BASE_URL` | | `https://token-plan.cn-beijing.maas.aliyuncs.com` | 区域根地址，推导文本/图像端点 |
| `QWEN_TEXT_BASE_URL` | | `{QWEN_BASE_URL}/compatible-mode/v1` | 文本透传目标 |
| `QWEN_IMAGE_BASE_URL` | | `{QWEN_BASE_URL}/api/v1/services/aigc/multimodal-generation/generation` | 图像转换目标 |
| `EXPOSED_API_KEY` | | 空（不鉴权） | 对外暴露的 Key |
| `LISTEN_ADDR` | | `:8080` | 监听地址 |
| `QWEN_IMAGE_MODEL` | | `qwen-image-2.0` | 图像模型兜底/映射目标 |
| `MODEL_ALIAS_<name>` | | 无 | 模型别名，如 `MODEL_ALIAS_gpt-image-1=qwen-image-2.0-pro` |
| `IMAGE_DOWNLOAD_CONCURRENCY` | | `4` | b64_json 并发下载数 |
| `IMAGE_MAX_BYTES` | | `20971520` (20MB) | 单张图片下载上限 |
| `UPSTREAM_TIMEOUT` | | `180s` | 图像上游超时 |
| `LOG_LEVEL` | | `info` | 日志级别 |

## 图像协议转换说明

### 请求：OpenAI → Qwen

| OpenAI | Qwen |
|---|---|
| `model` | 别名映射 > Qwen 系原样透传（`qwen-image-*`/`wan*-image`/`z-image-*`）> 默认 `QWEN_IMAGE_MODEL` |
| `prompt` | `input.messages[0].content[0].text` |
| `n`（1~10） | `parameters.n`（钳制 1~6） |
| `size` `"1024x1024"` | `parameters.size` `"1024*1024"`（`x`→`*`；非法则省略用 Qwen 默认） |
| `quality=high/low` | `parameters.prompt_extend=true/false` |
| `thinking`（仅 qwen-image-3.0 目标） | `parameters.thinking` |
| `user` / `style` / `background` / `output_format` | 忽略（Qwen 无对应能力；`output_format` 恒 PNG） |

### 响应：Qwen → OpenAI

```jsonc
// Qwen
{ "output": { "choices": [{ "message": { "content": [ { "image": "https://..." } ] } }] },
  "request_id": "..." }
// → OpenAI
{ "created": 1750000000, "data": [ { "url": "https://..." } ] }        // url 模式，零下载
{ "created": 1750000000, "data": [ { "b64_json": "..." } ] }          // b64_json 模式，并发下载后返回
```

- `request_id` 透出到响应头 `X-Request-Id`。
- 上游非 2xx：状态码与错误体原样透传。
- `revised_prompt` 省略（Qwen 不返回改写后提示词）。

## 性能设计

- 文本路径：`httputil.ReverseProxy` + 全局 `http.Transport` 连接池（keep-alive、HTTP/2），`FlushInterval=-1` 即时 flush，**不解析 body**。
- 图像路径：转换函数无状态、纯函数，`sync.Pool` 复用缓冲；`b64_json` 有界并发 + 大小上限；日志不记录 body。
- 基准：`make bench`（`go test -bench . -benchmem ./internal/image/`）。

## 开发

```bash
make build    # 编译 bin/openai-to-qwen
make test     # 单元 + 集成测试（httptest 模拟 Qwen 上游，无需真实 Key）
make bench    # 基准测试
make docker   # 构建镜像
```

## 项目结构

```
cmd/server/          入口
internal/config/     环境变量配置
internal/proxy/      文本透传反向代理
internal/image/      OpenAI↔Qwen 图像协议转换（request/response/download/edits）
internal/modelmap/   模型名映射
internal/server/     路由、鉴权、日志、健康检查
```

## 已知限制

- Qwen 输出恒为 PNG；`output_format`（jpeg/webp）、透明背景（`background`）不支持。
- `images/variations` 不支持。
- 语音（TTS 走 WebSocket）、视频（异步任务制）不在范围内。