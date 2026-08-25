# OpenAI → Qwen Token Plan 协议转换网关 — 规划文档

> 目标：提供一个高性能反向代理网关，对外暴露 OpenAI 兼容协议；文本模型直接透传到 Qwen Token Plan 的 OpenAI 兼容端点；图像模型（Qwen 自定义 multimodal-generation 协议）做请求/响应双向转换。
> 本文件为规划阶段产出，先评审、后实现。

---

## 1. 背景与需求

| # | 需求 | 说明 |
|---|------|------|
| 1 | 文本模型直接透传 | Token Plan 提供 OpenAI 兼容端点（`/compatible-mode/v1`），文本类（chat/completions、responses、embeddings、models）**不做任何 JSON 解析，纯字节级反向代理** |
| 2 | 图像模型重点转换 | Token Plan 图像模型走独立的自定义接口 `POST /api/v1/services/aigc/multimodal-generation/generation`，请求与响应结构都不同于 OpenAI Images API，需要双向转换 |
| 3 | 极致性能 | 只做协议转换：文本路径零解析、流式透传；图像路径最小化解析/分配；不落库、不缓存、无状态 |
| 4 | 环境变量配置 | Qwen base URL、Qwen API Key、对外暴露的 Key 全部通过环境变量注入 |
| 5 | Docker 部署 | 多阶段构建、静态二进制、非 root 运行、HEALTHCHECK、docker-compose 示例 |

---

## 2. 上游协议调研结论（已确认）

### 2.1 文本（Token Plan OpenAI 兼容模式）
- Base URL：`https://token-plan.<region>.maas.aliyuncs.com/compatible-mode/v1`
- 认证：`Authorization: Bearer sk-sp-xxxxx`（套餐专属 Key，`sk-sp-` 前缀）
- 支持 `/chat/completions`、`/responses` 等 OpenAI 兼容路径，与 OpenAI 客户端完全兼容
- → 结论：文本路径只需**原样转发**，仅替换认证头和 Host。

### 2.2 图像（Token Plan 自定义 multimodal-generation 协议）
- 端点：`POST https://token-plan.<region>.maas.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation`
- 认证：同 `Authorization: Bearer sk-sp-xxxxx`
- 同步调用（一次性返回结果，非任务轮询；视频/语音才是异步任务制，本次不涉及）
- 常用图像模型：`qwen-image-2.0`、`qwen-image-2.0-pro`、`wan2.7-image`、`wan2.7-image-pro`、`z-image-turbo` 等

#### Qwen 请求体（我们要转出的格式）
```json
{
  "model": "qwen-image-2.0",
  "input": {
    "messages": [
      {
        "role": "user",
        "content": [
          { "text": "描述" },
          { "image": "https://... 或 data:image/png;base64,..." }   // 图生图时出现，可 1~3 张
        ]
      }
    ]
  },
  "parameters": {
    "size": "1024*1024",        // 注意是 * 不是 x；范围 512*512 ~ 2048*2048
    "n": 1,                      // 1~6（qwen-image-2.0/3.0 系列）
    "negative_prompt": "...",    // 最多 500 字符
    "prompt_extend": true,       // 提示词改写
    "watermark": false,
    "seed": 1234567890,          // 0 ~ 2147483647
    "thinking": true             // 思考模式（qwen-image-3.0 系列；2.0 系列不支持则忽略）
  }
}
```

#### Qwen 响应体（我们要转成 OpenAI 格式）
```json
{
  "output": {
    "choices": [
      {
        "finish_reason": "stop",
        "message": {
          "role": "assistant",
          "content": [
            { "image": "https://dashscope-result-xxx.oss-.../xxx.png?Expires=xxx" },
            { "image": "https://dashscope-result-xxx.oss-.../xxx.png?Expires=xxx" }
          ]
        }
      }
    ]
  },
  "usage": {
    "output_width": 1024,
    "output_height": 1024,
    "output_image_count": 2,
    "input_image_count": 0
  },
  "request_id": "xxx"
}
```
要点：图片 URL 有效 **24 小时**（OpenAI 是 60 分钟，足够）；输出恒为 PNG；`output.choices[*].message.content[*].image` 即图片地址。

---

## 3. 对外接口列表（暴露给客户端）

| 方法 | 路径 | 类型 | 行为 |
|------|------|------|------|
| POST | `/v1/chat/completions` | 文本 | **透传**（字节级反向代理，含 SSE 流式） |
| POST | `/v1/responses` | 文本 | **透传** |
| POST | `/v1/embeddings` | 文本 | **透传** |
| GET  | `/v1/models` | 文本 | **透传**（也可本地合成，见 §7 可选） |
| POST | `/v1/images/generations` | 图像 | **转换**（OpenAI 文生图 → Qwen multimodal-generation） |
| POST | `/v1/images/edits` | 图像 | **转换**（OpenAI 图生图 multipart → Qwen multimodal-generation） |
| GET  | `/healthz` | 运维 | 健康检查（Docker HEALTHCHECK 使用） |
| *    | `/v1/*` 其他路径 | 通用 | **透传**（兜底，保证未来 OpenAI 新接口也能透传） |

> 暂不实现：`/v1/images/variations`（Qwen 无等价能力，可用图生图+固定改写提示词近似，作为后续增强项）、`/v1/audio/*`、`/v1/files/*`（Token Plan 语音走 WebSocket，不在本次范围）。

---

## 4. 图像协议转换映射表（核心）

### 4.1 请求：OpenAI `images/generations` → Qwen multimodal-generation

| OpenAI 字段 | 值示例 | 转换规则 | Qwen 字段 |
|---|---|---|---|
| `model` | `qwen-image-2.0` / `wan2.7-image` 等 | 请求的 model **原样透传**；`MODEL_ALIAS_<name>` 可选别名优先；未带 model 时兜底 `QWEN_IMAGE_MODEL` | `model` |
| `prompt` | `"a cat"` | 直接映射 | `input.messages[0].content[0].text` |
| `n` | `2` | 直接映射，钳制到 1~6 | `parameters.n` |
| `size` | `1024x1024` | **`x` → `*`**；非法/超出范围回退默认 | `parameters.size` |
| `quality` | `high`/`medium`/`low` | `high` → `prompt_extend: true`；`low` → `prompt_extend: false`；其余默认 | `parameters.prompt_extend` |
| `style` | `vivid`/`natural` | `vivid` 可在改写为 false 时忽略；不做硬映射 | —（可扩展） |
| `user` | `user-123` | 忽略（OpenAI 仅用于滥用追踪） | — |
| `response_format` | `url`/`b64_json` | **记录到转换上下文**，决定响应侧输出形式；不传给 Qwen | —（响应侧消费） |
| `output_format` | `png`/`jpeg`/`webp` | Qwen 恒输出 PNG：若请求 jpeg/webp，记录并在响应侧转换（见 §4.3）；默认忽略 | — |
| `background` | `transparent` | Qwen 不支持 → 忽略并打 warning 日志 | — |
| `thinking` | `off/low/medium/high` | 仅当目标模型为 qwen-image-3.0 系列时映射 `parameters.thinking`；否则忽略 | `parameters.thinking` |
| `quality` + `n` 等 OpenAI 专有字段 | — | 一律不向 Qwen 传递 | — |

### 4.2 请求：OpenAI `images/edits`（multipart/form-data）→ Qwen multimodal-generation

| OpenAI 字段 | 转换规则 | Qwen 字段 |
|---|---|---|
| `image`（文件） | 读取二进制 → `data:<mime>;base64,...` | `input.messages[0].content[0].image` |
| `mask`（可选文件） | 作为第二张输入图（best effort，Qwen 支持 1~3 张） | `content[1].image` |
| `prompt` | 直接映射 | `content[last].text` |
| `model` / `n` / `size` | 同 §4.1 | 同 §4.1 |

### 4.3 响应：Qwen multimodal-generation → OpenAI `images/generations` 响应

```json
// OpenAI 期望格式
{
  "created": 1750000000,          // Unix 秒，网关生成
  "data": [
    { "url": "https://...", "revised_prompt": "..." },   // url 模式
    { "b64_json": "..." }                                 // b64_json 模式
  ]
}
```

转换规则：
1. 遍历 `output.choices[*].message.content[*].image`，逐条映射到 `data[]`。
2. `response_format=url`（OpenAI 默认）：直接返回 Qwen 图片 URL（有效 24h），`revised_prompt` 省略（Qwen 不返回改写后提示词，2.0 系列无该字段）。
3. `response_format=b64_json`：网关**并发下载**各图片（有界并发 + 大小上限），转 Base64 后返回 `b64_json`；下载失败则该项返回错误结构。
4. `output_format` 非 png：对下载的图片做格式转换（若启用，需引入 `image` 解码库；**默认 V1 不实现，直接以 PNG 返回并记录 warning**，保持极致性能）。
5. 保留 `created`；Qwen 的 `request_id`、`usage` 不直接透传（OpenAI 无对应字段，可经响应头 `X-Request-Id` 透出便于排障）。
6. 上游非 200：**透传上游 JSON 错误体 + 状态码**，不改写（OpenAI 错误结构 `{"error":{...}}` 与 Qwen `{"code","message","request_id"}` 差异，V1 先原样透传，V2 可做错误体归一化）。

---

## 5. 模型名映射策略

| 情况 | 转发到 Qwen 的模型 |
|---|---|
| 请求带了 model（任意名字） | **原样透传**（客户端需传真实 Qwen 图像模型名，如 `qwen-image-2.0`、`wan2.7-image`） |
| 配置了 `MODEL_ALIAS_<原名>` | 别名优先（可选，如 `MODEL_ALIAS_gpt-image-1=qwen-image-2.0-pro`） |
| 请求未带 model | 兜底 `QWEN_IMAGE_MODEL`（默认 `qwen-image-2.0`） |

文本路径不做任何模型改写（纯透传）。

---

## 6. 架构设计

### 6.1 技术选型：Go 1.22+
理由：静态单二进制、内存占用低、标准库自带高性能 `net/http` 反向代理与流式处理、`sync.Pool` 零拷贝复用、goroutine 天然并发、Docker 可做到 scratch/distroless 极小镜像。

```
客户端 (OpenAI SDK / 任意 OpenAI 兼容工具)
        │  POST /v1/chat/completions, /v1/images/generations, ...
        ▼
┌─────────────────────────────┐
│  HTTP Server (LISTEN_ADDR)   │
│  ┌─────────┐   ┌──────────┐  │
│  │ 鉴权中间件 │   │ 路由      │  │
│  └─────────┘   └──────────┘  │
│       │                 │     │
│  ┌────▼─────┐      ┌────▼──────────┐
│  │ 文本透传    │      │ 图像转换器      │
│  │ (Reverse  │      │ request: 映射  │
│  │  Proxy,   │      │ response: 映射 │
│  │ 零解析)    │      │ b64: 并发下载   │
│  └────┬─────┘      └────┬──────────┘
│       │                 │
└───────┼─────────────────┼────────────┘
        ▼                 ▼
 QWEN_TEXT_BASE_URL   QWEN_IMAGE_BASE_URL
 (compatible-mode)    (multimodal-generation)
```

### 6.2 文本透传（性能核心）
- 使用 `httputil.ReverseProxy`：`Director` 仅做两件事——把 `Authorization` 替换为 `QWEN_API_KEY`、改写 Host/路径目标；**不触碰 body**。
- `FlushInterval = -1`：SSE 流式（`text/event-stream`）逐块即时转发，不缓冲。
- 连接池调优：`MaxIdleConnsPerHost`、`IdleConnTimeout`、`DisableCompression=false`（可省带宽），HTTP/2 上游启用。
- 统一 `http.Transport` 实例，全局复用，避免 TLS 握手重复开销。
- 默认不记录请求体/响应体（日志仅 method/path/status/耗时）。

### 6.3 图像转换（性能核心）
- 请求侧：仅对 `/v1/images/*` 做 JSON/multipart 解析；用 `json.Decoder`（或 `sonic`）流式解析，构造 Qwen 请求时复用 `sync.Pool` 缓冲。
- 响应侧：`output.choices[0].message.content[].image` 收集为 `[]string`；`url` 模式**零额外下载**直接映射；`b64_json` 模式有界并发下载（默认 4 并发）+ 大小上限（默认 20MB），全部失败才整体报错。
- 所有转换函数无状态、纯函数，便于单测与基准测试。

### 6.4 鉴权
- `EXPOSED_API_KEY` 非空：要求客户端 `Authorization: Bearer <EXPOSED_API_KEY>`（或 `api-key` 头），不匹配返回 `401`。
- `EXPOSED_API_KEY` 为空：不鉴权（便于内网/本地部署），启动时打 warning 日志。
- 仅做常量时间比较（`subtle.ConstantTimeCompare`），防时序攻击。

---

## 7. 环境变量清单

| 环境变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `QWEN_API_KEY` | ✅ | — | Qwen Token Plan 套餐专属 Key（`sk-sp-` 前缀） |
| `QWEN_BASE_URL` | — | `https://token-plan.cn-beijing.maas.aliyuncs.com` | 便捷配置：由此推导文本/图像端点；被下面两个覆盖 |
| `QWEN_TEXT_BASE_URL` | — | `{QWEN_BASE_URL}/compatible-mode/v1` | 文本透传目标 |
| `QWEN_IMAGE_BASE_URL` | — | `{QWEN_BASE_URL}/api/v1/services/aigc/multimodal-generation/generation` | 图像转换目标 |
| `EXPOSED_API_KEY` | — | 空（不鉴权） | 对外暴露的 Key，客户端必须携带 |
| `LISTEN_ADDR` | — | `:8080` | 监听地址 |
| `QWEN_IMAGE_MODEL` | — | `qwen-image-2.0` | 请求未带 model 时的兜底图像模型 |
| `MODEL_ALIAS_<name>` | — | 无 | 细粒度模型别名映射 |
| `IMAGE_DOWNLOAD_CONCURRENCY` | — | `4` | b64_json 模式并发下载数 |
| `IMAGE_MAX_BYTES` | — | `20971520` (20MB) | 单张图片下载上限 |
| `UPSTREAM_TIMEOUT` | — | `180s` | 图像上游超时（文生图较慢） |
| `LOG_LEVEL` | — | `info` | debug/info/warn/error |

`.env.example` 提供完整模板。

---

## 8. 项目结构（规划）

```
openai-to-qwen/
├── cmd/server/main.go          # 入口：加载配置、启动 HTTP
├── internal/
│   ├── config/config.go        # 环境变量解析 + 派生端点 + 模型别名表
│   ├── server/
│   │   ├── server.go           # 路由、中间件(鉴权/日志/恢复)
│   │   └── router.go           # 路径分流：/v1/images/* → 转换；其余 → 透传
│   ├── proxy/
│   │   └── proxy.go            # ReverseProxy 文本透传（Header 改写/连接池）
│   ├── image/
│   │   ├── convert_request.go  # OpenAI images 请求 → Qwen 请求
│   │   ├── convert_response.go # Qwen 响应 → OpenAI 响应
│   │   ├── edits.go            # multipart → Qwen（图生图）
│   │   └── download.go         # URL → base64（有界并发、限流、sync.Pool）
│   └── modelmap/modelmap.go    # 模型名映射
├── test/
│   ├── mock_qwen_test.go       # httptest 模拟 Qwen 上游做集成测试
│   └── convert_test.go         # 映射表单测 + golden 文件
├── Dockerfile                  # 多阶段构建
├── docker-compose.yml
├── .dockerignore
├── .env.example
├── Makefile                    # build/test/bench/docker 快捷命令
└── README.md
```

---

## 9. 性能设计要点（对照“极致性能”）

1. 文本路径零 JSON 解析、零 body 拷贝，`io.Copy` 级流式转发。
2. 全局复用 `http.Transport` 连接池（keep-alive、HTTP/2），避免重复 TLS 握手。
3. SSE 即时 flush，无缓冲延迟；`FlushInterval=-1`。
4. 图像转换用 `sync.Pool` 复用编解码缓冲；响应 `url` 模式零下载。
5. 无状态、无存储、无中间件拖累（鉴权为 O(1) 常量比较）。
6. 提供 `BenchmarkConvert` 基准测试，转换路径目标：< 1ms 均值（不含网络/下载）。
7. 可选开启上游 gzip，减少带宽与传输时间。

---

## 10. Docker 部署方案

### Dockerfile（多阶段）
```
阶段1 builder：golang:1.24-alpine → CGO_ENABLED=0 go build -ldflags="-s -w"
阶段2 runtime：gcr.io/distroless/static-debian12（或 alpine + ca-certificates）
  - 非 root 用户运行（USER 65532:65532）
  - EXPOSE 8080
  - HEALTHCHECK → wget/curl /healthz
  - ENTRYPOINT ["/openai-to-qwen"]
```
### docker-compose.yml
```yaml
services:
  openai-to-qwen:
    build: .
    ports: ["8080:8080"]
    env_file: .env
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/healthz"]
```

客户端接入示例（把网关当 OpenAI 用）：
```
base_url = http://localhost:8080/v1
api_key  = <EXPOSED_API_KEY>
```

---

## 11. 测试与验收

| 类型 | 内容 |
|---|---|
| 单元测试 | 请求/响应映射表逐字段断言；模型名映射；size `x`→`*`；n 钳制 |
| Golden 测试 | 典型 OpenAI 请求 → 期望 Qwen JSON 全量对比 |
| 集成测试 | `httptest` 模拟 Qwen 上游：文生图、图生图、b64_json 模式、上游错误透传 |
| 流式测试 | chat/completions SSE 分块透传完整性 |
| 基准测试 | `go test -bench=.`：图像转换耗时/分配数 |
| 手工验收 | 用 OpenAI SDK（python/node）分别调 text + images 验证端到端 |

### 里程碑（2026-08-25 已全部实现 ✅）\n1. ✅ M1：骨架 + 配置 + 文本透传 + 鉴权 + Docker（文本已跑通，冒烟测试经真实 Token Plan 端点验证）\n2. ✅ M2：`images/generations` 请求/响应转换 + 模型映射 + 单测\n3. ✅ M3：`images/edits`、b64_json 并发下载、错误透传\n4. ✅ M4：Docker 完善、README、基准测试、端到端验收（本机无 Docker，镜像构建需在有 Docker 的环境执行）

---

## 12. 开放问题（评审时确认）
1. ~~是否需要 `b64_json` 的 `output_format`（jpeg/webp）转换？~~ **已确认：不需要做**，恒以 PNG 返回，不引入图像解码库
2. ~~`/v1/models` 本地合成列表 vs 纯透传？~~ **已确认：纯透传**，不做本地合成
3. **已确认：图像响应把 Qwen `request_id` 透出到 `X-Request-Id` 响应头**
4. ~~`images/variations` 是否纳入？~~ **已确认：不纳入**，`/v1/images/variations` 返回 404



