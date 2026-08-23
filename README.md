# Steer

Steer 是一套严格、可解释的透明代理控制面。用户配置节点、逻辑路由、DNS Profile 和有序规则；共享 Go 核心负责 Canonical Intent、校验、编译、Apply 编排、订阅与测试，平台适配器负责网络资源和服务生命周期。

当前稳定版本为 **0.4.3**，当前可用预览版为 **0.5.0-alpha.1**，主线正在准备 **0.5.0-alpha.3**，公开配置为 **schema 7**。OpenWrt 25.12.5 x86/64 是稳定发行平台；Linux systemd 适配器已实现源码基线，覆盖主机及 VM/Docker 公网转发流量，暂不打包；macOS 尚未提供适配器。

## 已实现能力

- 严格 first-match 规则，支持域名、GeoSite、目标 IP、GeoIP、源 CIDR、源 MAC、网络、协议、端口和本地代理入口。
- Direct、Block、单节点逻辑路由；单节点路由可选择另一条单节点路由作为前置代理，任意深度但不得成环。
- SOCKS、HTTP、Shadowsocks、VMess、VLESS、Trojan、Hysteria、ShadowTLS、TUIC、Hysteria2、AnyTLS、SSH、NaiveProxy 和本机 Tor 节点。
- UDP、TCP、DoT、DoH、DoQ、DoH3 DNS Profile；每个实际使用的 `(DNS Profile, Route)` 拥有独立传输路径。
- HTTP(S) 节点订阅、稳定节点 ID、过期节点显式清理。
- 直连、当前代理、当前代理下载测速，以及裸节点和完整路由链测试。
- OpenWrt UCI/LuCI、procd、sing-box TUN `auto_route`/`auto_redirect`、最小 DNS/MAC nftables 辅助层。
- Linux systemd 适配器源码：Canonical JSON、sing-box TUN `auto_route`/`strict_route`/`auto_redirect`、主机与 VM/Docker DNS 的双栈 nft shim、systemd 服务和 loopback Web API/UI。

## 明确边界

- 只支持 sing-box `>= 1.13.18` 且 `< 1.14.0`；使用到 QUIC/uTLS 时会检查对应 build tags。
- Bootstrap DNS 必须是 IP 字面量并固定直连。远程订阅和 Geo 数据不能携带本地动作。
- 启用配置中必须恰好有一个 Direct 路由和一个 Default 规则。
- 三个测试 URL 都是必填 HTTPS scalar option，没有隐藏默认值。
- 没有自动故障转移、配置历史、自动回滚、人工 rollback 命令或运行时 outbound 命中追踪。
- 配置、能力或原生检查失败时直接拒绝；切换运行态后的失败保留现场，不伪装成功，也不自动恢复。

## 文档

- [范围与冻结规则](docs/SCOPE.md)
- [架构](docs/ARCHITECTURE.md)
- [配置与使用](docs/CONFIGURATION.md)
- [开发与验证](docs/DEVELOPMENT.md)
- [打包与发布](docs/PACKAGING.md)
- [测试说明](tests/README.md)
- [Linux 适配器](docs/LINUX.md)

## 公共 CLI

```sh
steer version
steer validate
steer apply
steer health
steer status
steer probe --kind direct
steer subscription status
steer geo-catalog --kind geosite
steer cleanup
```

公共命令固定为 `version validate apply health status probe subscription geo-catalog cleanup`。编译器中间结果和平台计划属于内部接口，不通过 CLI 或 RPC 暴露。

Linux 源码命令为 `steer-linux`，另外提供 loopback Web 控制面：

```sh
steer-linux web-token
steer-linux web
steer-linux subscription status
```

Linux 目前不维护 GitHub Actions 打包或发行版打包脚本；可直接用 `go build ./cmd/steer-linux` 生成本地二进制，并手工安装 `linux/systemd/` 中的 unit。

## 许可证

GPL-3.0-or-later。sing-box、Geo 数据和其他第三方组件继续遵循各自许可证；Steer 不复制或 fork sing-box。
