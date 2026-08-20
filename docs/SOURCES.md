# 发布输入与来源

本文件记录预览版软件包的直接构建输入。Steer 不把第三方运行时核心复制进自身仓库；软件包
依赖 OpenWrt 包管理器安装的 sing-box、SmartDNS、firewall4 与内核模块。SmartDNS 仅属于
当前迁移参考路径，不是目标架构依赖。

## OpenWrt 构建输入

- 版本与目标：OpenWrt 25.12.5，x86/64；
- OpenWrt 源码提交：`f0a60eee2fe051741c643ea6118718aae1ef17fb`；
- 官方 external toolchain 容器摘要：
  `sha256:16fe9150edb39da54b50f5d7e99e9baf0f2c9afb16183c54bfb07bc4e08b3b38`；
- packages feed 提交：`5caa62e0bc9f7fb9b0c12a23267bceb7724214dd`；
- LuCI feed 提交：`128a7812f4be233c5dd7f7466f534fd888785caf`；
- 外部 Go bootstrap：官方 `golang:1.26.4-bookworm`，清单摘要
  `sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b`；
- Artifact Action：[`actions/upload-artifact@043fb46d`](https://github.com/actions/upload-artifact/tree/043fb46d1a93c77aae656e7c1c64a875d1fc6a0a)，即使用 Node.js 24 的 `v7.0.1`。

构建使用固定 OpenWrt 源码、官方预构建 host tools 和 external toolchain，不使用全包 SDK
作为构建根。外部 Go 镜像只提供 OpenWrt 官方配置接口支持的 bootstrap GOROOT；最终 host Go
和 geoview 仍由固定 packages feed 与固定源码构建。GitHub cache 只持久化 OpenWrt `.ccache`，
不保存 `dl`、`build_dir`、`staging_dir`、stamp 或最终 APK。

Release 同时保存包、SHA-256、构建元数据和 Action 日志；最终 Release 必须且只能由
GitHub Actions 的标签工作流生成，不能上传本地产物替代 CI 结果。

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

`steer-geodata` 是 GeoSite/GeoIP 的唯一版本和更新所有者。路由器不再直接查询上游 release
或下载替换数据；升级由包管理器安装新版 `steer-geodata` 完成。Steer 只把包内数据转换为
`/var/lib/steer` 下的受控本地规则集，并在 Apply 中执行原生编译检查和 last-known-good
事务。远端数据不能定义本地路由动作。

## 设计参考而非复制来源

OpenWrt-momo、HomeProxy、PassWall2 与 OpenClash 用于核对 OpenWrt 生命周期、防火墙边界、
接口变化和故障处理。Steer 不复制 HomeProxy 的 GPL-2.0-only 源码，也不引入 OpenClash 的
任意 YAML/脚本覆盖模型。具体审计提交和结论见 [生产就绪审计](PRODUCTION_READINESS.md)。
