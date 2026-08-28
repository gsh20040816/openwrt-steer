# 分享链接兼容矩阵

Steer 的分享链接和订阅导入只经过 `go/internal/subscription`，再进入 Canonical Intent 校验、UCI/JSON 持久化和 sing-box 编译。三端不实现私有解析器。下表是当前实现的兼容边界；“手动配置”不代表存在可导入的 URI 格式。

| 协议 | 输入格式 | 关键字段（会保留到 Canonical/编译器） | 明确边界 |
| --- | --- | --- | --- |
| SOCKS / SOCKS5 | `socks://`、`socks5://` userinfo | 用户名、密码、服务器、端口 | SOCKS5 归一化为 `socks`；不接受路径 |
| HTTP / HTTPS | `http://`、`https://` userinfo | 用户名、密码、HTTPS TLS 名称/校验 | HTTPS 归一化为带 TLS 的 `http`；仅接受 `sni`、`insecure` |
| Shadowsocks | SIP002 userinfo、userinfo Base64、legacy opaque Base64 | method、密码、服务器、端口、`plugin`/`plugin-opts` | 仅 `obfs-local` 和 `v2ray-plugin`；未知 query Fail-Fast |
| VMess | Base64 JSON（标准或 URL-safe、有/无 padding） | UUID、aid/scy、transport、Host/path、TLS SNI/uTLS/ALPN/校验 | JSON 字段严格白名单；未知字段、非法 Base64/组合拒绝 |
| VLESS | `vless://` userinfo + query | UUID、TLS/Reality、uTLS、ALPN、transport、flow、packet encoding | `security` 仅 `none`/`tls`/`reality`；别名冲突拒绝 |
| Trojan | `trojan://` userinfo + query | 密码、TLS SNI/uTLS/ALPN/校验、transport | 仅 TCP/WS/gRPC/HTTP/QUIC transport |
| Hysteria | `hysteria://` userinfo + query | 认证、上下行带宽、TLS、ALPN、obfs 密码、端口跳跃 | 上下行带宽必填；`obfs` 作为 obfs 密码保留 |
| Hysteria2 | `hysteria2://` userinfo + query | 密码、TLS、ALPN、Salamander、端口跳跃、带宽 | `hy2://` 为同义 scheme；obfs 类型仅 salamander |
| ShadowTLS | `shadowtls://` userinfo + query | 版本、密码、TLS SNI/uTLS/ALPN/校验 | 版本仅 1–3；v2/v3 需要密码 |
| TUIC | `tuic://` UUID[:password] + query | UUID、可选密码、TLS SNI/uTLS/ALPN/校验、拥塞/UDP/心跳 | `allow_insecure`、`allowInsecure`、`insecure` 归一化；密码可为空 |
| AnyTLS | `anytls://` userinfo + query | 密码、TLS SNI/uTLS/ALPN/校验 | `type` 只能为 tcp |
| NaiveProxy | `naive+https://` userinfo + query | 用户名、密码、TLS SNI/uTLS/ALPN/校验、QUIC | QUIC 参数显式校验 |
| SSH | `ssh://` userinfo | 用户名、密码、服务器、端口 | 私钥通过结构化配置提供，不从 URI 传递敏感正文 |
| Tor | 无分享 URI | 仅手动配置的本地 executable/参数 | 文档和 UI 标记为 manual-only，不臆造导入格式 |

通用规则：重复键必须相同；已知别名必须归一化且不能冲突；非法布尔值、空列表项、控制字符、超长 ALPN（超过 255 字节）、未知参数/字段都明确失败。ALPN 使用逗号分隔输入并完整保留为列表。订阅可以是 CRLF 分隔的 URI 文档，也可以是标准/URL-safe Base64 包裹的同一文档；空行和 `#` 注释会跳过。批量导入继续处理其他条目，并返回不含凭据和原始 URL 的 `skipped_reasons` 安全摘要。

代表性矩阵和闭环断言位于 `go/internal/subscription/subscription_test.go`；Canonical JSON、OpenWrt UCI 和 compiler 回归测试确保新增字段不会在中间层丢失。
