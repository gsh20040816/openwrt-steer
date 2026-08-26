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
- 诊断统一容纳 overview/node/route probe 入口或结果、Validate 和日志入口。
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
- 单行测试、批量测试、订阅更新和 stale 清理不得禁用整张表或阻断滚动；只禁用当前正在执行的操作。
- 普通列表只显示“失败”与“详细原因请查看诊断日志”；临时核心名称、outbound ID、命令行和完整后端错误链只能出现在诊断日志。
- Direct 是系统必需且始终启用的固定路由；Reject 是固定类型但可启停。二者均不得显示删除、拖拽排序或类型转换操作，新建路由只能是 Single。Reject 只能编译为 sing-box route/DNS `reject` action，不得生成已废弃的 `type=block` outbound。
- 删除订阅时必须先检查其节点是否被 Route 引用；无引用时订阅与其生成节点必须一起从工作副本移除。
- 节点导入统一使用共享后端解析器，支持多行分享链接和 Base64 包装文档；文案不得声称在浏览器本地解析。

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

## 生成与测试门

`go generate` 生成 LuCI、Linux Web 和 SwiftUI 使用的只读规格。提交必须包含已更新的生成文件。CI 必须检查重新生成后工作树无差异，并验证：

1. 每个可选择节点协议都能由规格构造通过共享校验的代表配置；
2. Canonical 的每个用户字段都被规格覆盖或标为内部/平台不支持；
3. 三端生成文件拥有相同的规格 schema 与 digest；
4. capability 为 true 的操作存在平台后端契约测试；
5. 可见操作不得缺少后端实现；
6. OpenWrt、Linux 和 macOS 的原生 UI 回归测试继续通过。

新增或修改字段时，正确顺序是：Canonical model/Validate → `internal/uispec` → 重新生成 → 三端原生布局接入 → 契约测试。不得先在某一个前端添加字段再补其他平台。
