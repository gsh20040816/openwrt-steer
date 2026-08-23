# Linux 适配器

Linux 第一版是面向 systemd 发行版的源码适配器，覆盖 Linux 主机以及由该主机转发的 VM/Docker 公网流量；当前目标是验证 Steer 的跨平台语义，不维护发行版包。

## 范围

- 配置：严格 Canonical JSON schema 7 的用户 Intent 位于 `/etc/steer/config.json`；严格 schema 1 的 Linux 平台设置位于 `/etc/steer/platform.json`。
- 数据面：sing-box `>=1.13.18,<1.14.0`、TUN `auto_route + strict_route + auto_redirect`。
- DNS：sing-box 双栈 loopback DNS inbound，加 nftables `OUTPUT`/`PREROUTING` TCP/UDP 53 shim；VM/Docker 的传统 DNS 请求也进入 Steer。
- 生命周期：systemd `steer.service`，`_run` 完成准备后直接 exec sing-box；`cleanup` 由 `ExecStopPost` 调用。
- 管理：CLI `steer-linux` 和只监听 loopback 的 `steer-linux web`。
- 订阅：systemd timer 更新 JSON 配置，不自动 Apply；更新失败或 HTTP 200 但没有有效节点时保留旧配置。
- Geo：用户只在平台设置中选择 GeoSite/GeoIP 数据库绝对路径；仅当 Intent 引用对应 kind 时才要求文件存在。Steer 内部按文件内容 SHA-256 派生并复用 SRS，不要求特定 provider 或 release marker。

Linux 第一版明确不提供非 systemd、通用 LAN 网关配置向导、源 MAC、多用户权限分离、远程 Web、NetworkManager/systemd-resolved 深度集成、DIRECT kernel bypass、Clash API、实时连接图和 macOS GUI。Linux TUN 不使用 workstation-only 的接口白名单；主机转发的 VM/Docker 公网流量随主机规则进入代理，私有/链路本地目的地址仍按平台排除规则处理。

## 手工构建与安装

仓库暂不提供 Linux 打包脚本或 GitHub Actions。可在 Linux 主机上执行：

```sh
cd go
go build -trimpath -o /tmp/steer-linux ./cmd/steer-linux
sudo install -m 755 /tmp/steer-linux /usr/bin/steer-linux
sudo install -d -m 700 /etc/steer /var/lib/steer
sudo install -m 600 ../linux/config.json /etc/steer/config.json
sudo install -m 600 ../linux/platform.json /etc/steer/platform.json
sudo install -m 644 ../linux/systemd/*.service ../linux/systemd/*.timer /etc/systemd/system/
sudo steer-linux web-token
systemctl daemon-reload
systemctl enable --now steer.service steer-web.service steer-subscription.timer
```

实际启用前必须安装匹配版本的 sing-box、nftables、iproute2、ca-certificates 和 geoview。只有使用 `geosite:`/`geoip:` 规则时才需要相应数据库；路径可在 Web“系统设置”或 `/etc/steer/platform.json` 中配置。随后运行：

```sh
steer-linux validate
steer-linux apply
steer-linux health
```

Web 默认只监听 `127.0.0.1:9080`，远程访问使用 SSH 端口转发；不支持把 Web 绑定到公网地址。

`apply`、`_run`、`web` 和 `geo-catalog` 统一接受 `--platform`，默认读取 `/etc/steer/platform.json`。Web 的 `/api/v1/platform` 使用独立 ETag；“系统设置”保存两个路径后立即 Apply。路径或 category 不可用时，新设置仍被保存并返回结构化 Geo 错误，当前运行 generation 不切换。配置编辑器从当前数据库加载 Geo category 动态补全；catalog 可用时拒绝未知名称，catalog 不可用时由 Apply 做最终判定。

## 运行时路径

```text
/etc/steer/config.json              0600，用户 Canonical Intent
/etc/steer/platform.json            0600，Linux-only Geo 数据路径
/run/steer/current                  当前 generation 链接
/run/steer/generations/<id>/        intent、sing-box、platform、firewall
/run/steer/operation.lock           Apply、配置写入和订阅变更共用锁
/run/steer/last-apply.json          最近 Apply 结果
/var/lib/steer/geodata              Geo 派生 generation
/var/lib/steer/subscriptions        订阅 snapshot
/var/lib/steer/web.token            0600，Web bearer token
```

Linux 适配器不更改 `/etc/resolv.conf`、NetworkManager connection 或 systemd-resolved drop-in。应用若使用 DoT/DoH 上游，传统 53 端口 shim 无法捕获，这属于第一版明确边界。
