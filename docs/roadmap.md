# Roadmap

## 已完成

- 扫描项目结构、Provider 配置、HTTP client、资源、数据源、QEMU VM typed/raw 映射、文档生成和 CI/e2e 工具链。
- 新增 `docs/codebase.md`，整理开发者向代码库说明、API surface、资源/数据源职责、QEMU 扩展边界、测试与 CI 入口、已有 spec/plan 归档，并记录 `AGENTS.md` 中的贡献约束。
- 确认现有 `docs/` schema/reference 文档由 `tfplugindocs` 生成，3 个资源和 12 个数据源均有示例来源。
- 为 QEMU VM `protection` typed 字段新增预期失败的 client、mapping、raw conflict 与 schema 属性单元测试。
- 为 `proxmox_qemu_vm` 资源和数据源新增 Proxmox QEMU `protection` typed boolean，覆盖 schema、client decode/encode、state/request mapping、raw 冲突校验、测试、示例和生成文档；`raw.extra_config["protection"]` 迁移到 typed 字段。

## 接下来

- 继续按 Proxmox QEMU API 对齐下一个小型 typed 字段，优先评估 `scsihw` 或 `tablet`；仍保持 typed 与 `raw.extra_config` 单一 source of truth。
- 后续新增或修改 Provider schema 时，继续运行 `make generate` 更新 `docs/index.md`、`docs/resources/`、`docs/data-sources/`。
- 如需要面向用户的叙事型指南，可在现有 reference 文档之外补充认证方式选择、权限要求、常见 API 错误和本地 e2e 排障说明。
- 扩展 `proxmox_qemu_vm` typed 字段时，同步更新 schema、mapping、client 分类、测试和生成文档，并保持 typed 与 `raw.extra_config` 单一 source of truth。
