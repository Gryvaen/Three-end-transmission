# API

所有接口与页面同源。启用口令时，先调用 `/api/auth` 获取 cookie。

## WebSocket `GET /ws`

| 参数 | 说明 |
|------|------|
| `name` | 设备昵称 |
| `key` | 设备密钥（64 位十六进制），服务端据此推导设备 ID；省略则分配随机 ID |
| `platform` | `android` / `ios` / `windows` / `linux` / `macos` / `unknown`；省略则按 User-Agent 推断 |

连接后服务端依次推送：

```json
{ "type": "welcome",  "device": { "id": "…", "name": "…", "platform": "linux", "ip": "192.168.1.2" } }
{ "type": "history",  "messages": [] }
{ "type": "presence", "users": [ { "id": "…", "name": "…", "platform": "android", "ip": "…" } ] }
```

发送消息；加上 `to`（设备 ID 数组）即为私信，接收者全部无效时消息被丢弃：

```json
{ "type": "message", "payload": { "kind": "text", "content": "你好" } }
{ "type": "message", "to": ["3f2c…"], "payload": { "kind": "text", "content": "只给你" } }
```

文件消息先上传得到 `fileId`，`kind` 为 `file` 或 `image`：

```json
{ "kind": "file", "fileId": "a1b2…", "meta": { "name": "report.pdf", "size": 1024, "mime": "application/pdf" } }
```

## HTTP

| 路径 | 方法 | 说明 |
|------|------|------|
| `/api/info` | GET | 加入地址、是否需要口令、保留时长、上传上限 |
| `/api/qrcode?url=` | GET | PNG 二维码 |
| `/api/auth` | POST | `{"pin":"…"}`，正确则下发会话 cookie |
| `/api/uploads` | POST | 创建续传会话：`{"name","size","mime"}` → `{"uploadId","offset","chunkSize"}` |
| `/api/uploads/{id}` | GET | 查询已写入的 `offset`，完成后返回 `file` |
| `/api/uploads/{id}?offset=N` | PUT | 写入原始字节；`offset` 不符时返回 409 与正确值 |
| `/api/uploads/{id}` | DELETE | 取消上传 |
| `/api/upload` | POST | 单次上传，`multipart/form-data` 字段 `file` |
| `/api/files/{id}` | GET | 下载 |

## curl 示例

```bash
HUB=http://192.168.1.10:8787

# 单次上传
curl -F file=@photo.png $HUB/api/upload

# 续传
ID=$(curl -s -X POST -d "{\"name\":\"a.iso\",\"size\":$(stat -c%s a.iso)}" $HUB/api/uploads | jq -r .uploadId)
OFF=$(curl -s $HUB/api/uploads/$ID | jq .offset)
tail -c +$((OFF+1)) a.iso | head -c 8388608 | curl -s -X PUT --data-binary @- "$HUB/api/uploads/$ID?offset=$OFF"

# 启用口令时
curl -c jar -d '{"pin":"2468"}' $HUB/api/auth
curl -b jar -F file=@photo.png $HUB/api/upload
```
