# 原生前端与共享控制面开发约束

Steer 保留 OpenWrt LuCI、Linux Web 和 macOS SwiftUI 三种平台原生前端。三端可以采用不同的导航、布局、控件、鉴权与系统集成，但不得分别维护协议字段、配置约束、引用规则、操作结果或能力声明。前端差异只能来自平台呈现和明确的平台 capability，不能来自三份手工复制的产品规则。

## 分层

```text
Canonical Intent / Validate / Compiler
                  │
       shared UI specification
                  │ build-time generation
        ┌─────────┼──────────┐
        ↓         ↓          ↓
      LuCI     Linux Web   SwiftUI
        │         │          │
      ubus      HTTP      helper/socket
        └─────────┼──────────┘
                  ↓
          shared control contracts
                  ↓
       platform lifecycle adapters
```

职责必须保持如下边界：

- `intent` 和平台 `Validate` 是配置合法性的最终权威；UI 不复制第二套完整校验器。
- `internal/uispec` 是用户可编辑字段、控件类型、枚举、条件显示、敏感属性和平台 capability 的唯一 UI 规格源。
- 生成的 JavaScript/Swift 文件是构建产物，不得手工修改。修改必须从 `internal/uispec` 开始并重新生成。
- 平台 UI 只负责原生呈现、工作副本交互和调用平台 transport；不得决定 Apply、订阅合并、探测或 Geo 合法性。
- 平台 transport 可以不同，但同一操作必须使用相同的请求、结果和错误语义。

## 必须共享的功能规格

以下内容不得再分别硬编码：

1. 节点协议列表、显示名、字段、默认值、必填条件、枚举、TLS/transport 能力和切换协议时允许保留的字段；
2. Bootstrap、DNS Profile、本地代理和规则协议枚举；
3. 规则匹配字段、嗅探协议、Default 固定语义和 Geo 表达式前缀；
4. Direct/Reject/single Route 类型与 detour 约束；Reject 为兼容已有配置仍使用 `kind=block`；
5. collection 的稳定 ID 类型、只读来源、排序能力和删除引用策略；
6. capability 名称及不支持原因；
7. `saved`、`applied`、`revision`、`validation`、`status` 和结构化错误结果。

`id_policy` 规定常规对象由前端自动生成稳定 ID，`name` 只作为可选显示名；LuCI 仍使用 named UCI section，但 Add 不再要求用户发明 section ID。`creation_defaults` 与 `creation_required_fields` 规定新建对象必须真实写入 Draft/UCI 的字段，不能只靠控件 fallback 制造 phantom default。当前统一产品默认是 SOCKS/1080 节点、UDP/53 DNS Profile 和仅监听 loopback 的 Mixed/1090 本地入口；用户已有或显式修改的值不得被重新物化覆盖。同名引用只在发生歧义时追加 endpoint、来源和短 ID，普通名称保持简洁。

生成规格只能描述用户语义，不得包含 UCI section、systemd unit、launchd label、HTTP 路径、Unix socket 或 CSS/SwiftUI 布局。

## 原生呈现保留项

- LuCI 继续使用 `form.Map`、`GridSection`、UCI pending changes、LuCI session 与 ACL。
- Linux 继续使用 loopback Web、Bearer token、ETag 和浏览器可访问性控件。
- macOS 继续使用 `NavigationSplitView`、`Table`、`Form`、系统管理员授权和原生系统组件安装页。
- 三端自行决定字段如何分 tab/section、列表密度、弹窗、拖拽和平台帮助文本。
- CSS、SF Symbols、LuCI theme 和平台 shell 不做源码统一。

## 统一信息架构

三端必须保持相同的页面职责和默认顺序。Linux 与 macOS 可以显示分组标题；LuCI 必须把分组层折叠为 Steer 下的一层页面菜单，避免主题导航、产品分组和表单 tab 叠成三层。用户不能因为换平台而重新学习功能位于哪里。

```text
状态
  └─ 总览
配置
  ├─ 基础设置
  ├─ 节点
  ├─ 路由
  ├─ DNS Profile
  ├─ 本地代理
  └─ 规则
服务
  ├─ 订阅
  ├─ 诊断
  └─ 系统
高级
  └─ 高级配置 / Canonical 预览
```

- 总览只展示运行状态、执行模型、规模摘要和快捷操作，不承载长期配置表单。
- 基础设置只编辑 Main、探测目标、DNS cache 和 Bootstrap。
- 节点与路由必须是独立页面；订阅状态和更新不能塞入节点页。
- 诊断统一容纳 overview/node/route 的完整 sanitized probe 报告、Validate、最近 Apply 和相关日志，并提供 Refresh。
- 系统统一展示版本、schema、generation、Geo seed、平台路径和平台特有的安装/升级能力。
- 高级页在 JSON 平台提供 Canonical 编辑，在 OpenWrt 提供 Canonical 只读预览；它不能成为结构化 UI 缺字段的常规兜底。
- 平台特有能力放在最接近的共享页面内，不得创建只在单一平台存在的顶层导航层级。

生成规格必须同时输出这份导航元数据；三端可以翻译标题或使用不同图标。Linux/macOS 的分组、三端页面 key 与顺序必须通过契约测试保持一致；LuCI 的菜单契约是按生成分组顺序展开后的单层叶子列表，不得再为分组创建可访问 URL。

## 功能基线与 capability

三端必须实现的共享基线：

- 状态、启用/禁用、Validate、Save、Apply 以及 Apply 部分成功的真实反馈；
- Main、Bootstrap、节点、路由、DNS Profile、本地代理、规则和订阅的结构化编辑；
- 所有当前 Canonical 节点协议的可完成原生表单；
- 节点导入、Geo catalog、订阅状态/更新/stale 清理；
- direct/proxy/download、单节点、批量节点和 Route chain 探测；
- generation、last Apply、Steer/sing-box/schema/Geo 运行事实；
- revision conflict 或与平台单写者模型等价的明确保护。

平台差异必须通过 capability 明确表达：

- macOS 不支持 `source_mac_address`，UI 必须说明原因，不能伪造支持；
- Canonical JSON 原文编辑只适用于 JSON 真相源；OpenWrt 提供 Canonical 只读预览，UCI 仍是唯一真相；
- macOS 系统组件安装/升级只属于 macOS；
- macOS 的私网 Do53 捕获使用静态平台 plan：RFC1918、CGNAT 与 ULA 固定按“目标端口 53 劫持 → 私网 Direct → sniff/用户规则”排序，接口/DHCP 变化不触发 Apply；
- LuCI ACL、Linux Bearer token 和 macOS peer credential 是 transport 权限，不进入共享 Intent；
- 日志来源和订阅调度器属于平台实现，但用户操作和结果合同保持一致。

UI 不得显示后端未声明的操作。不可用能力应隐藏或禁用并给出稳定原因；不得用一段说明文字冒充已实现功能。

## 列表与操作一致性

- 节点必须按“手动节点 / 订阅”分组；订阅节点只读，不得显示可用但无效的编辑、删除或状态开关。
- 节点行和 Single Route 行必须同时提供连接测试与下载测速；批量测试只针对当前可见分组中已启用的节点。
- 单行测试、批量测试、订阅更新和 stale 清理不得禁用整张表或阻断滚动；只禁用当前正在执行的操作。disabled 对象不得提供后端必拒绝的动作；LuCI 存在 pending UCI 时不得用 committed Node/Route 执行测试。
- 普通列表只显示“失败”与“详细原因请查看诊断日志”。Diagnostics 展示 tested_at、scope/object、URL、TCP/TLS/TTFB/HTTP/bytes/rate 中实际存在的安全字段；临时核心名称、outbound ID、命令行和完整后端错误链不得进入报告 UI。
- Direct 是系统必需且始终启用的固定路由；Reject 是固定类型但可启停。二者均不得显示删除、拖拽排序或类型转换操作，新建路由只能是 Single。Reject 只能编译为 sing-box route/DNS `reject` action，不得生成已废弃的 `type=block` outbound。
- 删除订阅时必须先检查其节点是否被 Route 引用；无引用时订阅与其生成节点必须一起从工作副本移除。
- 节点导入统一使用共享后端解析器，支持多行分享链接和 Base64 包装文档；文案不得声称在浏览器本地解析。
- LuCI 批量节点导入在写入 pending UCI 前必须逐项展示名称、协议、endpoint、TLS 校验状态与真实的凭据存在性；凭据内容不得进入预览 DOM，Cancel 不得创建任何 section。JSON boolean 与 UCI `"1"` 必须使用同一 flag normalization。
- LuCI Named `GridSection` 必须保留原生 provisional section → editor modal → Save/Cancel 生命周期。ID 按共享策略自动生成，共享默认值在原生 `data.add()` 边界注入；Add 后立即编辑，Cancel 删除 provisional section，不能先 `map.save()` 留下空 pending row。
- `collection_references` 是三端删除保护的唯一关系表：Node ← Route.node、Route ← Rule.route/Route.detour、DNS Profile ← Rule.dns_profile、Local Proxy ← Rule.inbound。删除前必须列出引用并能跳转；订阅与 stale 节点复用 Node 引用保护，后端 Validate 仍是最终防线，不做级联删除。
- Rule 摘要必须覆盖全部 `rule_match_fields`。`rule_connection_only_fields` 明确 `ip_match/network/protocol/port` 不参与 DNS Profile 选择；只有这些条件时三端都显示“DNS 继续匹配后续规则”。macOS 必须显示共享 capability 中 Source MAC 不可用的稳定原因，不能静默隐藏。

## 敏感数据

- 分享 URL、节点密码、私钥、Web token 和订阅正文不得进入日志。
- 敏感正文不得通过进程参数传递；只允许请求正文、标准输入、权限受限临时文件或已鉴权本地 socket。
- 导入预览默认隐藏凭据；用户确认后才能进入工作副本。
- 生成规格只能标记字段为 sensitive，不能携带任何配置值。

## 操作结果

所有写操作必须区分持久化与运行态切换：

```json
{
  "saved": true,
  "applied": false,
  "revision": "sha256-…",
  "validation": { "ok": true, "errors": [], "warnings": [] },
  "apply_result": { "ok": false, "error": "…" }
}
```

如果配置已经保存而 Apply 失败，UI 必须明确报告部分成功，并把工作副本 revision 更新到磁盘事实。不得恢复成“未保存”状态，也不得暗示旧配置仍是持久化真相。

Linux Web 将 Draft、Saved 与 Active 分开显示：`dirty` 只表示浏览器工作副本，revision 属于已保存配置，Active generation/digest 只来自 `/run/steer/current`。Save 后即使 Draft 已 clean，只要已保存配置的编译运行投影与 Active 不同，或最近一次同投影 Apply 失败，全局 `Apply 已保存配置` 仍保持可用。订阅刷新产生但未被 Route 引用的节点库存不进入该运行投影，只显示库存 warning，不制造 pending Apply。

Linux Advanced JSON 与结构化页面必须共享同一个 Draft。textarea 输入立即进入 store 并触发 dirty；语法无效时保留原文、阻止 Save 和结构化导航，不能退回一份旧的解析对象冒充当前表单。顶部与 Advanced 页内的 Save / Save and Apply 调用同一动作。dirty 时全局必须提供带确认的“放弃修改”，确认后通过唯一 reload 路径同步 Intent、JSON 文本、revision、overview 与当前页面；取消不得改变 Draft。

Linux store 为每次 Draft mutation 分配递增 epoch。Save 必须发送不可变快照；响应只允许清理与请求 epoch 相同的 Draft，期间新增修改继续保持 dirty。Save、Apply Saved 与 reload 互斥，重复点击或乱序响应不得覆盖较新的 Intent/revision。订阅 update/clean 同样记录开始 epoch；若请求期间 Draft 变化，只刷新 inventory 提示，不自动 reload，也不得让离页后的旧 render 覆盖当前路由。

Save/Apply 写结果必须携带完整 `validation.errors/warnings`（包括 `code/object_type/object_id/option/message`）。三端结果面板绑定当前 Draft epoch；Draft 改变立即丢弃旧“通过/失败”。问题动作必须打开对应对象并定位字段，错误消息不得回显凭据值。

Linux 页面可见时低频刷新服务器 Saved revision 与 Active status。外部 revision 与 Draft 基线不同时只能设置冲突事实：dirty Draft 不得被替换，clean Draft 由用户显式一键 reload。显式 Refresh、周期刷新和 Save 后 overview 刷新必须走同一比较语义。

最近 Apply 是独立的操作记录。其 candidate、时间、成功/失败和错误摘要必须持久显示，但 candidate 不得作为 Active generation 的兜底来源。

macOS Load 必须同时返回 Saved revision，Save/Apply 必须携带 `expected_revision`。revision conflict 不得修改 Saved、Active 或本地 Draft；UI 必须提供 Reload Saved、保留本地 Draft 和显式覆盖。订阅手动更新完成时只能在 Draft 未发生变化的情况下自动 reload，否则保留 Draft 并进入同一冲突选择；订阅库存变更始终不自动 Apply。

macOS 每个 App 生命周期只初始化一次 Draft。所有页面与菜单栏共用 Save、Apply Saved、Save and Apply；Apply Saved 只部署磁盘 Saved，不得夹带 dirty Draft。Reload、安装/Repair 和退出等会替换或结束 Draft 的动作必须走同一个 Save / Discard / Cancel guard，dirty 时 Enable 必须禁用或显式确认全部副作用。安装完成默认保留编辑中的 Draft，Apply 失败时 Active 只显示后端实际 generation。

LuCI Overview 必须从当前 rpcd session 的 candidate、committed UCI 与 `/run/steer/current` 分别构造 Pending desired、Saved 与 Active。pending disable 不能隐藏仍运行的 Active generation；失败 Apply 在无 pending UCI 时必须提供 Apply Saved 重试。`pending_apply` 比较编译运行投影，不能由全文 Intent digest 推导，因此未引用的订阅节点库存变化只显示 inventory warning。

## 生成与测试门

`go generate` 生成 LuCI、Linux Web 和 SwiftUI 使用的只读规格。提交必须包含已更新的生成文件。CI 必须检查重新生成后工作树无差异，并验证：

1. 每个可选择节点协议都能由规格构造通过共享校验的代表配置；
2. Canonical 的每个用户字段都被规格覆盖或标为内部/平台不支持；
3. 三端生成文件拥有相同的规格 schema 与 digest；
4. capability 为 true 的操作存在平台后端契约测试；
5. 可见操作不得缺少后端实现；
6. OpenWrt、Linux 和 macOS 的原生 UI 回归测试继续通过。

新增或修改字段时，正确顺序是：Canonical model/Validate → `internal/uispec` → 重新生成 → 三端原生布局接入 → 契约测试。不得先在某一个前端添加字段再补其他平台。
