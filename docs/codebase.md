# 代码库扫描文档

本文档根据当前项目代码扫描整理，面向需要理解、扩展或审查该 Terraform Provider 的开发者。Terraform Registry 面向用户的 schema 文档仍由 `terraform-plugin-docs` 生成，见 `docs/index.md`、`docs/resources/` 和 `docs/data-sources/`；手工维护的认证、权限和错误排障说明见 `docs/guides/provider-configuration.md`。

## 项目概览

- 项目是基于 Terraform Plugin Framework 的 Proxmox VE Provider，Go module 为 `github.com/indexyz/terraform-provider-proxmox`。
- Provider 二进制入口在 `main.go`，通过 `providerserver.Serve` 以 `registry.terraform.io/indexyz/proxmox` 地址启动。
- Provider 实现集中在 `internal/provider/`，包括配置、HTTP 客户端、资源、数据源，以及 QEMU、LXC、存储、防火墙和 RBAC 映射逻辑。
- 面向 Terraform 用户的生成文档在 `docs/`；示例配置在 `examples/`；CI 和本地 Proxmox e2e 辅助脚本在 `tools/ci/`。

## 目录与职责

| 路径 | 职责 |
| --- | --- |
| `main.go` | Provider 进程入口；解析 `-debug` 并启动 Terraform provider server。 |
| `internal/provider/provider.go` | Provider metadata、schema、配置解析、资源/数据源注册。 |
| `internal/provider/client.go` | Proxmox API HTTP 客户端、认证、通用请求封装、基础资源 API 方法。 |
| `internal/provider/client_qemu.go` | QEMU VM API 方法，以及 Proxmox `/config` 原始响应解码。 |
| `internal/provider/resource_group.go` | `proxmox_group` 资源。 |
| `internal/provider/resource_pool.go` | `proxmox_pool` 资源及 pool 成员协调逻辑。 |
| `internal/provider/resource_qemu_vm.go` | `proxmox_qemu_vm` 资源生命周期、导入、配置验证、CD-ROM 附着安全检查。 |
| `internal/provider/resource_nocloud_iso.go` | `proxmox_nocloud_iso` 资源生命周期、上传任务对账、内容存在性判定。 |
| `internal/provider/nocloud_iso.go` | 纯 Go ISO9660 `CIDATA` seed 生成器。 |
| `internal/provider/qemu_vm_schema.go` | QEMU VM resource/data source 共享 schema 和 Terraform model。 |
| `internal/provider/qemu_vm_mapping.go` | QEMU VM Terraform model、API request、API state 之间的转换；typed/raw 冲突检测。 |
| `internal/provider/data_source_*.go` | Proxmox inventory、access、pool、storage、QEMU、LXC 和 node 数据源。 |
| `internal/provider/*_test.go` | Provider、client、resource/data source、QEMU 映射、e2e smoke 测试。 |
| `docs/guides/` | 从 `templates/guides/` 渲染的用户指南；当前包含 Provider 配置排障和 NoCloud seeded VM 供应链。 |
| `docs/superpowers/` | 已有 spec/plan 归档；当前包含 GitHub Actions Proxmox e2e 的设计与实施计划。 |
| `templates/guides/` | 手工维护的指南模板；`make generate` 时渲染到 `docs/guides/`，避免 tfplugindocs 清理生成目录时丢失。 |
| `examples/` | tfplugindocs 示例来源；包含 provider、21 个 data source、26 个 resource 示例。 |
| `tools/tools.go` | `go generate` 工具入口：copywrite、Terraform 示例格式化、tfplugindocs 文档生成。 |
| `tools/ci/` | GitHub Actions Proxmox e2e VM 镜像准备、启动脚本和脚本测试。 |

## Provider 配置流程

`ProxmoxProvider.Configure` 的流程：

1. 从 Terraform 配置读取 `ProxmoxProviderModel`。
2. `providerConfigFromModel` 合并显式配置和环境变量。
3. 校验 endpoint 和认证组合。
4. 调用 `NewClient` 创建 Proxmox API client。
5. 将 client 同时注入 `DataSourceData` 和 `ResourceData`。

支持的环境变量：

| Terraform 属性 | 环境变量 | 说明 |
| --- | --- | --- |
| `endpoint` | `PROXMOX_VE_ENDPOINT` | Proxmox API endpoint；缺省 path 会补为 `/api2/json`。 |
| `username` | `PROXMOX_VE_USERNAME` | Ticket 认证用户名，例如 `root@pam`。 |
| `password` | `PROXMOX_VE_PASSWORD` | Ticket 认证密码。 |
| `otp` | `PROXMOX_VE_OTP` | Ticket 认证的一次性密码。 |
| `api_token_id` | `PROXMOX_VE_API_TOKEN_ID` | API token ID，格式类似 `user@realm!tokenid`。 |
| `api_token_secret` | `PROXMOX_VE_API_TOKEN_SECRET` | API token secret。 |
| `insecure` | `PROXMOX_VE_INSECURE` | 是否跳过 TLS 证书校验。 |
| `timeout_seconds` | `PROXMOX_VE_TIMEOUT` | HTTP 超时秒数，默认 `30`。 |
| `user_agent` | 无 | 自定义 User-Agent；默认 `terraform-provider-proxmox/<version>`。 |

认证规则：

- API token 认证必须同时配置 `api_token_id` 和 `api_token_secret`。
- Ticket 认证必须同时配置 `username` 和 `password`，可选 `otp`。
- API token 认证和 ticket 认证互斥。
- 未配置任何认证会返回 Terraform diagnostic。
- `PROXMOX_VE_INSECURE` 或 `PROXMOX_VE_TIMEOUT` 解析失败时当前实现会回退到 `false`/默认超时，不额外产生 diagnostic。

## API 客户端

`Client.do` 是所有 API 方法的公共路径：

- 基于 `baseURL` 和 API path 拼接请求 URL。
- GET 请求使用 query string；非 GET 且调用方提供 form 时使用 `application/x-www-form-urlencoded` body。
- API token 认证设置 `Authorization: PVEAPIToken=...`。
- Ticket 认证设置 `PVEAuthCookie`，非 GET 请求额外设置 `CSRFPreventionToken`。
- 404 统一映射为 `errNotFound`，供资源 read/delete 处理远端消失。
- 非 2xx 响应通过 `decodeAPIError` 保留状态码、API errors/body。
- 成功响应按 Proxmox `{ "data": ... }` envelope 解码。

Endpoint 由 `normalizeEndpoint` 规范化：必须是完整 URL，不能包含 query/fragment；空 path 自动补 `/api2/json`，非 `/api2/json` 结尾的 path 会追加该后缀。

## API surface 对照

下表概览当前主要 API family；具体请求字段和映射以对应的 `client_*.go` 为准。

| Client 方法 | HTTP/API | 用途 |
| --- | --- | --- |
| `Version` | `GET /version` | Proxmox VE 版本信息。 |
| `Nodes` | `GET /nodes` | 集群节点列表。 |
| `NodeStatus` | `GET /nodes/{node}/status` | 单节点状态详情。 |
| `NodeDNS` | `GET /nodes/{node}/dns` | 节点 DNS 设置。 |
| `NodeTime` | `GET /nodes/{node}/time` | 节点时间和时区。 |
| `ClusterResources` | `GET /cluster/resources?type=...` | 集群资源清单，可按 `vm`、`storage`、`node`、`sdn` 过滤。 |
| Metrics server methods | `/cluster/metrics/server[/{id}]` | Graphite、InfluxDB 和 OpenTelemetry metrics server 查询与 CRUD。 |
| `GetPool`/`Pools` | `GET /pools` | 单个或全部 pool。 |
| `CreatePool`/`UpdatePool`/`DeletePool` | `POST`/`PUT`/`DELETE /pools` | Pool 与成员管理。 |
| `GetGroup`/`Groups` | `GET /access/groups/{groupid}`、`GET /access/groups` | 单个或全部 access group。 |
| `CreateGroup`/`UpdateGroup`/`DeleteGroup` | `POST /access/groups`、`PUT`/`DELETE /access/groups/{groupid}` | Group 管理。 |
| `GetQemuVMConfig` | `GET /nodes/{node}/qemu/{vmid}/config` | QEMU VM 配置。 |
| `GetQemuVMStatus` | `GET /nodes/{node}/qemu/{vmid}/status/current` | QEMU VM 运行状态。 |
| `CreateQemuVM` | `POST /nodes/{node}/qemu` | 普通创建 QEMU VM。 |
| `CloneQemuVM` | `POST /nodes/{sourceNode}/qemu/{sourceVMID}/clone` | clone 模式创建 QEMU VM。 |
| `UpdateQemuVM` | `PUT /nodes/{node}/qemu/{vmid}/config` | 更新 QEMU `/config`。 |
| `DeleteQemuVM` | `DELETE /nodes/{node}/qemu/{vmid}` | 删除 QEMU VM。 |
| LXC container methods | `/nodes/{node}/lxc[/{vmid}]`、`/config`、`/clone` | LXC 创建、clone、读取、更新和删除。 |
| Snapshot methods | `/nodes/{node}/{qemu|lxc}/{vmid}/snapshot[/{snapname}]` | QEMU/LXC 快照 CRUD 和任务等待。 |
| Storage methods | `/storage[/{storage}]` | 存储池 CRUD 和查询。 |
| Storage file methods | `/nodes/{node}/storage/{storage}/download-url`、`/content/{volume}` | 下载、读取和删除 ISO、LXC template 与 import image，并等待异步任务。 |
| Role/User/Token methods | `/access/roles`、`/access/users`、`/access/users/{userid}/token` | RBAC 角色、用户和 API token 管理。 |
| Realm methods | `/access/domains[/{realm}]` | 读取任意 realm，并管理 Proxmox VE 9 LDAP、AD 和 OpenID Connect 外部 realm CRUD。 |
| ACL methods | `GET/PUT /access/acl` | 权限绑定读取和差异更新。 |
| Backup job methods | `/cluster/backup[/{id}]` | vzdump 备份计划 CRUD，不执行备份任务。 |
| Replication job methods | `/cluster/replication[/{id}]` | 存储复制计划 CRUD，不执行 run-now。 |
| HA resource methods | `/cluster/ha/resources[/{sid}]` | PVE 9 HA enrollment/policy CRUD；collection read、digest update、`purge=0` destroy，不调用 guest 或 HA runtime 命令。 |
| Firewall methods | `/cluster/firewall/{options,rules,aliases,ipset,groups}`、node/guest `/firewall/{options,rules}` | 集群命名对象、cluster/node/guest/group rules 及防火墙选项管理。 |

## 资源

当前注册 **26 个资源**，并在 `examples/resources/` 中各有对应示例：

| 资源 | 主要职责 |
| --- | --- |
| `proxmox_acl` | 管理 path 下的 role 与 user/group 权限绑定。 |
| `proxmox_backup_job` | 管理 cluster-wide vzdump 备份计划、guest 选择和 retention。 |
| `proxmox_cluster_firewall_alias` | 管理集群级防火墙 IP/network alias。 |
| `proxmox_cluster_firewall_ip_set` | 管理集群级命名 IP set。 |
| `proxmox_cluster_firewall_ip_set_entry` | 管理集群级 IP set 地址、网络或 alias 条目。 |
| `proxmox_cluster_firewall_options` | 管理集群级防火墙启用、默认策略和日志限速。 |
| `proxmox_cluster_firewall_security_group` | 管理集群级防火墙 security group。 |
| `proxmox_cluster_metrics_server` | 管理 Graphite、InfluxDB 或 OpenTelemetry metrics server。 |
| `proxmox_firewall_rule` | 通过 content identity 管理 cluster、node、QEMU/LXC guest 或 security group 防火墙规则。 |
| `proxmox_group` | 管理 access group。 |
| `proxmox_guest_firewall_options` | 管理 QEMU/LXC guest 防火墙选项。 |
| `proxmox_ha_resource` | 管理现有 QEMU/LXC guest 的 PVE 9 HA enrollment、显式 requested state 和恢复策略；destroy 固定 `purge=0`，只退出 HA 管理。 |
| `proxmox_lxc_container` | 管理 LXC 容器、clone 和 typed/raw 配置。 |
| `proxmox_lxc_snapshot` | 管理 LXC 快照。 |
| `proxmox_node_firewall_options` | 管理节点防火墙选项。 |
| `proxmox_pool` | 管理 pool 及其 guest/storage 成员。 |
| `proxmox_qemu_snapshot` | 管理 QEMU VM 快照。 |
| `proxmox_qemu_vm` | 管理 QEMU VM、clone、typed/raw 配置、`start_on_create`/`stop_on_destroy` 生命周期钩子和 `nocloud_cdrom_slot` NoCloud seed 槽位标记。 |
| `proxmox_nocloud_iso` | 生成并管理 cloud-init NoCloud seed ISO（`CIDATA` 卷标）的 storage content；全部创建输入 RequiresReplace，已存在目标文件拒绝而不采纳。 |
| `proxmox_realm` | 管理 Proxmox VE 9 LDAP、AD 或 OpenID Connect 外部认证 realm；secret 使用 WriteOnly + version 轮换。 |
| `proxmox_replication_job` | 管理 cluster storage replication 计划，不隐式运行复制或清理数据。 |
| `proxmox_role` | 管理 RBAC 角色和权限集合。 |
| `proxmox_storage` | 管理 Proxmox 存储池。 |
| `proxmox_storage_file_download` | 下载并管理 storage 中的 ISO、LXC template 或 import image。 |
| `proxmox_user` | 管理 Proxmox 用户。 |
| `proxmox_user_token` | 管理用户 API token。 |

下面记录 group、pool 和 QEMU VM 的关键生命周期边界；其它资源的用户 schema 以 `docs/resources/` 中的生成文档为准。

### `proxmox_group`

实现文件：`internal/provider/resource_group.go`

- 管理 `/access/groups`。
- `group_id` 必填且变更需要替换。
- `comment` 可管理；`members` 为只读计算属性，来自 Proxmox group users。
- Read 时远端 404 会从 Terraform state 中移除资源。
- Import ID 同时写入 `group_id` 和 `id`。

### `proxmox_pool`

实现文件：`internal/provider/resource_pool.go`

- 管理 `/pools`。
- `pool_id` 必填且变更需要替换。
- `comment`、`vm_ids`、`storage_ids` 可管理；`members` 是 Proxmox 返回的解析后成员列表。
- `allow_move` 仅影响添加已属于其他 pool 的 guest/storage 时是否传 `allow-move=1`。
- Update 先读取当前 pool，比较当前成员与期望成员，再分别调用 remove/add。
- Delete 先清空 pool 成员，再删除 pool；远端已不存在时视为删除完成。

### `proxmox_qemu_vm`

实现文件：`internal/provider/resource_qemu_vm.go`、`qemu_vm_schema.go`、`qemu_vm_mapping.go`、`client_qemu.go`

- 标识为 `node/vm_id`，导入 ID 格式也是 `node/vmid`。
- `node` 和 `vm_id` 变更需要替换。
- Create 支持两条路径：
  - 无 `clone`：`POST /nodes/{node}/qemu` 创建。
  - 有 `clone`：先 clone，再把其它可管理配置通过 `/config` 更新到克隆出的 VM。
- Read 同时读取 `/config` 和 `/status/current`。
- `status`、`uptime`、`template` 是观察值；Provider 不管理电源状态或模板转换。
- `clone` 是 create-time 输入，变更需要替换；对 imported resource 或没有 prior state 的 refresh，Provider 不能从 Proxmox 推断 clone provenance，因此读回为空。
- QEMU 配置分为顶层常用字段、`common`、`cloud_init`、`network`、`disk`、`efi_disk`、`tpm_state`、`raw`。
- `raw.extra_config` 是未 typed 的 Proxmox `/config` escape hatch；`ValidateConfig` 会禁止同一个 Proxmox key 同时由 typed 字段和 raw 管理。
- `start_on_create` 在 create/clone 与 `/config` 更新成功后启动一次并等待任务；`stop_on_destroy` 在删除前对运行中 guest 执行硬断电（`qm stop` 语义，非优雅关机）并等待任务。两者都是 Terraform 侧 create/destroy 钩子而非声明式电源状态；更新选项或 refresh 已停止 guest 绝不触发动作；停止失败、超时或 stop 任务轮询 404 一律中止删除，后续重试由 config GET 判定真实缺失。
- Create 会先做 CD-ROM 附着安全检查：计划内 CD-ROM（`media = "cdrom"` 或 `.iso` 卷）仅允许 ide/sata/scsi 槽位。严格 seed 检查仅在设置 `nocloud_cdrom_slot` 标记时生效（该标记是 create-time 输入，变更 RequiresReplace，声明哪个 typed disk 槽位是 NoCloud seed）：标记槽位必须计划真实 ISO 卷，最多一个 seed，seed storage 必须在 VM 所在节点可见、active、支持 `iso` content，并通过节点 ISO content 集合读取确认精确卷存在（同名校名存储在其他节点缺文件时先于 clone 失败；403/500 仍按错误处理而非缺席）。未标记的普通多 ISO/native cloudinit 用法不受 seed 布局限制。clone/标记化 update 在应用 `/config` 前读取原始 wire disk 配置：目标槽位仅允许已持同一卷、空盘位、该 VM 的 Proxmox 生成 cloud-init 盘（`storage:vm-<vmid>-cloudinit` 或文件型 `storage:<vmid>/vm-<vmid>-cloudinit.<fmt>`，要求 `media=cdrom` 与精确 owner VMID）显式同槽替换；标记工作流额外拒绝伪介质值（`none`/`cdrom`）清除继承的硬盘/外来介质、拒绝有效 wire 配置中残留第二个 ISO/cloud-init 盘（含 typed parser 未完全识别的 raw 盘）；标记化 update 拒绝就地更换任何不同真实介质（含先前 apply 中本资源附着过的旧 seed：refresh 后的 state 只是观测现实，不构成附着所有权），仅允许同卷、空盘位、该 VM 同槽 PVE cloud-init 盘变更，拒绝错误明确指向以 VM 替换（如 lifecycle replace_triggered_by）完成 seed 更替而非采纳/覆盖。失败保留克隆身份，先于 PUT/start。clone 目的地使用官方 `target` 表单键（跨节点 clone 要求源 VM 磁盘在共享存储上，任务仍在源节点轮询）。QEMU VM 的 in-place Update PUT 采用增量写入：typed `disk` 与 `network` 映射仅发送相对 prior state 新增或变化的槽位，未变化的继承观测值（模板克隆的根盘/NIC）不再作为变更意图回发（全部计划期/live typed+raw 安全检查在收窄前覆盖完整有效附着集合，create/clone 后初始附着不受影响，serial/ipconfig/标量/raw 不在收窄范围）；资源盘状态投影以 prior 已知盘键集合为准，prior 已知空映射（`disk = {}`）保留为已知空映射，仅 prior 缺失/unknown 保留完整观测盘清单（data source 仍可查询完整 guest 盘清单）。

### `proxmox_nocloud_iso`

实现文件：`internal/provider/resource_nocloud_iso.go`、`nocloud_iso.go`、`client_storage_content.go`

- 标识为 `node/storage/volume_id`；全部创建输入 RequiresReplace，内容输入写入 state 明文（sensitive 不加密）。
- 内存外的真实生成路径：`user-data`/`meta-data`/`network-config` 先以明文写入 ISO9660 writer 的私有 `0700` 临时 staging 目录（位置跟随 `TMPDIR`），最终镜像在内存组装后经 `/nodes/{node}/storage/{storage}/upload` multipart 上传；正常路径全清理 staging 且清理错误并入返回错误，provider 崩溃/强杀可能在临时目录残留明文。任务按 UPID 实际 owner 节点轮询，accepted UPID 先写入 private state 再等待，失败可对账重试。
- 上传节点与 VM 节点都要求目标 storage 可见、enabled、active 且支持 `iso` content；存在性判定以 content collection 读取为准。
- 删除仅针对 state 记录的精确卷；VM 依赖 `volume_id` 引用保证销毁顺序，示例指南要求 ISO 资源 `create_before_destroy` 以避免在 VM 仍引用时删除 ISO。

## QEMU typed/raw 映射规则

QEMU VM 映射代码的核心边界：typed schema 覆盖常见配置，raw 保留长尾配置；同一个 Proxmox key 只能有一个 source of truth。

- Cloud-init IP 配置以 `ipconfig0` 这类 slot key 管理，字段映射到 Proxmox `ip`、`gw`、`ip6`、`gw6`。
- Network 配置以 `net0` 这类 slot key 管理；支持 model、bridge、macaddr、tag、trunks、firewall、link_down、mtu、queues、rate。
- Disk 配置以 `ide*`、`sata*`、`scsi*`、`virtio*` slot key 管理；支持 storage/volume/size、media/cache/discard、boolean flags 和 IOPS/MBPS QoS 字段。
- `efidisk0` 和 `tpmstate0` 有 typed block；Provider 无法解析的 grammar 会保留到 `raw.extra_config`。
- 从 API 读取时，完全支持的 slot 会进入 typed map；无法完整解析的 network/disk/EFI/TPM 项会回落到 `raw.extra_config`，避免静默丢配置。
- 写入时，typed block 会编码为 Proxmox 逗号分隔配置字符串；`raw.extra_config` 中的 key 按排序后写入 form。

扩展 QEMU typed 字段时，通常需要同时更新：

1. `qemu_vm_schema.go` 的 model、attr types 和 resource/data source attribute。
2. `qemu_vm_mapping.go` 的 parse、encode、state、request、typed conflict key 逻辑。
3. `client_qemu.go` 的已知字段或分类逻辑（如果是新的顶层 Proxmox key 或 slot 类型）。
4. `qemu_vm_mapping_test.go` 和相关 resource/data source schema 测试。
5. 运行 `make generate` 更新 Terraform schema 文档。

## 数据源

| 数据源 | 实现文件 | API/说明 |
| --- | --- | --- |
| `proxmox_version` | `data_source_version.go` | `GET /version`。 |
| `proxmox_nodes` | `data_source_nodes.go` | `GET /nodes`，列出节点概要。 |
| `proxmox_node` | `data_source_node.go` | `GET /nodes/{node}/status`，读取单节点详细状态。 |
| `proxmox_node_dns` | `data_source_node_dns.go` | `GET /nodes/{node}/dns`。 |
| `proxmox_node_time` | `data_source_node_time.go` | `GET /nodes/{node}/time`。 |
| `proxmox_cluster_resources` | `data_source_cluster_resources.go` | `GET /cluster/resources`，支持 `type` 过滤。 |
| `proxmox_cluster_metrics_servers` | `data_source_cluster_metrics_servers.go` | `GET /cluster/metrics/server`。 |
| `proxmox_group` | `data_source_group.go` | `GET /access/groups/{groupid}`。 |
| `proxmox_groups` | `data_source_groups.go` | `GET /access/groups`。 |
| `proxmox_lxc_container` | `data_source_lxc_container.go` | 读取 LXC `/config` 和 `/status/current`，共享 LXC typed/raw 映射。 |
| `proxmox_pool` | `data_source_pool.go` | `GET /pools?poolid=...`。 |
| `proxmox_pools` | `data_source_pools.go` | `GET /pools`。 |
| `proxmox_qemu_vm` | `data_source_qemu_vm.go` | 读取 QEMU `/config` 和 `/status/current`，共享 QEMU typed/raw 映射。 |
| `proxmox_realm` | `data_source_realm.go` | `GET /access/domains/{realm}`；仅输出公开 typed 字段，支持只读查询内建 `pam`/`pve`。 |
| `proxmox_role` | `data_source_role.go` | `GET /access/roles/{roleid}`。 |
| `proxmox_roles` | `data_source_roles.go` | `GET /access/roles`。 |
| `proxmox_storage` | `data_source_storage.go` | `GET /storage/{storage}`。 |
| `proxmox_storages` | `data_source_storages.go` | `GET /storage`。 |
| `proxmox_user` | `data_source_user.go` | `GET /access/users/{userid}`。 |
| `proxmox_users` | `data_source_users.go` | `GET /access/users`。 |

新增数据源时，应补齐 client 方法、data source schema/read、`provider.go` 注册、单元测试和生成文档。

## 文档生成

用户文档由 `tools/tools.go` 的 `go:generate` 指令驱动：

```bash
make generate
```

该命令会执行：

1. `copywrite headers` 更新版权头。
2. `terraform fmt -recursive ../examples/` 格式化 Terraform 示例。
3. `tfplugindocs generate --provider-dir .. -provider-name proxmox` 生成 `docs/index.md`、`docs/resources/`、`docs/data-sources/`。

示例来源约定：`examples/provider/provider.tf` 进入 provider 首页；`examples/resources/<完整资源名>/resource.tf` 进入资源页；`examples/data-sources/<完整数据源名>/data-source.tf` 进入数据源页。当前 26 个资源和 21 个数据源均有对应示例。

注意：本地运行 `make generate` 需要 Terraform CLI；CI 的 `generate` job 会安装 Terraform 并检查生成后是否有未提交 diff。指南源码手工维护在 `templates/guides/`，由 tfplugindocs 渲染到 `docs/guides/`；不要只编辑生成结果。

## 测试与 CI

常用本地命令：

```bash
go test ./...
(cd tools && go test ./...)
make generate
```

`GNUmakefile` 还提供 `fmt`、`lint`、`build`、`install`、`test`、`testacc` 目标。

测试覆盖重点：

- `provider_unit_test.go`：配置合并、认证校验、资源/数据源导出。
- `framework_lifecycle_test.go`、`lifecycle_http_test.go`：schema-backed plan/state/config、provider wiring 与本地 Proxmox HTTP 合约测试工具；普通 Go 测试不依赖 Terraform CLI 或真实 PVE。
- `data_source_lifecycle_test.go`：使用真实 provider client 与本地 HTTP server 覆盖全部注册数据源的 Schema、Configure、Read、typed state 和代表性 API error。
- `resource_*_lifecycle_test.go`：按 access/pool/firewall/cluster service/storage/snapshot/guest 家族覆盖 Create、Read、Update、Delete、import、missing/error、task polling、private managed fields、secret preservation 和安全删除请求。
- `client_test.go`、`client_qemu_test.go`：HTTP 方法、认证 header/cookie、API error、基础和 QEMU endpoints；QEMU create/clone/delete 测试同时验证 UPID completion polling。
- `resource_data_mapping_test.go`、`helpers_behavior_test.go`：通用 flatten/diff/value helper。
- `resource_qemu_vm_test.go`、`data_source_qemu_vm_test.go`、`qemu_vm_mapping_test.go`：QEMU schema、state/request 映射、typed/raw 冲突、parse/encode。
- `resource_qemu_vm_cdrom_test.go`：CD-ROM 附着安全的全链路 mock 测试（seed 上传 → clone → 同槽替换 Proxmox cloud-init → start → 硬断电销毁 → ISO 清理）与外来介质/双 seed/硬盘覆盖/槽位与节点存储拒绝矩阵。
- `terraform_core_generation_test.go`：构建 provider 二进制并用真实 Terraform CLI（dev_overrides）对本地 mock PVE API 执行指南的代次链路：generation 1 apply → generation 2 替换 → destroy，断言真实 Terraform core 依赖图导出的顺序（seed 上传先于 clone、替换时旧 VM 先销毁、旧 seed 仅在替换 VM 重新指向并启动后才删除、销毁时 VM 先于 seed 删除），而非手工调用资源回调。
- `resource_nocloud_iso_lifecycle_test.go`：NoCloud ISO 的 multipart 上传、task owner 轮询、retained task 对账、existing-file 拒绝与幂等清理。
- `client_realm_test.go`、`resource_realm_test.go`、`data_source_realm_test.go`：PVE 9 realm exact-form CRUD/read、variant 校验、secret 过滤、WriteOnly version 轮换和 managed-field deletion。
- `client_ha_resource_test.go`、`resource_ha_resource_test.go`：PVE 9 HA collection lookup、exact-form CRUD、fresh digest contract、`purge=0`、schema/validation、effective defaults 和 managed-field deletion。
- `e2e_smoke_test.go`：三个真实 Proxmox API e2e 测试，要求 PVE 9；只读测试覆盖 node/cluster/access/storage inventory，CRUD 测试用随机 ID 管理并清理 pool、group、role、user、API token 和 ACL，QEMU task-waiting 测试创建随机高 VMID 的空 source VM 与同节点 full clone，并验证 create/clone/delete UPID polling、正常 Terraform destroy 和带名称所有权校验的失败清理。
- `tools/ci/*_test.go`：GitHub Actions e2e 脚本行为。

GitHub Actions `Tests` workflow 包含 build、generate、Terraform CLI 矩阵单元测试，以及单节点 Proxmox e2e job。e2e job 通过 `tools/ci/prepare-proxmox-e2e-image.sh` 准备 Proxmox VE 9.2-1 qcow2，再用 `tools/ci/start-proxmox-e2e.sh` 启动 QEMU 并轮询 `/api2/json/version`；精确 selector 只选择三个仓库自有 acceptance 测试：node/cluster/access/storage 只读数据源、随机 access CRUD，以及随机高 VMID 的空 QEMU source VM/同节点 full clone create/clone/delete task polling。QEMU 测试不附加 disk、不指定 storage、不配置 network，也不启动 VM；失败清理按 clone 后 source 顺序删除且先核对随机 owned name。真实 PVE 验证仍不覆盖存储配置、防火墙、HA、replication、backup、外部 realm、LXC、guest runtime 或多节点行为。本地依赖安装、复现、停止、重建和日志排障见 `tools/ci/README.md`。Release workflow 在 `v*` tag 上使用 GoReleaser 发布；另有 issue comment triage 和 inactive lock 维护工作流。

## 贡献边界

根据 `AGENTS.md`，后续修改代码或文档时应保持以下边界：

- 不添加 legacy fallback；确定无用的兼容层应删除而不是保留 shim。
- Lyre audio topology 只能是 server relay；不要添加、恢复或保留 peer mesh audio mode、peer-to-peer audio negotiation 或 mesh compatibility fallback。
- 只做当前需求需要的最小改动，不做无关重构；防御性检查只放在真正的外部边界。
- 不吞掉底层错误信息；跨配置、网络、系统调用、runtime 边界时保留 cause/context 链。
- QEMU VM 的 `status`、`uptime`、`template` 保持观察值，不从读取结果推断 declarative power/template 管理。
- Clone 配置保持 create-mode 输入；不要把 clone provenance 当作可从 Proxmox 反查的长期 drift source。
- Typed nested block 与 `raw.extra_config` 不应管理同一个 Proxmox key 或 slot。
- 更新代码后同步维护 `docs/roadmap.md`，记录已完成和下一步。
