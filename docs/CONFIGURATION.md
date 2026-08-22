# 配置与使用

当前公开配置是 schema 7。OpenWrt 的 `/etc/config/steer` 是唯一配置真相，LuCI 只编辑这份 UCI；Linux 的 `/etc/steer/config.json` 是严格 Canonical JSON 真相，不读取或迁移 UCI。

## 基本配置

```uci
config steer 'main'
	option schema_version '7'
	option enabled '1'
	option log_level 'warn'
	option dns_cache_capacity '4096'
	option probe_direct 'https://www.baidu.com/'
	option probe_proxy 'https://www.google.com/generate_204'
	option speedtest_proxy 'https://speed.cloudflare.com/__down?bytes=1000000'

config bootstrap 'bootstrap'
	option protocol 'udp'
	option server '1.1.1.1'
	option server_port '53'
	option strategy 'prefer_ipv4'
```

三个测试地址都必须是没有凭据和 fragment 的单个 HTTPS URL：

- `probe_direct`：当前运行配置的直连测试；
- `probe_proxy`：当前代理、裸节点和路由链连接测试；
- `speedtest_proxy`：完整下载测速。

后端不会补默认 URL。Bootstrap 只支持 UDP/TCP，服务器必须是 IP 字面量，策略为 `prefer_ipv4`、`prefer_ipv6`、`ipv4_only` 或 `ipv6_only`。

## 节点、路由和前置代理

规则引用 Route，不直接引用 Node。Route 可以是 `direct`、`block` 或 `single`：

```uci
config node 'front_node'
	option name 'Front'
	option type 'socks'
	option server '192.0.2.10'
	option server_port '1080'

config node 'exit_node'
	option name 'Exit'
	option type 'vless'
	option server 'proxy.example'
	option server_port '443'
	option uuid '00000000-0000-4000-8000-000000000001'
	option tls_server_name 'proxy.example'

config route 'direct'
	option kind 'direct'

config route 'front'
	option kind 'single'
	option node 'front_node'

config route 'exit'
	option kind 'single'
	option node 'exit_node'
	option detour 'front'

config route 'block'
	option kind 'block'
```

`detour` 可留空，表示节点直接拨号；非空时必须引用另一条启用的 single Route。链可以多级，但不能自环或间接成环。Direct/Block 不能携带或充当前置 Route。后端是唯一语义裁决者，Apply 会拒绝完整非法路径。

支持的节点类型：`socks http shadowsocks vmess vless trojan hysteria shadowtls tuic hysteria2 anytls ssh naive tor`。具体字段由协议决定；未知字段、错误字段形态和该协议不支持的选项都会明确失败。

## DNS Profile

```uci
config dns_profile 'secure_dns'
	option protocol 'https'
	option server '1.1.1.1'
	option server_port '443'
	option tls_server_name 'one.one.one.one'
	option path '/dns-query'
	option strategy 'prefer_ipv4'
```

协议支持 `udp tcp tls https quic h3`。规则选择代理 Route 时，DNS transport 使用同一 Route 及其完整前置链。`dns_cache_persist` 和 `dns_optimistic_cache` 为 sing-box 1.14 预留，在当前 1.13 基线上启用会被拒绝。

## 规则

规则严格按 UCI 顺序 first-match。最后必须恰好有一条启用的 Default；Default 之后不能再有启用普通规则。

```uci
config rule 'service'
	option name 'Example service'
	option dns_profile 'secure_dns'
	option route 'exit'
	list domain_match 'domain:example.com'
	list domain_match 'geosite:category-example'
	list network 'tcp'
	list port '443'

config rule 'default'
	option default '1'
	option dns_profile 'secure_dns'
	option route 'direct'
```

条件包括：

- `inbound`：本地 SOCKS/Mixed 入口 ID；
- `domain_match`：普通关键字、`full:`、`domain:`、`regexp:`、`geosite:`；
- `ip_match`：CIDR 或 `geoip:`；
- `source_ip_cidr`、`source_mac_address`；
- `network`、`protocol`、`port`。

同一字段多值为 OR，不同非空字段为 AND。目标 IP、网络、协议和端口只参与业务路由，不参与 DNS 选择。

## 本地入口和订阅

本地入口支持 `socks` 与 `mixed`，监听地址必须是 loopback：

```uci
config local_proxy 'local'
	option protocol 'mixed'
	option listen '127.0.0.1'
	option listen_port '1090'
```

订阅只管理节点：

```uci
config subscription 'public'
	option enabled '1'
	option name 'Public'
	option url 'https://example.com/subscription'
	option update_interval '6h'
```

```sh
steer subscription update --id public
steer subscription status
steer subscription clean --id public --node <node-id>
```

URL 必须是可访问的 HTTP 或 HTTPS 地址，允许私网地址和正常重定向。订阅内容可为逐行标准代理 URI 或整段 Base64 URI 列表。单条无效节点会被跳过并计数；如果没有任何有效节点，更新会提交空节点集并删除该订阅此前生成的节点。非空更新使用稳定 ID，保留本地启用状态；上游消失的节点标为 `pinned_stale`，必须显式 clean。订阅提交节点后不自动 Apply。

## Apply、状态和测试

```sh
steer validate
steer apply
steer health --timeout 10s
steer status
```

`validate` 只做严格解码和共享语义校验。`apply` 还会执行能力检查、Geo 准备、sing-box/nftables 原生检查、切换与本地健康检查。`status` 只包含 `healthy` 和可选 `last_apply`，不会把配置合法性或组件明细混在状态对象中。

概览测试读取当前运行 generation 的 Intent；未 Apply 的 UCI 修改不会参与：

```sh
steer probe --kind direct
steer probe --kind proxy
steer probe --kind speedtest
```

裸节点和路由链测试读取磁盘 UCI并启动临时环回 sing-box，不切换当前运行态：

```sh
steer probe --kind speedtest --node <node-id>
steer probe --kind speedtest --node <node-id> --download
steer probe --kind speedtest --route <route-id>
steer probe --kind speedtest --route <route-id> --download
```

连接测试记录 TCP、TLS、首字节、HTTP 状态和尝试次数；下载测试记录字节和耗时。报告保存在 `/var/lib/steer/logs/tests/{overview,nodes,routes}`。测试证明对应配置下 URL 可达，不提供命名 outbound 的命中回执。

## 版本与升级

0.4 只接受 schema 7，不迁移旧 schema，也不注入缺失字段。安装/升级在 schema 不匹配时 fail-fast。旧版本遗留的 `/var/lib/steer/rollback.uci` 会被删除，因为 rollback 功能和接口已经完全移除。
