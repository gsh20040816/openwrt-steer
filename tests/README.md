# 测试

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
- 仅连接阶段规则不生成空 DNS 规则；
- TPROXY 与 sing-box 绕行 mark 冲突；
- firewall zone 实际设备去重、双栈 DNS/TPROXY 规则和空设备拒绝。

本机尚未安装 ucode，因此该测试必须在目标 OpenWrt 或等价构建容器中执行。仅通过静态检查不能替代目标运行时验证。

`tests/fixtures/representative-valid/steer` 使用完全虚构的名称、凭据、示例域名和文档地址，覆盖 VLESS + Reality + Vision、Hysteria2 端口跳跃、Trojan、双栈客户端规则、UDP 连接阶段规则、两个 DNS Profile、三条按 DNS 上游目标表达的普通规则和一条禁用规则。fixture 不映射任何私人部署配置。

`tests/fixtures/local-proxy-valid/steer` 验证具名 mixed 本地代理入口，以及规则以入口为 DNS 和 Route 的共同匹配维度。该 fixture 只监听回环地址，不映射任何现有服务端口。

`tests/fixtures/geo-rules-valid/` 使用 sing-box 1.13 规则集格式版本 4 的最小源文件，验证规则内直接填写 GeoSite 分类、自动生成本地 `.srs` 引用和目标 sing-box 原生配置检查。测试数据只包含 `example.com`，不依赖联网下载。

真实 geodata 验证还需要目标机安装 `geoview` 与 `steer-geodata` 种子文件。验证链为
`.dat` → geoview 严格 JSON → 配套 sing-box 原生 `.srs`，并覆盖 GeoSite/GeoIP 首次
生成、分类缺失时保持 `current` 不变，以及经普通本机 Rules 下载后的在线事务激活。

`tests/fixtures/dangling-reference-invalid/steer` 使用通用示例域名验证悬空引用：`steerctl validate` 必须失败并报告 `DANGLING_OUTBOUND`。

`tests/integration/run-openwrt-vm.sh` 在一次性 OpenWrt x86/64 VM 中执行完整集成检查。环境和安全边界见 `docs/DEVELOPMENT.md`。

`tests/check-luci-i18n.py` 要求简体中文语言包完整覆盖 LuCI JavaScript 文案和菜单标题，并拒绝空翻译、重复项与已经失去来源的旧翻译。PO 格式与占位符还需通过 `msgfmt --check-format --check-header`。

`tests/node/share_url_test.js` 在宿主 Node.js 中加载与 LuCI 共用的纯客户端解析器，覆盖
VLESS + Reality、Hysteria2/`hy2` 官方多端口、兼容 `mport` 和 IPv6、Trojan TLS，以及对 WS、gRPC、ECH、
证书指纹固定、未知参数和语义冲突的显式拒绝。测试链接只包含虚假凭据。

`tests/node/luci_view_test.js` 用最小 LuCI 表单桩加载真实页面模块，锁定零本地代理时的规则
编辑、合并字段的多行存储、当前行 GeoSite/GeoIP 动态补全、零节点时的路由编辑、悬空引用的
可修复显示，以及可选面板不得把 `null` 渲染为页面文本。它是快速回归门禁，不能替代
OpenWrt KVM 中对候选实时过滤、键盘与鼠标选择的真实浏览器检查。
