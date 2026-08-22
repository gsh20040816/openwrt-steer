# Steer

Steer 是面向 OpenWrt 的透明代理意图编译器和控制平面。用户通过 UCI 或 LuCI 配置节点、逻辑路由、DNS Profile 与有序规则；Steer 将这些意图严格校验后，确定性地编译为 sing-box 1.13 配置和最小 OpenWrt 网络辅助规则。

当前公开配置版本为 **schema 7**，软件版本线为 **0.3.x**。项目仍以预览版发布，但核心配置、编译、Apply、订阅和诊断路径均有自动化测试。

## 当前能力

- 使用 OpenWrt 原生 UCI、procd、firewall4 与 apk；Steer 本身不常驻监督子进程。
- 普通流量由 sing-box TUN `auto_route` / `auto_redirect` 接管，传统 DNS 与源 MAC 匹配只使用必要的 nftables 辅助规则。
- 支持 Direct、Block 和单节点路由；单节点路由可通过 `detour` 引用另一条单节点路由，组成任意深度的无环前置代理链。
- 同一节点可以被多条路由复用，每条路由拥有独立的前置链；自环、间接环、悬空、禁用或非单节点前置引用都会拒绝 Apply。
- 节点覆盖 SOCKS、HTTP、Shadowsocks、VMess、VLESS、Trojan、Hysteria、ShadowTLS、TUIC、Hysteria2、AnyTLS、SSH、NaiveProxy 和本机 Tor。
- DNS 支持 UDP、TCP、DoT、DoH、DoQ 与 DoH3。每个实际使用的 `(DNS Profile, Route)` 会生成独立 DNS transport，代理路由也会包含完整前置链。
- 规则支持域名、GeoSite、目标 IP、GeoIP、源 CIDR、源 MAC、网络、协议、端口和本地代理入口；非空条件之间为 AND，同一字段内多个值为 OR。
- 订阅接受公开 HTTPS URL，内容可以是逐行标准代理 URI 或整段 Base64 URI 列表；订阅只能管理节点，不能修改路由、DNS 或规则。
- LuCI 支持单 URI 本地导入、订阅更新、节点分组、裸节点测试、逐路由链测试，以及概览页直连、代理、代理速度测试。

## 关键边界

- 只支持 sing-box `>= 1.13.18` 且 `< 1.14.0`，并按当前配置检查 `with_quic`、`with_utls` 等实际 build tags。
- Bootstrap DNS 必须是 IP 字面量并固定直连；远程数据不能携带动作或改变本地策略。
- 启用配置中必须恰好有一个 Direct 路由和一个 Default 规则。
- `probe_direct`、`probe_proxy`、`speedtest_proxy` 都是不可缺失的单个 HTTPS URL。默认值只存在于包内默认 UCI，程序没有隐藏兜底。
- 概览测试通过当前运行规则访问目标 URL，能证明可达性和性能，但不能单独证明连接命中了哪个命名路由。
- 当前没有自动节点故障转移、运行时 outbound 追踪、配置历史或自动启动恢复。后端仍保留一次性 `steer rollback`，LuCI 概览不再提供恢复按钮。

## 文档

- [配置与使用](docs/CONFIGURATION.md)：安装、schema 7、LuCI、CLI、代理链和升级。
- [架构](docs/ARCHITECTURE.md)：数据流、编译不变量、OpenWrt 接管和状态目录。
- [开发与验证](docs/DEVELOPMENT.md)：本地测试、OpenWrt VM、原生校验和发布门。
- [打包与发布](docs/PACKAGING.md)：包所有权、版本、迁移、构建产物和持久状态。
- [测试说明](tests/README.md)：各测试层具体覆盖范围。

## 快速检查

```sh
steer validate
steer plan
steer apply
steer status
steer probe --kind direct
steer probe --kind proxy
steer probe --kind speedtest
```

完整配置示例和节点/路由测试命令见[配置与使用](docs/CONFIGURATION.md)。

## 许可证

GPL-3.0-or-later。第三方组件和数据仍遵循各自许可证，Steer 不复制或 fork sing-box。
