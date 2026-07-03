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
- 将 `proxmox_lxc_container` 资源和数据源的 `network`（`netN`）与 `mount_point`（`mpN`）从 `map[string]string` 升级为 typed 嵌套块（类比 QEMU network/disk 的 parse/encode + raw 回退），覆盖 network 的 `name`/`bridge`/`ip`/`gw`/`ip6`/`gw6`/`hwaddr`/`type`/`tag`/`trunks`/`rate`/`mtu`/`firewall`/`link_down` 与 mount_point 的 `volume`/`mp`/`size`/`backup`/`ro`/`quota`/`replicate`/`shared`/`acl`；无法解析的语法仍回退到 `raw.extra_config`，保持单一 source of truth。
- 新增 `proxmox_storage` 资源，覆盖 Proxmox VE 存储池管理（`POST/GET/PUT/DELETE /storage`），typed 常用字段（`storage`/`type` required + `content`/`nodes`/`disable`/`shared`/`path`/`pool`/`vg_name`/`thin_pool`/`server`/`export`/`share`/`username`/`password`/`monhost`/`datastore`/`namespace`/`fingerprint`/`smb_version`/`options`/`format`/`mkdir`/`sparse`/`nocow`/`krbd`/`blocksize`/`fs_name` + `raw.extra_config` 回退）、`storage`/`type` RequiresReplace、delete-key diff、import（`storage`）、provider 注册、测试、示例和 reference 文档。
- 新增 `proxmox_role` 资源（`/access/roles` CRUD，`role_id` required + `privs` 权限字符串）和 `proxmox_user` 资源（`/access/users` CRUD，`user_id` required + `comment`/`email`/`enable`/`expire`/`firstname`/`lastname`/`groups`/`keys`/`password`），覆盖 client、schema、import、provider 注册、测试、示例和 reference 文档；补全 access 管理 RBAC 三件套（group/role/user）。
- 新增 `proxmox_acl` 资源，覆盖 Proxmox VE 权限绑定（`GET/PUT /access/acl`），管理给定 `path` 下的 role×user/group 绑定（`path` required + `roles`/`users`/`groups` 列表 + `propagate`），支持 update diff（移除已删除绑定）、import（`path`）、provider 注册、测试、示例和 reference 文档；配合 group/role/user 形成完整 RBAC 工作流。
- 新增 `proxmox_storage` 和 `proxmox_storages` 数据源，覆盖存储池查询（`GET /storage/{storage}` 单个、`GET /storage` 列表），provider 注册、示例和 reference 文档。
- 新增 `proxmox_user` 和 `proxmox_users` 数据源，覆盖用户查询（`GET /access/users/{userid}` 单个、`GET /access/users` 列表），补全 access 管理数据源；provider 注册、示例和 reference 文档。
- 新增 `proxmox_role` 和 `proxmox_roles` 数据源，覆盖角色查询（`GET /access/roles/{roleid}` 单个、`GET /access/roles` 列表），provider 注册、示例和 reference 文档。
- 新增 `proxmox_node_firewall_options` 资源，覆盖节点防火墙选项管理（`GET/PUT /nodes/{node}/firewall/options`），typed 全部 20 个选项（`enable`/log_level_in/out/forward/conntrack/synflood/smurf/tcpflags/nftables 等），delete 通过 reset 所有 key 实现，import（`node`）、provider 注册、测试、示例和 reference 文档。
- 新增 `proxmox_user_token` 资源，覆盖 API token 管理（`POST/GET/PUT/DELETE /access/users/{userid}/token/{tokenid}`），`user_id`/`token_id` required + `comment`/`expire`/`privsep`，create 返回的敏感 `value`/`full_token_id` 保存在 state（不可从 Proxmox 读回），import（`userid/tokenid`）、provider 注册、测试、示例和 reference 文档。
- 新增 `proxmox_firewall_rule` 资源（`/cluster/firewall/rules` CRUD），content-based identity（11 identity fields `RequiresReplaceIfConfigured` + `enable`/`comment` mutable），`pos` computed 每次操作重新解析，duplicate pre-check（≥1 match 报错）、ambiguous match（≥2 报错）、no digest、no import、provider 注册、测试、示例和 reference 文档。
- 新增 `proxmox_lxc_container` 资源和数据源支持，覆盖 LXC client、task wait、schema、mapping、raw 冲突校验、provider 注册、示例和 reference 文档。

## 接下来

- 后续新增或修改 Provider schema 时，继续运行 `make generate` 更新 `docs/index.md`、`docs/resources/`、`docs/data-sources/`。
- 如需要面向用户的叙事型指南，可在现有 reference 文档之外补充认证方式选择、权限要求、常见 API 错误和本地 e2e 排障说明。
- 扩展 `proxmox_qemu_vm` typed 字段时，同步更新 schema、mapping、client 分类、测试和生成文档，并保持 typed 与 `raw.extra_config` 单一 source of truth。
