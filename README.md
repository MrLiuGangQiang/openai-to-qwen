# OpenAI 閳?Qwen Token Plan 閸楀繗顔呮潪顒佸床缂冩垵鍙?
妤傛ɑ鈧嗗厴閸欏秴鎮滄禒锝囨倞閿涙艾顕径鏍ㄦ瘹闂?**OpenAI 閸忕厧顔愰崡蹇氼唴**閿涘本濡哥拠閿嬬湴鏉烆剙褰傞崚?Qwen Token Plan閿涙稒鏋冮張顒伳侀崹?*缁绢垰鐡ч懞鍌滈獓闁繋绱?*閿涘苯娴橀崓蹇斈侀崹瀣剁礄Qwen 閼奉亜鐣炬稊?`multimodal-generation` 閸楀繗顔呴敍澶婁粵**鐠囬攱鐪?閸濆秴绨查崣灞芥倻鏉烆剚宕?*閵嗗倹妫ら悩鑸碘偓浣碘偓浣风瑝閽€钘夌氨閵嗕礁褰ч崑姘虫祮閹诡潿鈧?
## 閻楄鈧?
- **閺傚洦婀伴柅蹇庣炊**閿涙瓪/v1/chat/completions`閵嗕梗/v1/responses`閵嗕梗/v1/embeddings`閵嗕梗/v1/models` 閸欏﹥澧嶉張?`/v1/*` 闂堢偛娴橀崓蹇氱熅瀵板嫸绱濋梿?JSON 鐟欙絾鐎介敍灞界摟閼哄倻楠囬崣宥呮倻娴狅絿鎮婇敍瀛睸E 濞翠礁绱￠崡铏鏉烆剙褰傞敍鍧凢lushInterval=-1`閿涘鈧?- **閸ユ儳鍎氭潪顒佸床**閿涘牓鍣搁悙鐧哥礆閿涙瓪/v1/images/generations`閿涘牊鏋冮悽鐔锋禈閿涘鈧梗/v1/images/edits`閿涘牆娴橀悽鐔锋禈 multipart閿涘鍟?Qwen `multimodal-generation`閿涙稑鎼锋惔鏂垮冀閸氭垶妲х亸鍕礋 OpenAI 閺嶇厧绱￠敍灞炬暜閹?`url` / `b64_json`閵?- **閺嬩浇鍤ч幀褑鍏?*閿涙俺娴嗛幑銏㈠嚱閸戣姤鏆?+ `sync.Pool` 婢跺秶鏁ら敍娌梪rl` 濡€崇础闂嗘湹绗呮潪鏂ょ幢`b64_json` 濡€崇础閺堝鏅獮璺哄絺娑撳娴囬敍鍫ョ帛鐠?4閿涘苯宕熷鐘辩瑐闂?20MB閿涘绱遍崗銊ョ湰鏉╃偞甯村Ч?+ HTTP/2閵?  - 鐎圭偞绁撮敍鍧?-1135G7閿涘绱伴弬鍥╂晸閸ユ崘顕Ч鍌濇祮閹?~1.5纰宻/op閿涘苯鎼锋惔鏃€妲х亸?~89ns/op閵?- **閻滎垰顣ㄩ崣姗€鍣洪柊宥囩枂**閿涙瓐wen Base URL / Qwen Key / 鐎电懓顦?Key 閸忋劑鍎寸挧鎵箚婢у啫褰夐柌蹇ョ礉`.env.example` 閹绘劒绶靛Ο鈩冩緲閵?- **Docker 闁劎璁?*閿涙艾顦块梼鑸殿唽閺嬪嫬缂撻妴渚€娼?root 鏉╂劘顢戦妴涓燛ALTHCHECK閵嗕龚ocker-compose 娑撯偓闁款喖鎯庨崝銊ｂ偓?
## 韫囶偊鈧喎绱戞慨?
### 閺堫剙婀存潻鎰攽

```bash
export QWEN_API_KEY=sk-sp-xxxx            # Token Plan 娑撴挸鐫?Key
export EXPOSED_API_KEY=sk-my-exposed     # 鐎电懓顦婚弳鎾苟閻?Key閿涘牆褰查柅澶涚礉閻ｆ瑧鈹栭崚娆庣瑝闁村瓨娼堥敍?go run ./cmd/server
```

### Docker 闁劎璁?
```bash
cp .env.example .env                     # 婵夘偄鍙?QWEN_API_KEY / EXPOSED_API_KEY
docker compose up -d
```

### 鐎广垺鍩涚粩顖涘复閸忋儻绱欓幎濠勭秹閸忓啿缍嬫担?OpenAI 娴ｈ法鏁ら敍?
```python
from openai import OpenAI
client = OpenAI(
    base_url="http://localhost:8080/v1",   # 缂冩垵鍙ч崷鏉挎絻
    api_key="sk-my-exposed",               # EXPOSED_API_KEY
)

# 閺傚洦婀伴敍姘辨纯閹恒儵鈧繋绱?chat = client.chat.completions.create(
    model="qwen3.8-max",
    messages=[{"role": "user", "content": "娴ｇ姴銈?}],
)

# 閸ユ儳鍎氶敍姝刾enAI 閺傚洨鏁撻崶?閳?Qwen multimodal-generation
img = client.images.generate(
    model="gpt-image-1",          # 娴兼俺顫﹂弰鐘茬殸娑?QWEN_IMAGE_MODEL閿涘牓绮拋?qwen-image-2.0閿?    prompt="a cat on the moon",
    size="1024x1024",
    n=1,
    response_format="url",        # 閹?"b64_json"
)
```

```bash
# curl 閺傚洨鏁撻崶?curl http://localhost:8080/v1/images/generations \
  -H "Authorization: Bearer sk-my-exposed" \
  -H "Content-Type: application/json" \
  -d '{"model":"dall-e-3","prompt":"a cat","size":"1024x1024"}'
```

## 閹恒儱褰涢崚妤勩€?
| 閺傝纭?| 鐠侯垰绶?| 鐞涘奔璐?|
|---|---|---|
| POST | `/v1/chat/completions` | 閺傚洦婀伴柅蹇庣炊閿涘牆鎯?SSE閿?|
| POST | `/v1/responses` | 閺傚洦婀伴柅蹇庣炊 |
| POST | `/v1/embeddings` | 閺傚洦婀伴柅蹇庣炊 |
| GET | `/v1/models` | 閺傚洦婀伴柅蹇庣炊 |
| POST | `/v1/images/generations` | **鏉烆剚宕?*閿涙碍鏋冮悽鐔锋禈 |
| POST | `/v1/images/edits` | **鏉烆剚宕?*閿涙艾娴橀悽鐔锋禈閿涘潰ultipart閿?|
| GET | `/healthz` | 閸嬨儱鎮嶅Λ鈧弻銉礄閸忓秹澹岄弶鍐跨礆 |
| * | `/v1/*` 閸忔湹绮?| 閺傚洦婀伴柅蹇庣炊閸忔粌绨?|

> `/v1/images/variations` 閺勫海鈥樻稉宥呯杽閻滃府绱濇潻鏂挎礀 404閵?
## 閻滎垰顣ㄩ崣姗€鍣?
| 閸欐﹢鍣?| 韫囧懎锝?| 姒涙顓婚崐?| 鐠囧瓨妲?|
|---|---|---|---|
| `QWEN_API_KEY` | 閴?| 閳?| Token Plan 娑撴挸鐫?Key閿涘潉sk-sp-` 閸撳秶绱戦敍?|
| `QWEN_BASE_URL` | | `https://token-plan.cn-beijing.maas.aliyuncs.com` | 閸栧搫鐓欓弽鐟版勾閸р偓閿涘本甯圭€靛吋鏋冮張?閸ユ儳鍎氱粩顖滃仯 |
| `QWEN_TEXT_BASE_URL` | | `{QWEN_BASE_URL}/compatible-mode/v1` | 閺傚洦婀伴柅蹇庣炊閻╊喗鐖?|
| `QWEN_IMAGE_BASE_URL` | | `{QWEN_BASE_URL}/api/v1/services/aigc/multimodal-generation/generation` | 閸ユ儳鍎氭潪顒佸床閻╊喗鐖?|
| `EXPOSED_API_KEY` | | 缁岀尨绱欐稉宥夊閺夊喛绱?| 鐎电懓顦婚弳鎾苟閻?Key |
| `LISTEN_ADDR` | | `:8080` | 閻╂垵鎯夐崷鏉挎絻 |
| `QWEN_IMAGE_MODEL` | | `qwen-image-2.0` | 閸ユ儳鍎氬Ο鈥崇€烽崗婊冪俺/閺勭姴鐨犻惄顔界垼 |
| `MODEL_ALIAS_<name>` | | 閺?| 濡€崇€烽崚顐㈡倳閿涘苯顩?`MODEL_ALIAS_gpt-image-1=qwen-image-2.0-pro` |
| `IMAGE_DOWNLOAD_CONCURRENCY` | | `4` | b64_json 楠炶泛褰傛稉瀣祰閺?|
| `IMAGE_MAX_BYTES` | | `20971520` (20MB) | 閸楁洖绱堕崶鍓у娑撳娴囨稉濠囨 |
| `UPSTREAM_TIMEOUT` | | `180s` | 閸ユ儳鍎氭稉濠冪埗鐡掑懏妞?|
| `LOG_LEVEL` | | `info` | 閺冦儱绻旂痪褍鍩?|

## 閸ユ儳鍎氶崡蹇氼唴鏉烆剚宕茬拠瀛樻

### 鐠囬攱鐪伴敍姝刾enAI 閳?Qwen

| OpenAI | Qwen |
|---|---|
| `model` | 閸掝偄鎮曢弰鐘茬殸 > Qwen 缁甯弽鐑解偓蹇庣炊閿涘潉qwen-image-*`/`wan*-image`/`z-image-*`閿? 姒涙顓?`QWEN_IMAGE_MODEL` |
| `prompt` | `input.messages[0].content[0].text` |
| `n`閿?~10閿?| `parameters.n`閿涘牓鎸搁崚?1~6閿?|
| `size` `"1024x1024"` | `parameters.size` `"1024*1024"`閿涘潉x`閳妶*`閿涙盯娼▔鏇炲灟閻胶鏆愰悽?Qwen 姒涙顓婚敍?|
| `quality=high/low` | `parameters.prompt_extend=true/false` |
| `thinking`閿涘牅绮?qwen-image-3.0 閻╊喗鐖ｉ敍?| `parameters.thinking` |
| `user` / `style` / `background` / `output_format` | 韫囩晫鏆愰敍鍦en 閺冪姴顕惔鏃囧厴閸旀冻绱盽output_format` 閹?PNG閿?|

### 閸濆秴绨查敍姝坵en 閳?OpenAI

```jsonc
// Qwen
{ "output": { "choices": [{ "message": { "content": [ { "image": "https://..." } ] } }] },
  "request_id": "..." }
// 閳?OpenAI
{ "created": 1750000000, "data": [ { "url": "https://..." } ] }        // url 濡€崇础閿涘矂娴傛稉瀣祰
{ "created": 1750000000, "data": [ { "b64_json": "..." } ] }          // b64_json 濡€崇础閿涘苯鑻熼崣鎴滅瑓鏉炶棄鎮楁潻鏂挎礀
```

- `request_id` 闁繐鍤崚鏉挎惙鎼存柨銇?`X-Request-Id`閵?- 娑撳﹥鐖堕棃?2xx閿涙氨濮搁幀浣虹垳娑撳酣鏁婄拠顖欑秼閸樼喐鐗遍柅蹇庣炊閵?- `revised_prompt` 閻胶鏆愰敍鍦en 娑撳秷绻戦崶鐐存暭閸愭瑥鎮楅幓鎰仛鐠囧稄绱氶妴?
## 閹嗗厴鐠佹崘顓?
- 閺傚洦婀扮捄顖氱窞閿涙瓪httputil.ReverseProxy` + 閸忋劌鐪?`http.Transport` 鏉╃偞甯村Ч鐙呯礄keep-alive閵嗕笭TTP/2閿涘绱漙FlushInterval=-1` 閸楄櫕妞?flush閿?*娑撳秷袙閺?body**閵?- 閸ユ儳鍎氱捄顖氱窞閿涙俺娴嗛幑銏犲毐閺佺増妫ら悩鑸碘偓浣碘偓浣哄嚱閸戣姤鏆熼敍瀹峴ync.Pool` 婢跺秶鏁ょ紓鎾冲暱閿涙矖b64_json` 閺堝鏅獮璺哄絺 + 婢堆冪毈娑撳﹪妾洪敍娑欐）韫囨ぞ绗夌拋鏉跨秿 body閵?- 閸╁搫鍣敍姝歮ake bench`閿涘潉go test -bench . -benchmem ./internal/image/`閿涘鈧?
## 瀵偓閸?
```bash
make build    # 缂傛牞鐦?bin/openai-to-qwen
make test     # 閸楁洖鍘?+ 闂嗗棙鍨氬ù瀣槸閿涘潝ttptest 濡剝瀚?Qwen 娑撳﹥鐖堕敍灞炬￥闂団偓閻喎鐤?Key閿?make bench    # 閸╁搫鍣ù瀣槸
make docker   # 閺嬪嫬缂撻梹婊冨剼
```

## 妞ゅ湱娲扮紒鎾寸€?
```
cmd/server/          閸忋儱褰?internal/config/     閻滎垰顣ㄩ崣姗€鍣洪柊宥囩枂
internal/proxy/      閺傚洦婀伴柅蹇庣炊閸欏秴鎮滄禒锝囨倞
internal/image/      OpenAI閳摪wen 閸ユ儳鍎氶崡蹇氼唴鏉烆剚宕查敍鍧甧quest/response/download/edits閿?internal/modelmap/   濡€崇€烽崥宥嗘Ё鐏?internal/server/     鐠侯垳鏁遍妴渚€澹岄弶鍐︹偓浣规）韫囨ぜ鈧礁浠存惔閿嬵梾閺?```

## 瀹歌尙鐓￠梽鎰煑

- Qwen 鏉堟挸鍤幁鎺嶈礋 PNG閿涙矖output_format`閿涘潠peg/webp閿涘鈧線鈧繑妲戦懗灞炬珯閿涘潉background`閿涘绗夐弨顖涘瘮閵?- `images/variations` 娑撳秵鏁幐浣碘偓?- 鐠囶參鐓堕敍鍦盩S 鐠?WebSocket閿涘鈧浇顫嬫０鎴礄瀵倹顒炴禒璇插閸掕绱氭稉宥呮躬閼煎啫娲块崘鍛偓
## 鍙戝竷锛堟帹閫佸埌闃块噷浜?ACR锛?
闀滃儚鍦板潃锛歚registry.cn-hangzhou.aliyuncs.com/liugangqiang/openai-to-qwen`

涓夌鍙戝竷鏂瑰紡锛堜换閫夊叾涓€锛夛細

1. **GitHub Actions锛堟帹鑽愶級**锛氭帹閫?`v*` tag 鍚庤嚜鍔ㄦ瀯寤哄苟鎺ㄩ€併€備粨搴撻渶閰嶇疆 Secrets锛歚ALIYUN_REGISTRY_USERNAME`銆乣ALIYUN_REGISTRY_PASSWORD`銆備篃鍙湪 Actions 椤甸潰鎵嬪姩瑙﹀彂 `release` 宸ヤ綔娴併€?2. **鏈満鏃?Docker 鏃讹紙daemonless锛?*锛氫娇鐢ㄤ粨搴撳唴缃伐鍏凤紝鐩存帴鏋勫缓 OCI 闀滃儚骞舵帹閫侊紙scratch + 闈欐€佷簩杩涘埗 + CA 璇佷功锛屾棤闇€浠讳綍瀹瑰櫒杩愯鏃讹級锛?   ```bash
   ACR_USERNAME=<浣犵殑ACR鐢ㄦ埛鍚? ACR_PASSWORD=<浣犵殑ACR瀵嗙爜> go run ./tools/release
   # 榛樿鎺?registry.cn-hangzhou.aliyuncs.com/liugangqiang/openai-to-qwen:v1.0.0 鍜?:latest
   # 鏈湴楠岃瘉锛堜笉鎺ㄩ€侊級锛歡o run ./tools/release -tarball out.tar / -extract dir
   ```
3. **鏈?Docker 鏃?*锛歚docker build -t registry.cn-hangzhou.aliyuncs.com/liugangqiang/openai-to-qwen:v1.0.0 . && docker push ...`
## 日志说明（排查用）

网关日志分三类，`docker logs openai-to-qwen` 里按时间看：

**1. 访问日志（每个请求）**
```
req method=POST path=/v1/images/generations query="" remote=1.2.3.4:5678 ua=curl status=200 duration=69.6s bytes_in=42
```

**2. 图像路径（转换 + 上游）**
```
image generations incoming path=/v1/images/generations body={"model":"qwen-image-3.0-pro",...}   ← 客户端发来的原始请求
image generations: model=qwen-image-3.0-pro params=map[] response_format="" prompt_len=4
image upstream request url=https://token-plan.../generation model=qwen-image-3.0-pro body={...}    ← 实际转发给 Token Plan 的请求
image ok: model=qwen-image-3.0-pro status=200 upstream=69.5s total=69.6s images=1 request_id=xxx first_url=https://...
image upstream non-2xx: model=... status=400 duration=0.3s request_id=xxx request={...} body={...}   ← 上游报错时，转发的请求和错误体都会打出来
image upstream error: model=... duration=180s err=context deadline exceeded                          ← 上游挂起超时
```

**3. 文本路径（透传）**
```
text upstream url=https://token-plan.../chat/completions status=200 duration=1.2s content_type=application/json
```

**排查口诀**：
- 有 `req` 行但没 `image generations incoming` / `text upstream` → 请求没进转换/转发逻辑（路径不对，比如少了 /v1）
- 有 `image upstream request` 但没有后续 `image ok` / `non-2xx` / `error` → 请求发出去后上游挂起，等超时
- `image upstream non-2xx` 里的 `body=` 就是上游真实报错（如 Unsupported model）
- 完全没有 `req` 行 → 请求根本没到网关（被前面 nginx/ingress 拦了）