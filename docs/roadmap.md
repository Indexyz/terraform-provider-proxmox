# Roadmap

## 已完成

- 扫描项目结构、Provider 配置、HTTP client、资源、数据源、QEMU VM typed/raw 映射、文档生成和 CI/e2e 工具链。
- 新增 `docs/codebase.md`，整理开发者向代码库说明、API surface、资源/数据源职责、QEMU 扩展边界、测试与 CI 入口、已有 spec/plan 归档，并记录 `AGENTS.md` 中的贡献约束。
- 确认现有 `docs/` schema/reference 文档由 `tfplugindocs` 生成，3 个资源和 12 个数据源均有示例来源。
- 为 QEMU VM `protection` typed 字段新增预期失败的 client、mapping、raw conflict 与 schema 属性单元测试。
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU `protection` typed boolean，覆盖 schema、client decode/encode、state/request mapping、raw 冲突校验、测试、示例和生成文档；`raw.extra_config["protection"]` 迁移到 typed 字段。
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU `scsihw` typed 字段，覆盖 schema、client decode/encode、state/request mapping、raw 冲突校验、测试、示例和生成文档；`raw.extra_config["scsihw"]` 迁移到 typed 字段。
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU `tablet` typed 字段，覆盖 schema、client decode/encode、state/request mapping、raw 冲突校验、测试、示例和生成文档；`raw.extra_config["tablet"]` 迁移到 typed 字段。
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU `vga` typed 嵌套块（`type`/`memory`/`clipboard`），覆盖 schema、parse/encode、state/request mapping、raw 冲突校验、测试、示例和生成文档；无法解析的 `vga` 语法仍回退到 `raw.extra_config["vga"]`。
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU `serial` typed slot map（`serial0`–`serial3`），覆盖 schema、client slot 分类、state/request mapping、per-slot raw 冲突校验、测试、示例和生成文档；`raw.extra_config["serialN"]` 迁移到 typed 字段。
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU CPU 字段组（`numa` boolean、`vcpus`/`cpuunits` int64、`cpulimit` float64），覆盖 schema、新增 `proxmoxOptionalFloat64` 类型与 `setOptionalFloat64` helper、client decode/encode、state/request mapping、raw 冲突校验、测试、示例和生成文档；相关 `raw.extra_config` key 迁移到 typed 字段。
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU 内存 balloon 字段（`balloon`/`shares` int64、`hugepages` string enum），覆盖 schema、client decode/encode、state/request mapping、raw 冲突校验、测试、示例和生成文档；相关 `raw.extra_config` key 迁移到 typed 字段。
- 为 `proxmox_lxc_container` 资源和数据源新增 LXC CPU 字段（`cpulimit` float64、`cpuunits` int64），覆盖 schema、client decode/encode、state/request mapping、raw 冲突校验、测试和生成文档；相关 `raw.extra_config` key 迁移到 typed 字段。
- 为 `proxmox_lxc_container` 资源和数据源新增 LXC 控制台与脚本字段（`console` boolean、`tty` int64、`cmode`/`hookscript` string），覆盖 schema、client decode/encode、state/request mapping、raw 冲突校验、测试和生成文档；相关 `raw.extra_config` key 迁移到 typed 字段。
- 为 `proxmox_lxc_container` 资源新增 LXC clone 创建路径（`clone` create-time 嵌套块：`source_node`/`source_vmid`/`full`/`snapshot_name`/`storage`/`bwlimit`），覆盖 client `CloneLXCContainer`（`POST /nodes/{node}/lxc/{vmid}/clone` + task wait）、create 路径分支（clone vs ostemplate）、state 映射、测试、示例和生成文档；clone provenance 不可从 Proxmox 反查，导入/无 prior state 的 refresh 读回为 null。
- 新增 `proxmox_lxc_snapshot` 资源，覆盖 LXC 容器快照 CRUD（`POST/GET/PUT/DELETE /nodes/{node}/lxc/{vmid}/snapshot[/{snapname}]` + task wait）、schema（`node`/`vm_id`/`name` required、`description`、只读 `parent`/`snaptime`）、import（`node/vm_id/name`）、provider 注册、测试、示例和 reference 文档。
- 新增 `proxmox_qemu_snapshot` 资源，覆盖 QEMU VM 快照 CRUD（`POST/GET/PUT/DELETE /nodes/{node}/qemu/{vmid}/snapshot[/{snapname}]` + task wait）；提取共享 `client_snapshot.go`（QEMU/LXC 通用 snapshot 操作），schema、import、provider 注册、测试、示例和 reference 文档与 LXC snapshot 对齐。
- 新增 `proxmox_lxc_container` 资源和数据源支持，覆盖 LXC client、task wait、schema、mapping、raw 冲突校验、provider 注册、示例和 reference 文档。

## 接下来

- 针对 LXC 容器继续小步补齐 `network`/`mount_point` 结构化解析等后续能力，并保持 typed 与 `raw.extra_config` 单一 source of truth。
- 继续按 Proxmox QEMU API 对齐下一个小型 typed 字段，优先评估内存 balloon 相关 scalar（如 `balloon`/`shares`/`hugepages`）或剩余 slot 字段；仍保持 typed 与 `raw.extra_config` 单一 source of truth。
- 后续新增或修改 Provider schema 时，继续运行 `make generate` 更新 `docs/index.md`、`docs/resources/`、`docs/data-sources/`。
- 如需要面向用户的叙事型指南，可在现有 reference 文档之外补充认证方式选择、权限要求、常见 API 错误和本地 e2e 排障说明。
- 扩展 `proxmox_qemu_vm` typed 字段时，同步更新 schema、mapping、client 分类、测试和生成文档，并保持 typed 与 `raw.extra_config` 单一 source of truth。
