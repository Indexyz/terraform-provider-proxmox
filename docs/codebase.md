# 代码库扫描文档

本文档根据当前项目代码扫描整理，面向需要理解、扩展或审查该 Terraform Provider 的开发者。Terraform Registry 面向用户的 schema 文档仍由 `terraform-plugin-docs` 生成，见 `docs/index.md`、`docs/resources/` 和 `docs/data-sources/`。

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
| `internal/provider/resource_qemu_vm.go` | `proxmox_qemu_vm` 资源生命周期、导入、配置验证。 |
| `internal/provider/qemu_vm_schema.go` | QEMU VM resource/data source 共享 schema 和 Terraform model。 |
| `internal/provider/qemu_vm_mapping.go` | QEMU VM Terraform model、API request、API state 之间的转换；typed/raw 冲突检测。 |
| `internal/provider/data_source_*.go` | Proxmox inventory、access、pool、storage、QEMU、LXC 和 node 数据源。 |
| `internal/provider/*_test.go` | Provider、client、resource/data source、QEMU 映射、e2e smoke 测试。 |
| `docs/superpowers/` | 已有 spec/plan 归档；当前包含 GitHub Actions Proxmox e2e 的设计与实施计划。 |
| `examples/` | tfplugindocs 示例来源；包含 provider、19 个 data source、16 个 resource 示例。 |
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
| `ClusterMetricsServers` | `GET /cluster/metrics/server` | 集群 metrics server 配置。 |
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
| Role/User/Token methods | `/access/roles`、`/access/users`、`/access/users/{userid}/token` | RBAC 角色、用户和 API token 管理。 |
| ACL methods | `GET/PUT /access/acl` | 权限绑定读取和差异更新。 |
| Backup job methods | `/cluster/backup[/{id}]` | vzdump 备份计划 CRUD，不执行备份任务。 |
| Firewall methods | `/cluster/firewall/options`、`/cluster/firewall/rules`、节点和 guest `/firewall/options` | 集群选项/规则及节点/guest 防火墙选项管理。 |

## 资源

当前注册 **16 个资源**，并在 `examples/resources/` 中各有对应示例：

| 资源 | 主要职责 |
| --- | --- |
| `proxmox_acl` | 管理 path 下的 role 与 user/group 权限绑定。 |
| `proxmox_backup_job` | 管理 cluster-wide vzdump 备份计划、guest 选择和 retention。 |
| `proxmox_cluster_firewall_options` | 管理集群级防火墙启用、默认策略和日志限速。 |
| `proxmox_firewall_rule` | 管理集群级防火墙规则。 |
| `proxmox_group` | 管理 access group。 |
| `proxmox_guest_firewall_options` | 管理 QEMU/LXC guest 防火墙选项。 |
| `proxmox_lxc_container` | 管理 LXC 容器、clone 和 typed/raw 配置。 |
| `proxmox_lxc_snapshot` | 管理 LXC 快照。 |
| `proxmox_node_firewall_options` | 管理节点防火墙选项。 |
| `proxmox_pool` | 管理 pool 及其 guest/storage 成员。 |
| `proxmox_qemu_snapshot` | 管理 QEMU VM 快照。 |
| `proxmox_qemu_vm` | 管理 QEMU VM、clone 和 typed/raw 配置。 |
| `proxmox_role` | 管理 RBAC 角色和权限集合。 |
| `proxmox_storage` | 管理 Proxmox 存储池。 |
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

示例来源约定：`examples/provider/provider.tf` 进入 provider 首页；`examples/resources/<完整资源名>/resource.tf` 进入资源页；`examples/data-sources/<完整数据源名>/data-source.tf` 进入数据源页。当前 16 个资源和 19 个数据源均有对应示例。

注意：本地运行 `make generate` 需要 Terraform CLI；CI 的 `generate` job 会安装 Terraform 并检查生成后是否有未提交 diff。

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
- `client_test.go`、`client_qemu_test.go`：HTTP 方法、认证 header/cookie、API error、基础和 QEMU endpoints。
- `resource_data_mapping_test.go`、`helpers_behavior_test.go`：通用 flatten/diff/value helper。
- `resource_qemu_vm_test.go`、`data_source_qemu_vm_test.go`、`qemu_vm_mapping_test.go`：QEMU schema、state/request 映射、typed/raw 冲突、parse/encode。
- `e2e_smoke_test.go`：真实 Proxmox API smoke test，读取 `proxmox_version` 和 `proxmox_nodes`。
- `tools/ci/*_test.go`：GitHub Actions e2e 脚本行为。

GitHub Actions `Tests` workflow 包含 build、generate、Terraform CLI 矩阵单元测试，以及单节点 Proxmox e2e smoke job。e2e job 通过 `tools/ci/prepare-proxmox-e2e-image.sh` 准备 Proxmox VE 8.4-1 qcow2，再用 `tools/ci/start-proxmox-e2e.sh` 启动 QEMU 并轮询 `/api2/json/version`；acceptance smoke 只读验证 `proxmox_version` 和 `proxmox_nodes`。Release workflow 在 `v*` tag 上使用 GoReleaser 发布；另有 issue comment triage 和 inactive lock 维护工作流。

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
