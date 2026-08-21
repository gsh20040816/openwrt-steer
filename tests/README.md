# 测试

`tests/check-package-boundaries.py` 固定 M1 的包所有权：可安全选择的运行依赖必须进入 OpenWrt
标准构建依赖图；存在上游 Kconfig 环的 curl 与 sing-box 仍必须进入 APK 运行依赖元数据。
主包不能安装第三方二进制，GeoSite/GeoIP 只能由 `steer-geodata` 包提供。普通 Apply 不得
隐式联网更新订阅；订阅下载由显式命令或每 15 分钟 cron dispatcher 触发，批量写入 UCI 节点并普通提交，等待用户下一次 Apply 才改变运行时。

`tests/check-build-cache.py` 固定 OpenWrt 官方缓存边界：发布必须使用固定源码、官方
external toolchain、预构建 host tools 和 package ccache，配置 `CONFIG_CCACHE=y`
与一次性构建使用的 `CONFIG_AUTOREMOVE=y`，拒绝重新使用全包 SDK，
按官方顺序安装预构建 tools 与 external toolchain wrapper，构建后显示并清理
ccache 统计，再更新同 key 缓存。固定上游版本的下载目录与 Go build cache 也使用跨
Steer 版本的稳定 key，并在默认分支构建后原位轮换；工作流不得恢复 target
`build_dir`/`staging_dir`，不得 seed SDK 内部目录、刷新 stamp、写完成 marker，或
重新引入强制 target-cache 命中的自定义状态协议；还必须将 ccache 挂载到 OpenWrt
make 实际解析的源码树 `.ccache`，禁止沿用 SDK 的 `/builder/.ccache`。配置必须清除
x86 固件镜像的默认包，拒绝 target-profile firmware，并在单包构建时覆盖默认
`=y` kmod；包含 kmod 时必须先构建当前配置的 target kernel。构建只提交
`luci-app-steer` 这一个完整依赖闭包。

`steer-openwrt/src/internal` 的 Go 测试直接验证 schema 6 语义模型、编译器和 OpenWrt adapter。首批回归用例覆盖：

- 最小直连配置；
- 启用规则的悬空引用；
- 保留但不启用的通用规则；
- 托管 zone 缺失；
- Default 顺序；
- DNS Profile 与 Route 组合的独立 sing-box DNS transport/cache；
- Bootstrap DNS IP 字面量、传统 DNS redirect shim 与路由器 output DNS 接管；
- 混合规则分别生成 DNS 可见条件投影和完整 Route 投影；
- 源 MAC 规则生成独立的双栈 DNS/TPROXY 入口，DNS 与 Route 投影保留同一客户端上下文；
- 严格 IPv4/IPv6 字面量与 CIDR 校验，包括压缩 IPv6 和非法冒号序列；
- 仅连接阶段规则不生成空 DNS 规则；
- 全局双栈 DNS/MAC shim、旧策略规则清理和 MAC mark 隔离；
- Apply 串行锁、候选预检、本地健康检查、失败现场、单份健康 UCI 备份与一次性 rollback；
- HTTPS probe 与 Apply 解耦，只作为显式手动诊断。
- sing-box 1.13 代理出站族的强类型校验和编译、标准 URI/Base64 订阅解析、UCI 批量更新与 pinned-stale 保留、临时节点测速指标。

Go 单元测试不能替代目标 OpenWrt 上的 procd、ubus、rpcd、nftables、TUN 和内核模块验证。

`tests/fixtures/m1-representative-valid/steer` 使用完全虚构的名称、凭据、示例域名、文档地址和
本地管理 MAC，覆盖 VLESS + Reality + Vision、Hysteria2、Trojan、六种 DNS transport、源
MAC、具名本地代理、GeoSite/GeoIP 和禁用对象。fixture 不映射任何私人部署配置。配套
`tests/fixtures/m1-geodata/` JSON 在 VM 中编译为 `.srs` 并交给同一 sing-box 原生检查。

真实 geodata 验证还需要目标机安装 `geoview` 与 `steer-geodata` 数据包。验证链为
`.dat` → geoview 严格 JSON → 配套 sing-box 原生 `.srs`，并覆盖 GeoSite/GeoIP 首次
生成、包版本变化后重建，以及分类缺失时保持 `current` 不变。数据版本更新只通过包管理器，
不再经过普通本机 Rules 在线下载。

`tests/integration/run-openwrt-vm.sh` 在一次性 OpenWrt x86/64 VM 中执行完整集成检查。环境和安全边界见 `docs/DEVELOPMENT.md`。
它验证 sing-box 原生检查、双栈 nft/TUN/MAC 接管、DNS、procd respawn、fw4 reload，以及
LuCI 会话化 UCI commit 在第一次提交后恰好触发一个 Apply。它还验证 HTTPS probe 不阻断
Apply、`luci.steer rollback` 不与 rpcd 自死锁、一次性恢复上一健康 UCI，以及禁用后 commit
仍能自动重新启用服务。

`tests/check-luci-i18n.py` 要求简体中文语言包完整覆盖 LuCI JavaScript 文案和菜单标题，并拒绝空翻译、重复项与已经失去来源的旧翻译。PO 格式与占位符还需通过 `msgfmt --check-format --check-header`。

`tests/node/share_url_test.js` 在宿主 Node.js 中加载与 LuCI 共用的纯客户端解析器，覆盖
VLESS + Reality、Hysteria2/`hy2` 官方多端口、兼容 `mport` 和 IPv6、Trojan TLS，以及对 WS、gRPC、ECH、
证书指纹固定、未知参数和语义冲突的显式拒绝。测试链接只包含虚假凭据。

`tests/node/luci_view_test.js` 用最小 LuCI 表单桩加载真实页面模块，锁定零本地代理时的规则
编辑、合并字段的多行存储、当前行 GeoSite/GeoIP 动态补全、零节点时的路由编辑、悬空引用的
可修复显示，以及可选面板不得把 `null` 渲染为页面文本。它是快速回归门禁，不能替代
OpenWrt KVM 中对候选实时过滤、键盘与鼠标选择的真实浏览器检查。

`tests/node/steer_helper_test.js` 锁定总览状态与应用交互：LuCI 先保存、只 commit 一次，明确
禁止不可等待的 `ui.changes.apply()` 和第二个 Steer Apply；前端通过字符串 sequence 等待 procd
结果。存在备份时显示确认恢复按钮，成功后重新载入已恢复 UCI。
