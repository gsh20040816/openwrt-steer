# 测试

`tests/check-package-boundaries.py` 固定 M1 的包所有权：可安全选择的运行依赖必须进入 OpenWrt
标准构建依赖图；存在上游 Kconfig 环的 curl 与 sing-box 仍必须进入 APK 运行依赖元数据。
主包不能安装第三方二进制，GeoSite/GeoIP 只能由 `steer-geodata` 包提供，运行时和 LuCI 不得
恢复联网 updater 或调度器。

`tests/check-build-cache.py` 固定 OpenWrt 官方缓存边界：发布必须使用固定源码、官方
external toolchain、预构建 host tools 和 package ccache，配置 `CONFIG_CCACHE=y`
与一次性构建使用的 `CONFIG_AUTOREMOVE=y`，拒绝重新使用全包 SDK，
按官方顺序安装预构建 tools 与 external toolchain wrapper，构建后显示并清理
ccache 统计，再更新同 key 缓存。工作流不得恢复 target
`build_dir`/`staging_dir`，不得 seed SDK 内部目录、刷新 stamp、写完成 marker，或
重新引入强制 target-cache 命中的自定义状态协议；还必须将 ccache 挂载到 OpenWrt
make 实际解析的源码树 `.ccache`，禁止沿用 SDK 的 `/builder/.ccache`。配置必须清除
x86 固件镜像的默认包，拒绝 target-profile firmware，并在单包构建时覆盖默认
`=y` kmod；包含 kmod 时必须先构建当前配置的 target kernel。构建只提交
`luci-app-steer` 这一个完整依赖闭包。

`tests/ucode/model_test.uc` 直接在 OpenWrt 的 ucode 运行时验证语义模型和编译器。首批回归用例覆盖：

- 最小直连配置；
- 启用规则的悬空引用；
- 保留但不启用的通用规则；
- 托管 zone 缺失；
- Default 顺序；
- DNS Profile 与独立 SmartDNS 实例；
- SmartDNS 业务上游不生成内部 SOCKS，而是经 output TPROXY 进入普通 Rules；
- Bootstrap DNS 使用 IP 字面量和核心绕行 mark，业务上游的独立 mark 只防止 UDP/TCP 53 再劫持；
- 路由器本机 UDP/123 在 output 普通规则标记前固定直连，且不放宽 LAN 客户端 NTP；
- 混合规则分别生成 DNS 可见条件投影和完整 Route 投影；
- 源 MAC 规则生成独立的双栈 DNS/TPROXY 入口，DNS 与 Route 投影保留同一客户端上下文；
- 严格 IPv4/IPv6 字面量与 CIDR 校验，包括压缩 IPv6 和非法冒号序列；
- 仅连接阶段规则不生成空 DNS 规则；
- TPROXY 与 sing-box 绕行 mark 冲突；
- firewall zone 实际设备去重、双栈 DNS/TPROXY 规则和空设备拒绝。

本机尚未安装 ucode，因此该测试必须在目标 OpenWrt 或等价构建容器中执行。仅通过静态检查不能替代目标运行时验证。

`tests/fixtures/representative-valid/steer` 使用完全虚构的名称、凭据、示例域名、文档地址和本地管理 MAC，覆盖 VLESS + Reality + Vision、Hysteria2 端口跳跃、Trojan、双栈客户端规则、源 MAC 规则、UDP 连接阶段规则、两个 DNS Profile、三条按 DNS 上游目标表达的普通规则和一条禁用规则。fixture 不映射任何私人部署配置。

`tests/fixtures/local-proxy-valid/steer` 验证具名 mixed 本地代理入口，以及规则以入口为 DNS 和 Route 的共同匹配维度。该 fixture 只监听回环地址，不映射任何现有服务端口。

`tests/fixtures/geo-rules-valid/` 验证规则内直接填写 GeoSite 分类、从已安装的
`steer-geodata` 包生成本地 `.srs` 引用，以及目标 sing-box 原生配置检查。测试不会运行时联网
下载数据。

真实 geodata 验证还需要目标机安装 `geoview` 与 `steer-geodata` 数据包。验证链为
`.dat` → geoview 严格 JSON → 配套 sing-box 原生 `.srs`，并覆盖 GeoSite/GeoIP 首次
生成、包版本变化后重建，以及分类缺失时保持 `current` 不变。数据版本更新只通过包管理器，
不再经过普通本机 Rules 在线下载。

`tests/fixtures/dangling-reference-invalid/steer` 使用通用示例域名验证悬空引用：`steerctl validate` 必须失败并报告 `DANGLING_OUTBOUND`。

`tests/integration/run-openwrt-vm.sh` 在一次性 OpenWrt x86/64 VM 中执行完整集成检查。环境和安全边界见 `docs/DEVELOPMENT.md`。
它还验证系统 SmartDNS 冲突会在网络接管前被拒绝、失败原因可由 LuCI RPC 读取，并验证应用总开关只同步 Steer 自身的开机启动状态。

`tests/check-luci-i18n.py` 要求简体中文语言包完整覆盖 LuCI JavaScript 文案和菜单标题，并拒绝空翻译、重复项与已经失去来源的旧翻译。PO 格式与占位符还需通过 `msgfmt --check-format --check-header`。

`tests/node/share_url_test.js` 在宿主 Node.js 中加载与 LuCI 共用的纯客户端解析器，覆盖
VLESS + Reality、Hysteria2/`hy2` 官方多端口、兼容 `mport` 和 IPv6、Trojan TLS，以及对 WS、gRPC、ECH、
证书指纹固定、未知参数和语义冲突的显式拒绝。测试链接只包含虚假凭据。

`tests/node/luci_view_test.js` 用最小 LuCI 表单桩加载真实页面模块，锁定零本地代理时的规则
编辑、合并字段的多行存储、当前行 GeoSite/GeoIP 动态补全、零节点时的路由编辑、悬空引用的
可修复显示，以及可选面板不得把 `null` 渲染为页面文本。它是快速回归门禁，不能替代
OpenWrt KVM 中对候选实时过滤、键盘与鼠标选择的真实浏览器检查。

`tests/node/steer_helper_test.js` 锁定总览状态与应用交互：禁用状态不把替代服务误报为错误，
启用失败时显示持久化原因和准确冲突服务，应用 RPC 完成后立即刷新状态卡而不是保留旧页面快照。
