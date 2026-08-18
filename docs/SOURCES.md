# 发布输入与来源

本文件记录预览版软件包的直接构建输入。Steer 不把运行时核心复制进自身仓库；软件包依赖
OpenWrt 上已安装的 sing-box、SmartDNS、firewall4 与内核模块。

## OpenWrt SDK

- 版本与目标：OpenWrt 25.12.5，x86/64；
- 官方 SDK：`openwrt-sdk-25.12.5-x86-64_gcc-14.3.0_musl.Linux-x86_64.tar.zst`；
- 官方 SHA-256：`0c8df0151a1e88feb7c03d694d61f6a18d51872815b7c811d76e2b77504d5e9c`；
- 下载目录：[downloads.openwrt.org/releases/25.12.5/targets/x86/64](https://downloads.openwrt.org/releases/25.12.5/targets/x86/64/)；
- GitHub 构建入口以 [`openwrt/gh-action-sdk@f5813d30`](https://github.com/openwrt/gh-action-sdk/tree/f5813d30eeef3534b58ac7e79c5d8842b6035434) 为上游基线，仓库内只保留适配固定发布包集合、LuCI 简体中文选择和并行编译所需的最小差异；
- Artifact Action：[`actions/upload-artifact@043fb46d`](https://github.com/actions/upload-artifact/tree/043fb46d1a93c77aae656e7c1c64a875d1fc6a0a)，即使用 Node.js 24 的 `v7.0.1`。

构建使用官方 `ghcr.io/openwrt/sdk:x86_64-25.12.5` 容器。固定 SDK 已包含在按摘要锁定的
官方镜像层中，并由 GitHub BuildKit 缓存复用；仓库入口不会再次调用镜像内遗留的下载器，
包编译仍在每次运行中从当前源码重新执行。GitHub cache 只保留与固定 SDK、固定 packages
feed 对应的下载目录和 Go host toolchain 中间产物，不缓存 Steer、geoview 或 LuCI 的最终 APK；
每次运行仍执行包下载哈希检查和编译目标。
Release 同时保存包、SHA-256、构建元数据和 Action 日志；不能仅凭本地工作树生成未留证据的发布包。

本地发布前验证时解析到的容器清单摘要为
`sha256:c8a248ce2411962a89f227db444bf5cea022829b049e6326c7d1032d9762982a`。这个记录用于审计
本地验证环境；最终 Release 必须且只能由 GitHub Actions 的标签工作流生成，不能上传本地
编译产物替代 CI 结果。

## geoview

- 上游：[snowie2000/geoview](https://github.com/snowie2000/geoview)；
- 版本：`0.2.6`；
- 固定源码提交：`3c91926d360b8f49d47520639e574608318baf12`；
- OpenWrt 确定性源码归档 SHA-256：`6a5325e3390e9a9061abedc9bddd196c9182b1827b817471bf8d36d0f9715e45`；
- 许可证：Apache-2.0。

仓库只保留 geoview 的 OpenWrt 打包配方和一项可审查补丁，不复制完整 Go 源码。补丁
`010-list-geosite-attributes.patch` 扩展已有的分类枚举输出：统一返回小写、排序后的 GeoIP
名称，并为 GeoSite 同时列出数据中真实存在的 `名称@属性` 组合，例如
`category-games@cn`。它不修改分类提取或规则转换语义，只为 LuCI 提供合法名称目录。
发布前冷构建确认上游 `0.2.6`
标签的 codeload 归档与上游 Makefile 记录的旧哈希不一致，因此不再把可变标签归档当作构建
输入；配方固定到包含版本更新的精确提交，并由 OpenWrt 下载器生成确定性归档后强制校验
上述哈希。生成的 `.apk` 与 Steer 包一同发布，运行时不能从未锁定的主线下载并替换它。

## GeoSite 与 GeoIP 种子

- 上游：[Loyalsoldier/v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat)；
- release：`202608162214`；
- `geosite.dat` SHA-256：`e0f0f3f07dee391d757174e3645f3284facfe57a1f9932388252e6b4f7a67dab`；
- `geoip.dat` SHA-256：`b406dd3759037188b0674b110dcaf33664a699c0518152d0ca0d9023fc774c6b`。

`steer-geodata` 只提供首次启动种子。后续更新仍必须经过 Steer 的分类转换、原生编译检查
和 last-known-good 事务，远端数据不能定义本地路由动作。

## 设计参考而非复制来源

OpenWrt-momo、HomeProxy、PassWall2 与 OpenClash 用于核对 OpenWrt 生命周期、防火墙边界、
接口变化和故障处理。Steer 不复制 HomeProxy 的 GPL-2.0-only 源码，也不引入 OpenClash 的
任意 YAML/脚本覆盖模型。具体审计提交和结论见 [生产就绪审计](PRODUCTION_READINESS.md)。
