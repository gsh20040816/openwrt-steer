# Linux 适配器

Linux 第一版面向 systemd 发行版，覆盖 Linux 主机以及由该主机转发的 VM/Docker 公网流量。主仓库发布平台中立的 x86_64/aarch64 tar.zst，并维护 Arch AUR 源码配方；CI 不构建或发布 deb、rpm、`.pkg.tar.zst` 等发行版二进制包。

## 范围

- 配置：严格 Canonical JSON schema 9 的用户 Intent 位于 `/etc/steer/config.json`；没有第二份 Linux platform settings。
- 数据面：无版本锁定的 sing-box 提供 TUN `auto_route + strict_route + auto_redirect`；Apply 用 native config check 判断当前构建是否支持所用字段，源 MAC 使用被接受时的 `source_mac_address` 原生匹配。
- DNS：sing-box IPv4/IPv6 wildcard DNS inbound 分别监听 1053/1054，加 nftables `OUTPUT`/`PREROUTING` TCP/UDP 53 shim；TUN 显式使用 `dns_mode: disabled`，不会注册 systemd-resolved link DNS 或创建第二套 DNS hijack。`input` 链只允许 DNAT/REDIRECT 后的请求进入 listener，直接访问主机 1053/1054 会被拒绝。VM/Docker 的传统 DNS 请求也进入 Steer。不能把整个 TUN 直接接到 `hijack-dns`，否则 UDP 源端口复用可能把后续非 DNS 流量粘进 DNS 会话。
- 生命周期：systemd `steer.service`，`_run` 完成准备后直接 exec sing-box；`cleanup` 由 `ExecStopPost` 调用。`steer.service` 是 `nftables.service` 的 `PartOf`，正常重启 nftables 时会在其后重启并重建 Steer 数据面。
- 管理：统一 CLI `steer` 和只监听 loopback 的 `steer web`。
- 订阅：systemd timer 更新 JSON 配置，不自动 Apply；更新失败或 HTTP 200 但没有有效节点时保留旧配置。
- Geo：发行归档携带完整的 Loyalsoldier-compatible SRS seed 与 manifest；Steer 精确校验 selector，sing-box 使用 remote rule-set 后台更新。

Linux 第一版明确不提供非 systemd、通用 LAN 网关配置向导、多用户权限分离、远程 Web、NetworkManager/systemd-resolved 深度集成、DIRECT kernel bypass、Clash API、实时连接图和 macOS GUI。Linux TUN 不使用 workstation-only 的接口白名单；主机转发的 VM/Docker 公网流量随主机规则进入代理，私有/链路本地目的地址仍按平台排除规则处理。

## 通用发行产物与安装

GitHub Release 提供：

```text
steer-linux-x86_64.tar.zst
steer-linux-aarch64.tar.zst
```

每个归档包含 `/usr/bin/steer` 所需的单一可执行文件、systemd unit、两个示例配置、许可证和 `geodata-seed/`；不包含 sing-box、geoview、DAT 或发行版安装脚本。以 x86_64 为例：

```sh
tar --zstd -xf steer-linux-x86_64.tar.zst
cd steer-linux-x86_64
sudo install -m 755 steer /usr/bin/steer
sudo install -d -m 700 /etc/steer /var/lib/steer
sudo install -m 600 config.example.json /etc/steer/config.json
sudo install -m 600 web.example.json /etc/steer/web.json
sudo install -d -m 755 /usr/share/steer/geodata-seed
sudo cp -a geodata-seed/. /usr/share/steer/geodata-seed/
sudo install -m 644 systemd/*.service systemd/*.timer /etc/systemd/system/
sudoedit /etc/steer/web.json
sudo steer web-token  # 输出配置中的 token，粘贴到 Web 登录页
systemctl daemon-reload
systemctl enable --now steer.service steer-web.service steer-subscription.timer
```

也可以从 source tag 的 `go/` 目录构建：`CGO_ENABLED=0 go build -trimpath -o ../steer ./cmd/steer-linux`。发行版包必须把它安装为 `/usr/bin/steer`。

实际启用前必须通过系统包管理器安装匹配版本的 sing-box、nftables、iproute2 和 ca-certificates。Geo seed 已在归档内，不再安装 geoview 或另行选择 DAT。随后运行：

```sh
steer validate
steer apply
steer health
```

Web 默认只监听 `127.0.0.1:9080`，远程访问使用 SSH 端口转发；不支持把 Web 绑定到公网地址。

Web 顶部状态条提供 Steer 启用/禁用开关。切换开关会立即保存 `/etc/steer/config.json` 并执行 Apply；Apply 失败会明确提示并保留已保存配置，禁用会清理运行态资源。所有配置页共享 Save、Save and Apply 与 `Apply 已保存配置`；后者不依赖浏览器工作副本的 dirty 状态，并在已保存运行投影待切换时保持可用。

Advanced JSON textarea 是同一个浏览器 Draft 的原文视图，不是第二份编辑缓存。有效 JSON 与结构化页面双向同步；无效 JSON 原文会保留，并阻止 Save 与结构化导航。dirty 时顶部提供“放弃修改”，确认后重新载入 Saved Intent、revision 与 overview，并重绘当前页面；切页不会丢失 Draft，浏览器 reload/关闭继续使用统一的未保存保护。

异步 Save 使用请求时的不可变 Intent 快照和 Draft epoch；请求期间继续编辑不会被旧响应清成 clean。Save、Apply Saved 与 reload 串行互斥。订阅更新或 stale 清理期间若 Draft 发生变化，Web 保留本地 Draft 并提示 inventory 已变化，不自动 reload，也不重绘已经离开的订阅页面。订阅列表区分未抓取、最近成功和最近失败，并持久显示 skipped/stale；已停用订阅的 Update 按钮不可用。

状态条、总览和诊断中的 Active generation/digest 只读取 `/run/steer/current`。最近 Apply 作为带时间、candidate 和错误摘要的独立记录展示；失败 candidate 不会被冒充为 Active。订阅更新只改变未引用节点库存时显示 warning，不制造 pending Apply。

Web Bearer token 的唯一配置源是严格 schema 1 的 `/etc/steer/web.json`。用户直接设置 `token`（32–256 个无空格可见 ASCII 字符）；`steer web-token` 只读取并输出当前配置，不生成、不迁移、不维护第二份 token 文件。

`geo-catalog` 从包内 manifest 返回完整 category/attribute selector。Web“系统”页只显示 seed 版本、规则数量和运行时事实，不再维护无内容的 `/api/v1/platform`。配置编辑器离线加载同一 catalog，未知 selector 在保存/Apply 前失败。

## 运行时路径

```text
/etc/steer/config.json              0600，用户 Canonical Intent
/etc/steer/web.json                 0600，Web bearer token
/usr/share/steer/geodata-seed       只读 SRS seed 与 manifest
/run/steer/current                  当前 generation 链接
/run/steer/generations/<id>/        intent、sing-box、platform、firewall
/run/steer/operation.lock           Apply、配置写入和订阅变更共用锁
/run/steer/last-apply.json          最近 Apply 结果
/var/lib/steer/cache.db             sing-box remote SRS 与可选 DNS cache
/var/lib/steer/subscriptions        订阅 snapshot
```

Linux 适配器不更改 `/etc/resolv.conf`、NetworkManager connection 或 systemd-resolved drop-in。应用若使用 DoT/DoH 上游，传统 53 端口 shim 无法捕获，这属于第一版明确边界。

发布门会在一次性 privileged systemd 容器中运行 `tests/integration/run-linux-system.sh`。测试使用两个独立 netns，覆盖主机与转发流量的 IPv4/IPv6 TCP、UDP、UDP/TCP53、listener 访问限制、禁用/启用、`steer.service` 重启和 `nftables.service` 重启恢复；不会修改开发机或 CI runner 本身的网络规则。
