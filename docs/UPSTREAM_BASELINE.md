# 第一版上游基线

## 结论

Steer 不从 PassWall2 的配置模型继续打补丁，也不直接换皮 Momo 或 HomeProxy。第一版采用以下边界：

- 运行集成以 OpenWrt-momo 的 procd、firewall4、双栈 TPROXY 和 zone 到实际设备解析方式为主要工程参考；
- Steer 自己实现 UCI 意图模型、引用校验、编译、事务和 LuCI；
- sing-box 与 SmartDNS 保持未修改上游版本；
- PassWall2 只作为公开代码层面的对照来源，不作为运行时输入；
- GeoSite/GeoIP 数据使用
  [Loyalsoldier/v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat)
  release，上游数据定期更新，但由 Steer 转换和事务式发布为本地 `.srs`；
- HomeProxy 只用于核对节点字段与 sing-box 配置形态，不复制其 GPL-2.0-only 代码。

这是一种产品级 fork：继承经过验证的 OpenWrt 集成经验，但不继承上游的原始配置抽象和用户界面。若未来确实复制 Momo 代码，必须保留其 GPL-3.0-or-later 许可证与版权声明，并在来源清单中逐项记录。

## 第一版必须删除的上游抽象

- 整份 sing-box JSON 或远程配置作为运行真相；
- Fake-IP 和围绕 Fake-IP 形成的特殊接管路径；
- 隐式中国大陆绕过、隐藏 fallback 和规则列表交叉覆盖；
- 按私有源地址集合推断“这是 LAN 客户端”的 DNS 接管方式；
- 订阅修改路由动作、DNS 或故障语义的能力；
- 自动忽略不支持节点或悬空引用的成功假象。

## 已冻结的目标版本

第一版开发与实机验证基线：

- OpenWrt 25.12；
- sing-box 1.13.x；
- firewall4 / nftables；
- ucode 作为 OpenWrt 侧配置编译语言。

升到 sing-box 1.14 之前必须单独核对配置迁移，不能因为在线文档已经展示 1.14 字段就提前生成仅在 1.14 可用的配置。

## 当前未继承的能力

- Momo 的订阅整份配置、Dashboard、Fake-IP、China bypass；
- HomeProxy 的全部节点类型、URLTest 和复杂自定义配置；
- PassWall2 的 ACL、规则链、多个独立核心和本地代理端口矩阵。

这些能力只有在产生真实、可验证且能够公开复现的需求后才重新评估。

## GeoSite/GeoIP 上游边界

Steer 选择以下公开发布作为 GeoSite/GeoIP 数据源：

```text
https://api.github.com/repos/Loyalsoldier/v2ray-rules-dat/releases/latest
```

Steer 使用这个 release 的 `geosite.dat` 与 `geoip.dat` 资产，不自行更换分类口径。
兼容性验证发现 `geoview 0.2.6` 直接生成的旧 SRS 不能被配套 sing-box 1.13.x 加载，因此 Steer 使用
同一工具做严格分类提取，但先输出无动作 JSON，再交给配套 sing-box 原生
`rule-set compile`。Steer 自己管理输入校验、生成目录、原子切换、失败保留旧版本和
更新路由，不能依赖其他代理插件的运行目录或生命周期。

LuCI 的 GeoSite/GeoIP 候选不能使用写死列表。Steer 对 geoview 0.2.6 保留一项最小补丁，
让其分类枚举同时返回实际存在的 GeoSite 属性组合并统一排序；规则转换仍走原有严格模式。
用户在普通规则中直接选择分类，内部 `.srs` 名称、去重和生命周期由编译器负责。

首版不自动转换其他代理插件的 Geo 表达式、外部规则集或历史特判。规则集只提供匹配数据，DNS Profile、Route、顺序和 fallback 始终由
Steer UCI 规则决定。

## SmartDNS 与 DoH3 边界

SmartDNS Release 47 才加入 DoQ/DoH3，并要求 OpenSSL 3.4 或更高版本。低于该版本的
SmartDNS 二进制不支持 `server-h3` / `server-http3`。Steer 不能只增加一个
协议下拉项来冒充支持，否则生成的配置会直接导致 DNS 实例启动失败。

当前版本只允许已验证的 UDP、TCP、DoT 和 DoH。DoH3 应作为独立上游升级处理：先为目标
OpenWrt 打包 SmartDNS 47 或更高版本，再验证 QUIC 能力、证书校验、Bootstrap 直连、普通
Rules 路由和失败回滚，最后才扩展 Steer 配置模型。依据见
[SmartDNS Release 47](https://github.com/pymumu/smartdns/releases/tag/Release47) 与
[SmartDNS 配置文档](https://pymumu.github.io/smartdns/en/configuration/)。
