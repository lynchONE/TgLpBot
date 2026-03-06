<!-- OPENSPEC:START -->
# OpenSpec Instructions

These instructions are for AI assistants working in this project.

Always open `@/openspec/AGENTS.md` when the request:
- Mentions planning or proposals (words like proposal, spec, change, plan)
- Introduces new capabilities, breaking changes, architecture shifts, or big performance/security work
- Sounds ambiguous and you need the authoritative spec before coding

Use `@/openspec/AGENTS.md` to learn:
- How to create and apply change proposals
- Spec format and conventions
- Project structure and guidelines

Keep this managed block so 'openspec update' can refresh the instructions.

<!-- OPENSPEC:END -->

## 项目约束

- 后续新增或修改 `OpenSpec` 提案时，`proposal.md`、`design.md`、`tasks.md` 以及 spec delta 的正文默认使用中文撰写。
- 为兼容 `OpenSpec` 校验，固定结构头保持原格式不变，例如 `## ADDED Requirements`、`### Requirement:`、`#### Scenario:`；除这些固定头之外，其余说明、需求正文、场景内容默认写中文。
