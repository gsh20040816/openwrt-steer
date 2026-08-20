# 打包与文件所有权

## 原则

- 软件、第三方二进制和规则数据必须由系统包管理器安装、升级和删除。
- Steer 不在运行时下载文件并覆盖 `/usr`，不从网络替换包管理器拥有的文件。
- 第三方依赖独立成包；Steer 主包只声明依赖，不复制 sing-box、SmartDNS 或 geoview 二进制。
- `/usr` 是只读发布输入，`/etc` 是用户配置，`/var/lib/steer` 是可重建状态，
  `/var/run/steer` 是临时运行状态。
- 生成物不作为包更新输入；删除生成物后必须能从包和配置重新构建。

## 当前 OpenWrt 包

| 包 | 拥有内容 | 不得拥有 |
|---|---|---|
| `steer` | 当前 OpenWrt adapter、UCI、运行事务、编译器和 init 脚本 | 第三方二进制、远端规则数据 |
| `luci-app-steer` | LuCI 页面、RPC 和 ACL | 核心运行时、规则数据 |
| `steer-geodata` | 固定版本、固定哈希的 GeoSite/GeoIP 源数据 | updater、调度器、运行状态 |
| `geoview` | 固定上游源码构建的独立转换工具 | Steer 配置与状态 |
| 发行版 `sing-box` | sing-box 二进制 | Steer 配置与状态 |
| 发行版其他依赖 | firewall4、ucode 等各自文件 | Steer 配置与状态 |

`steer-geodata` 安装到 `/usr/share/steer/geodata-seed`。Steer 可以把其中的数据转换为
`/var/lib/steer/geodata` 下的 sing-box `.srs`，但 GeoSite/GeoIP 版本升级只能通过安装新版
`steer-geodata` 包完成。包升级后，下一次 Apply 根据 package release 重新生成派生规则；如果
新数据缺少被引用分类，Apply 必须失败并保留当前运行代。

## 目标包拆分

Go 核心可执行后，OpenWrt 包将进一步拆成：

- `steer-core`：平台无关的第一方 Go 核心与 CLI；
- `steer-openwrt`：UCI、procd、firewall4 和 OpenWrt 生命周期适配；
- `luci-app-steer`：只依赖 `steer-openwrt`；
- `steer-geodata`：独立版本化数据包。

拆分前不创建无调用方的占位二进制。`steer-core` 只有在接管至少一项真实校验或编译路径并有
回归测试时才进入发布包。

## 发布约束

- OpenWrt Makefile 中可安全选择的运行依赖使用 `DEPENDS`。OpenWrt 25.12 的 `curl/libcurl` 与
  `sing-box/sing-box-tiny` 存在上游 Kconfig 递归选择，暂以标准 `EXTRA_DEPENDS` 写入 APK 依赖
  元数据，但不把其运行变体变成 Steer 的构建选项；不得借此隐藏其他可正常解析的依赖。
- 下载发生在包构建阶段，必须固定版本、源码提交和哈希。
- Release 只发布 CI 从锁定 SDK 构建的独立 APK、SHA-256、元数据和日志。
- 不允许通过安装脚本、LuCI RPC 或后台调度器实现自更新。

## CI 依赖缓存

Release 工作流缓存 OpenWrt host 工具链和 target 依赖的 `build_dir`/
`staging_dir`，但不得把上一次的 Steer 自有包当作本次构建结果。缓存命中后，SDK
必须先通过 OpenWrt 自身的 `package/<name>/clean` 清理 `geoview`、`steer-geodata`、
`steer` 和 `luci-app-steer`，然后才能刷新第三方依赖的构建时间戳。

冷缓存不能直接用空目录覆盖 SDK 的 target 目录。工作流必须先从锁定的 SDK 镜像
复制预置的内核构建树和 target staging 基线，再以该目录继续构建并写入缓存；否则
会丢失官方 SDK 已准备的内核文件，错误地触发残缺的内核重建。

SDK 容器以 `buildbot` 用户构建，而 `actions/cache` 由 runner 用户归档。构建结束后
必须为缓存树补齐只读和目录遍历权限，避免权限不足被 post step 仅记录为 warning、
却没有真正写入缓存。

发布构建只向 OpenWrt 提交一个 `package/luci-app-steer/compile` 顶层目标；其
`LUCI_DEPENDS`/`DEPENDS` 会完整选择 `steer`、`geoview`、`steer-geodata` 和第三方
依赖。不得同时提交四个独立顶层目标，否则共享依赖会被重复展开。用于一次性 CI
检查的 `CONFIG_AUTOREMOVE=y` 也不得启用，因为它会在目标完成后删除 `build_dir`，
使下一次即使恢复缓存仍重新编译 OpenSSL、内核等依赖。

缓存命中时，第三方依赖 stamp 必须在 feed 安装和最终 `make defconfig` 全部完成后
刷新；如果先刷新 stamp 再运行 `defconfig`，新配置的时间戳会立即使恢复的依赖状态
过期。该顺序修复不改变 SDK、feed 或依赖图，应继续复用同一个精确缓存 key。

缓存只允许精确 key 命中，key 锁定 SDK、base/packages/LuCI feed 与当前依赖图；
只要 `DEPENDS`/`LUCI_DEPENDS` 选择、SDK 或 feed 锁定变化，就必须升级 key 中的
`targetdeps-vN`。成功构建会写入完整性标记；下一次精确命中时缺少该标记必须
立即失败。手动工作流的 `require_build_cache_hit=true` 用于强制验证命中；缓存
未精确命中时不允许继续构建。
