# 打包与发布

Steer 0.3.0 使用 OpenWrt 官方 25.12.5 x86/64 SDK构建。仓库中的包定义、固定上游版本和 GitHub Actions 共同构成发布输入；Release 只复用同一提交已经成功生成的 master 构建，不重新编译。

## 包与所有权

| 包 | 当前版本 | 职责 |
| --- | --- | --- |
| `steer-openwrt` | `0.3.0-r2` | `/usr/sbin/steer`、默认 UCI、init/procd、升级迁移 |
| `luci-app-steer` | `0.3.0-r2` | LuCI 页面、ucode RPC、ACL |
| `luci-i18n-steer-zh-cn` | `0.3.0-r2` | 简体中文翻译，由 LuCI 构建系统生成 |
| `steer-geodata` | `202608162214-r1` | 固定版本 GeoSite/GeoIP seed |
| `geoview` | `0.2.6-r2` | 提取 Geo 分类的上游工具 |

`steer-openwrt` 依赖发行版的 sing-box，不复制、替换或监督另一个代理核心。其显式平台依赖包括 firewall4、ip、TUN、nft queue/TProxy、CA 证书、`geoview` 和 `steer-geodata`。

## 固定输入

构建必须固定所有远程输入：

- OpenWrt SDK 镜像使用 digest `sha256:c8a248ce2411962a89f227db444bf5cea022829b049e6326c7d1032d9762982a`；
- OpenWrt base、packages 和 LuCI feeds 使用工作流中记录的 commit；
- `geoview` 使用 commit `3c91926d360b8f49d47520639e574608318baf12` 和 `PKG_MIRROR_HASH`；
- GeoSite、GeoIP 使用版本 `202608162214`，两个下载分别由各自 SHA-256 校验。

更新任一输入时，必须同时更新边界检查和发布元数据，不得把浮动分支或未校验下载引入发布构建。

## 配置与持久状态

`/etc/config/steer` 是 conffile，包升级保留用户内容。新装默认配置提供 schema 7 和三个测试 URL；程序不会在现有配置缺项时补默认值。

包拥有或创建的持久路径：

```text
/etc/config/steer
/usr/share/steer/geodata-seed
/var/lib/steer/geodata
/var/lib/steer/subscriptions
/var/lib/steer/logs/tests
/var/lib/steer/rollback.uci
```

`/run/steer` 只存运行代、当前 Plan 和最近 Apply 状态，重启后可以重建。卸载包不会把 `/var/lib/steer` 当作普通包文件强行删除。

## 0.3.0 升级迁移

post-install 只接受 schema 6 或 7。schema 6 升级事务按以下顺序执行：

1. 对 `probe_direct`、`probe_proxy`、`speedtest_proxy` 各读取原 list 第一项；
2. 删除原 list，再以 UCI option 写回非空第一项；
3. 把 `schema_version` 改为 7；
4. 清理 0.2.x LuCI 错写在 node section 的 `_connect_speedtest` 和 `_download_speedtest`；
5. 提交 UCI；
6. 把 `/var/lib/steer/logs/speedtests/<node>` 中的最新报告移动到 `/var/lib/steer/logs/tests/nodes/<node>`；
7. 安装订阅 cron 后执行正常、带原生检查的 `steer apply`。

迁移不为缺失 URL 注入默认值。转换后的配置不合法，或 Apply 失败时，包安装明确失败；不能把错误配置伪装成升级成功。schema 7 重装不重复改写字段，schema 5 及更早版本没有自动迁移窗口。

这次清理旧按钮字段和旧测速目录是一个发布窗口内的迁移代码。所有受支持安装完成升级后，后续版本应删除这段历史兼容逻辑，而不是继续扩展它。

## 官方 SDK 构建

`Build OpenWrt packages` 在 master push 或手动触发时执行：

1. 运行 i18n、包边界、构建缓存、LuCI 和 URI parser 检查；
2. 在固定官方 SDK 容器中安装本地 feed；
3. 只选择 `luci-app-steer` 及其依赖闭包，拒绝无关设备 profile kmod；
4. 串行下载，按 CPU 并行编译；
5. 收集五个且每类恰好一个 APK；
6. 生成 `BUILD-METADATA.txt` 和 `SHA256SUMS`；
7. 上传 `openwrt-25.12.5-x86_64-packages` 构建证据。

本地 Go/Node/Python 测试不能替代 SDK 构建；SDK 编译成功也不能替代真机 Apply 和 sing-box 原生检查。完整门槛见[开发与验证](DEVELOPMENT.md)。

## Tag 与 Release

0.3.0-r2 的预发布 tag 为 `v0.3.0-alpha.2`。推送 tag 前必须确认同一 commit 的 master 构建成功。

`Publish tagged release` 会：

1. 查找 tag commit 对应的成功 master push 构建；
2. 下载该构建的精确 artifact；
3. 执行 `sha256sum -c SHA256SUMS`；
4. 核对 `BUILD-METADATA.txt` 的 source revision；
5. 再次要求五类 APK 各且仅各一个；
6. 创建 GitHub prerelease。

最终 Release 资产包含五个 APK、`BUILD-METADATA.txt` 和 `SHA256SUMS`。SHA-256 只能发现下载损坏或资产被替换，当前流程没有单独的包签名信任链；安装者仍需信任 GitHub 仓库和 Actions 发布权限。

## 真机安装

发布验收必须从 Release 下载资产并先校验：

```sh
sha256sum -c SHA256SUMS
scp ./*.apk root@<router>:/tmp/steer-release/
ssh root@<router> 'apk add --allow-untrusted /tmp/steer-release/*.apk'
```

当前 APK 没有加入路由器信任的仓库签名，所以示例明确使用 `--allow-untrusted`；这不是对校验的替代。安装前备份 `/etc/config/steer`，安装后核对 UCI schema、包版本、服务状态并执行[开发与验证](DEVELOPMENT.md)中的真机发布门。
