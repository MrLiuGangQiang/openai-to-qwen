# OpenAI 鈫?Qwen Token Plan 鍗忚杞崲缃戝叧

楂樻€ц兘鍙嶅悜浠ｇ悊锛氬澶栨毚闇?**OpenAI 鍏煎鍗忚**锛屾妸璇锋眰杞彂鍒?Qwen Token Plan锛涙枃鏈ā鍨?*绾瓧鑺傜骇閫忎紶**锛屽浘鍍忔ā鍨嬶紙Qwen 鑷畾涔?`multimodal-generation` 鍗忚锛夊仛**璇锋眰/鍝嶅簲鍙屽悜杞崲**銆傛棤鐘舵€併€佷笉钀藉簱銆佸彧鍋氳浆鎹€?
## 鐗规€?
- **鏂囨湰閫忎紶**锛歚/v1/chat/completions`銆乣/v1/responses`銆乣/v1/embeddings`銆乣/v1/models` 鍙婃墍鏈?`/v1/*` 闈炲浘鍍忚矾寰勶紝闆?JSON 瑙ｆ瀽锛屽瓧鑺傜骇鍙嶅悜浠ｇ悊锛孲SE 娴佸紡鍗虫椂杞彂锛坄FlushInterval=-1`锛夈€?- **鍥惧儚杞崲**锛堥噸鐐癸級锛歚/v1/images/generations`锛堟枃鐢熷浘锛夈€乣/v1/images/edits`锛堝浘鐢熷浘 multipart锛夆啋 Qwen `multimodal-generation`锛涘搷搴斿弽鍚戞槧灏勪负 OpenAI 鏍煎紡锛屾敮鎸?`url` / `b64_json`銆?- **鏋佽嚧鎬ц兘**锛氳浆鎹㈢函鍑芥暟 + `sync.Pool` 澶嶇敤锛沗url` 妯″紡闆朵笅杞斤紱`b64_json` 妯″紡鏈夌晫骞跺彂涓嬭浇锛堥粯璁?4锛屽崟寮犱笂闄?20MB锛夛紱鍏ㄥ眬杩炴帴姹?+ HTTP/2銆?  - 瀹炴祴锛坕5-1135G7锛夛細鏂囩敓鍥捐姹傝浆鎹?~1.5碌s/op锛屽搷搴旀槧灏?~89ns/op銆?- **鐜鍙橀噺閰嶇疆**锛歈wen Base URL / Qwen Key / 瀵瑰 Key 鍏ㄩ儴璧扮幆澧冨彉閲忥紝`.env.example` 鎻愪緵妯℃澘銆?- **Docker 閮ㄧ讲**锛氬闃舵鏋勫缓銆侀潪 root 杩愯銆丠EALTHCHECK銆乨ocker-compose 涓€閿惎鍔ㄣ€?
## 蹇€熷紑濮?
### 鏈湴杩愯

```bash
export QWEN_API_KEY=sk-sp-xxxx            # Token Plan 涓撳睘 Key
export EXPOSED_API_KEY=sk-my-exposed     # 瀵瑰鏆撮湶鐨?Key锛堝彲閫夛紝鐣欑┖鍒欎笉閴存潈锛?go run ./cmd/server
```

### Docker 閮ㄧ讲

```bash
cp .env.example .env                     # 濉叆 QWEN_API_KEY / EXPOSED_API_KEY
docker compose up -d
```

### 瀹㈡埛绔帴鍏ワ紙鎶婄綉鍏冲綋浣?OpenAI 浣跨敤锛?
```python
from openai import OpenAI
client = OpenAI(
    base_url="http://localhost:8080/v1",   # 缃戝叧鍦板潃
    api_key="sk-my-exposed",               # EXPOSED_API_KEY
)

# 鏂囨湰锛氱洿鎺ラ€忎紶
chat = client.chat.completions.create(
    model="qwen3.8-max",
    messages=[{"role": "user", "content": "浣犲ソ"}],
)

# 鍥惧儚锛歄penAI 鏂囩敓鍥?鈫?Qwen multimodal-generation
img = client.images.generate(
    model="gpt-image-1",          # 浼氳鏄犲皠涓?QWEN_IMAGE_MODEL锛堥粯璁?qwen-image-2.0锛?    prompt="a cat on the moon",
    size="1024x1024",
    n=1,
    response_format="url",        # 鎴?"b64_json"
)
```

```bash
# curl 鏂囩敓鍥?curl http://localhost:8080/v1/images/generations \
  -H "Authorization: Bearer sk-my-exposed" \
  -H "Content-Type: application/json" \
  -d '{"model":"dall-e-3","prompt":"a cat","size":"1024x1024"}'
```

## 鎺ュ彛鍒楄〃

| 鏂规硶 | 璺緞 | 琛屼负 |
|---|---|---|
| POST | `/v1/chat/completions` | 鏂囨湰閫忎紶锛堝惈 SSE锛?|
| POST | `/v1/responses` | 鏂囨湰閫忎紶 |
| POST | `/v1/embeddings` | 鏂囨湰閫忎紶 |
| GET | `/v1/models` | 鏂囨湰閫忎紶 |
| POST | `/v1/images/generations` | **杞崲**锛氭枃鐢熷浘 |
| POST | `/v1/images/edits` | **杞崲**锛氬浘鐢熷浘锛坢ultipart锛?|
| GET | `/healthz` | 鍋ュ悍妫€鏌ワ紙鍏嶉壌鏉冿級 |
| * | `/v1/*` 鍏朵粬 | 鏂囨湰閫忎紶鍏滃簳 |

> `/v1/images/variations` 鏄庣‘涓嶅疄鐜帮紝杩斿洖 404銆?
## 鐜鍙橀噺

| 鍙橀噺 | 蹇呭～ | 榛樿鍊?| 璇存槑 |
|---|---|---|---|
| `QWEN_API_KEY` | 鉁?| 鈥?| Token Plan 涓撳睘 Key锛坄sk-sp-` 鍓嶇紑锛?|
| `QWEN_BASE_URL` | | `https://token-plan.cn-beijing.maas.aliyuncs.com` | 鍖哄煙鏍瑰湴鍧€锛屾帹瀵兼枃鏈?鍥惧儚绔偣 |
| `QWEN_TEXT_BASE_URL` | | `{QWEN_BASE_URL}/compatible-mode/v1` | 鏂囨湰閫忎紶鐩爣 |
| `QWEN_IMAGE_BASE_URL` | | `{QWEN_BASE_URL}/api/v1/services/aigc/multimodal-generation/generation` | 鍥惧儚杞崲鐩爣 |
| `EXPOSED_API_KEY` | | 绌猴紙涓嶉壌鏉冿級 | 瀵瑰鏆撮湶鐨?Key |
| `LISTEN_ADDR` | | `:8080` | 鐩戝惉鍦板潃 |
| `QWEN_IMAGE_MODEL` | | `qwen-image-2.0` | 鍥惧儚妯″瀷鍏滃簳/鏄犲皠鐩爣 |
| `MODEL_ALIAS_<name>` | | 鏃?| 妯″瀷鍒悕锛屽 `MODEL_ALIAS_gpt-image-1=qwen-image-2.0-pro` |
| `IMAGE_DOWNLOAD_CONCURRENCY` | | `4` | b64_json 骞跺彂涓嬭浇鏁?|
| `IMAGE_MAX_BYTES` | | `20971520` (20MB) | 鍗曞紶鍥剧墖涓嬭浇涓婇檺 |
| `UPSTREAM_TIMEOUT` | | `180s` | 鍥惧儚涓婃父瓒呮椂 |
| `LOG_LEVEL` | | `info` | 鏃ュ織绾у埆 |

## 鍥惧儚鍗忚杞崲璇存槑

### 璇锋眰锛歄penAI 鈫?Qwen

| OpenAI | Qwen |
|---|---|
| `model` | 鍒悕鏄犲皠 > Qwen 绯诲師鏍烽€忎紶锛坄qwen-image-*`/`wan*-image`/`z-image-*`锛? 榛樿 `QWEN_IMAGE_MODEL` |
| `prompt` | `input.messages[0].content[0].text` |
| `n`锛?~10锛?| `parameters.n`锛堥挸鍒?1~6锛?|
| `size` `"1024x1024"` | `parameters.size` `"1024*1024"`锛坄x`鈫抈*`锛涢潪娉曞垯鐪佺暐鐢?Qwen 榛樿锛?|
| `quality=high/low` | `parameters.prompt_extend=true/false` |
| `thinking`锛堜粎 qwen-image-3.0 鐩爣锛?| `parameters.thinking` |
| `user` / `style` / `background` / `output_format` | 蹇界暐锛圦wen 鏃犲搴旇兘鍔涳紱`output_format` 鎭?PNG锛?|

### 鍝嶅簲锛歈wen 鈫?OpenAI

```jsonc
// Qwen
{ "output": { "choices": [{ "message": { "content": [ { "image": "https://..." } ] } }] },
  "request_id": "..." }
// 鈫?OpenAI
{ "created": 1750000000, "data": [ { "url": "https://..." } ] }        // url 妯″紡锛岄浂涓嬭浇
{ "created": 1750000000, "data": [ { "b64_json": "..." } ] }          // b64_json 妯″紡锛屽苟鍙戜笅杞藉悗杩斿洖
```

- `request_id` 閫忓嚭鍒板搷搴斿ご `X-Request-Id`銆?- 涓婃父闈?2xx锛氱姸鎬佺爜涓庨敊璇綋鍘熸牱閫忎紶銆?- `revised_prompt` 鐪佺暐锛圦wen 涓嶈繑鍥炴敼鍐欏悗鎻愮ず璇嶏級銆?
## 鎬ц兘璁捐

- 鏂囨湰璺緞锛歚httputil.ReverseProxy` + 鍏ㄥ眬 `http.Transport` 杩炴帴姹狅紙keep-alive銆丠TTP/2锛夛紝`FlushInterval=-1` 鍗虫椂 flush锛?*涓嶈В鏋?body**銆?- 鍥惧儚璺緞锛氳浆鎹㈠嚱鏁版棤鐘舵€併€佺函鍑芥暟锛宍sync.Pool` 澶嶇敤缂撳啿锛沗b64_json` 鏈夌晫骞跺彂 + 澶у皬涓婇檺锛涙棩蹇椾笉璁板綍 body銆?- 鍩哄噯锛歚make bench`锛坄go test -bench . -benchmem ./internal/image/`锛夈€?
## 寮€鍙?
```bash
make build    # 缂栬瘧 bin/openai-to-qwen
make test     # 鍗曞厓 + 闆嗘垚娴嬭瘯锛坔ttptest 妯℃嫙 Qwen 涓婃父锛屾棤闇€鐪熷疄 Key锛?make bench    # 鍩哄噯娴嬭瘯
make docker   # 鏋勫缓闀滃儚
```

## 椤圭洰缁撴瀯

```
cmd/server/          鍏ュ彛
internal/config/     鐜鍙橀噺閰嶇疆
internal/proxy/      鏂囨湰閫忎紶鍙嶅悜浠ｇ悊
internal/image/      OpenAI鈫擰wen 鍥惧儚鍗忚杞崲锛坮equest/response/download/edits锛?internal/modelmap/   妯″瀷鍚嶆槧灏?internal/server/     璺敱銆侀壌鏉冦€佹棩蹇椼€佸仴搴锋鏌?```

## 宸茬煡闄愬埗

- Qwen 杈撳嚭鎭掍负 PNG锛沗output_format`锛坖peg/webp锛夈€侀€忔槑鑳屾櫙锛坄background`锛変笉鏀寔銆?- `images/variations` 涓嶆敮鎸併€?- 璇煶锛圱TS 璧?WebSocket锛夈€佽棰戯紙寮傛浠诲姟鍒讹級涓嶅湪鑼冨洿鍐呫€
## 发布（推送到阿里云 ACR）

镜像地址：`registry.cn-hangzhou.aliyuncs.com/liugangqiang/openai-to-qwen`

三种发布方式（任选其一）：

1. **GitHub Actions（推荐）**：推送 `v*` tag 后自动构建并推送。仓库需配置 Secrets：`ALIYUN_REGISTRY_USERNAME`、`ALIYUN_REGISTRY_PASSWORD`。也可在 Actions 页面手动触发 `release` 工作流。
2. **本机无 Docker 时（daemonless）**：使用仓库内置工具，直接构建 OCI 镜像并推送（scratch + 静态二进制 + CA 证书，无需任何容器运行时）：
   ```bash
   ACR_USERNAME=<你的ACR用户名> ACR_PASSWORD=<你的ACR密码> go run ./tools/release
   # 默认推 registry.cn-hangzhou.aliyuncs.com/liugangqiang/openai-to-qwen:v1.0.0 和 :latest
   # 本地验证（不推送）：go run ./tools/release -tarball out.tar / -extract dir
   ```
3. **有 Docker 时**：`docker build -t registry.cn-hangzhou.aliyuncs.com/liugangqiang/openai-to-qwen:v1.0.0 . && docker push ...`