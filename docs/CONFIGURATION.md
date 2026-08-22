# 配置与使用

本文描述 Steer 0.3.x 的公开 schema 7。UCI 是唯一配置真相；LuCI 只编辑同一份 `/etc/config/steer`，不会维护第二套模型。

## 安装与入口

发布资产面向 OpenWrt 25.12.5 x86/64，包含 `steer-openwrt`、`steer-geodata`、`geoview`、`luci-app-steer` 和简体中文语言包。安装后 LuCI 入口位于“服务 → Steer”，CLI 为 `/usr/sbin/steer`。

首次配置建议按以下顺序：

1. 在“概览”确认 Bootstrap DNS 和三个测试 URL；
2. 在“节点与路由”导入或创建节点，再创建逻辑路由；
3. 在“DNS”创建业务 DNS Profile；
4. 在“规则”从上到下配置条件，最后设置固定 Default；
5. 保存并 Apply，确认概览状态健康后执行手动测试。

## 主配置与测试 URL

```uci
config steer 'main'
	option schema_version '7'
	option enabled '1'
	option log_level 'warn'
	option dns_cache_capacity '4096'
	option probe_direct 'https://www.baidu.com/'
	option probe_proxy 'https://www.google.com/generate_204'
	option speedtest_proxy 'https://speed.cloudflare.com/__down?bytes=1000000'
```

三个测试地址都必须是无凭据、无 fragment 的 HTTPS URL，并且只能各配置一个：

- `probe_direct`：概览“直连测试”使用；
- `probe_proxy`：概览“代理测试”、裸节点连接测试和路由链连接测试使用；
- `speedtest_proxy`：概览、节点和路由链的完整下载测速使用。

删除任意一项都会使配置校验失败。默认 URL 只由新装包的默认 UCI 提供，后端不会在字段缺失时偷偷替换。

## Bootstrap DNS

```uci
config bootstrap 'bootstrap'
	option protocol 'udp'
	option server '1.1.1.1'
	option server_port '53'
	option strategy 'prefer_ipv4'
```

Bootstrap 服务器必须使用 IP 字面量。协议为 `udp` 或 `tcp`，地址策略可选 `prefer_ipv4`、`prefer_ipv6`、`ipv4_only`、`ipv6_only`。

## 节点与路由

节点只描述代理协议和认证参数，规则不会直接引用节点。规则引用稳定的逻辑路由，路由再选择节点。

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
	option name 'Direct'
	option kind 'direct'

config route 'front'
	option name 'Front route'
	option kind 'single'
	option node 'front_node'

config route 'exit'
	option name 'Exit through front'
	option kind 'single'
	option node 'exit_node'
	option detour 'front'

config route 'block'
	option name 'Block'
	option kind 'block'
```

`route.detour` 只允许出现在启用的 `single` 路由上，目标也必须是另一条启用的 `single` 路由。链可以继续引用下一条前置路由，但不能形成环。LuCI 会显示所有单节点候选，包括会形成环的候选；保存后由同一后端校验器给出完整环路径，不在浏览器里复制一套不完整判断。

同一节点可以被多条路由引用。例如可同时创建“Exit 直出”和“Exit 经 Front”，两条路由会编译为独立 sing-box 出站，不共享 `detour`。

## DNS Profile

```uci
config dns_profile 'secure_dns'
	option name 'Secure DNS'
	option protocol 'https'
	option server '1.1.1.1'
	option server_port '443'
	option tls_server_name 'one.one.one.one'
	option path '/dns-query'
	option strategy 'prefer_ipv4'
```

协议支持 `udp`、`tcp`、`tls`、`https`、`quic`、`h3`。当一条规则同时选择 DNS Profile 和代理路由时，DNS transport 也固定通过该路由及其完整前置链。

持久 DNS cache 和 optimistic cache 字段为 sing-box 1.14 预留；在当前 1.13 基线上启用会明确失败。

## 规则

规则严格从上到下匹配，第一条命中后停止。Default 固定在普通规则之后，只能选择 DNS Profile 和 Route。

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

支持的条件字段：

- `inbound`：本地 SOCKS/Mixed 入口 ID；
- `domain_match`：普通关键字、`full:`、`domain:`、`regexp:`、`geosite:`；
- `ip_match`：CIDR 或 `geoip:`；
- `source_ip_cidr`、`source_mac_address`；
- `network`、`protocol`、`port`。

同一字段内多值为 OR，不同非空字段之间为 AND。源 MAC 由当前 sing-box 1.13 所需的 nftables/TProxy 辅助路径实现，不会解析成邻居表中的临时 IP。

## 订阅

```uci
config subscription 'public'
	option enabled '1'
	option name 'Public'
	option url 'https://example.com/subscription'
	option update_interval '6h'
```

订阅 URL 必须是公开 HTTPS 地址。更新命令只提交节点变化，不自动 Apply：

```sh
steer subscription update --id public
steer subscription status
steer subscription clean --id public --node <node-id>
```

订阅节点使用稳定 ID。上游删除但仍需人工确认的节点会保留并标记 `pinned_stale`，只有显式 clean 才删除。

## 手动测试

概览页三个按钮读取 `/run/steer/current/plan.json`，因此只测试当前真正运行的配置，未 Apply 的 UCI 修改不会参与：

```sh
steer probe --kind direct
steer probe --kind proxy
steer probe --kind speedtest
```

节点和路由测试读取当前磁盘 UCI，临时启动只监听环回地址的 sing-box，不切换运行态：

```sh
# 裸节点：不包含任何 route.detour
steer probe --kind speedtest --node <node-id>
steer probe --kind speedtest --node <node-id> --download

# 路由：包含完整 detour 链
steer probe --kind speedtest --route <route-id>
steer probe --kind speedtest --route <route-id> --download
```

连接测试超时 10 秒，失败重试一次；完整下载超时 30 秒且不重试。报告包含 HTTP 状态、连接/TLS/首字节延迟，或下载字节、耗时和 Mbps。最新报告保存在：

```text
/var/lib/steer/logs/tests/overview/<direct|proxy|speedtest>.json
/var/lib/steer/logs/tests/nodes/<node-id>/<connect|download>.json
/var/lib/steer/logs/tests/routes/<route-id>/<connect|download>.json
```

测试只能证明目标 URL 在相应配置下可达。概览测试没有 sing-box outbound 命中回执，不能把“可达”解释为对某个命名路由的独立证明。

## 常用 CLI

```sh
steer validate
steer compile
steer compile-sing-box
steer compile-firewall
steer plan
steer capabilities
steer prepare
steer apply
steer health
steer status
steer cleanup
```

`steer rollback` 仍可一次性恢复 Apply 前保存的上一份本地健康 UCI，并复用正常 Apply。LuCI 概览不提供此按钮；它不是配置历史、自动回退或开机恢复机制。

## 配置版本要求

0.3.0 正式版只接受 schema 7，不自动转换旧配置。新安装直接使用包内默认配置；旧版本升级前必须先把 UCI 转换为 schema 7，并确保三个测试 URL 都是非空的 HTTPS scalar option。版本不匹配或配置非法时，安装会明确失败，不会注入默认值或静默修改用户配置。
