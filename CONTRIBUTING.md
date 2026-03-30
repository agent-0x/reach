# Contributing to Reach / 贡献指南

[English](#english) | [中文](#中文)

---

## English

Thank you for your interest in contributing to Reach!

### Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<you>/reach.git`
3. Create a branch: `git checkout -b feat/my-feature`
4. Make your changes
5. Run tests: `make test`
6. Run linter: `make lint`
7. Commit with a descriptive message (see [Commit Convention](#commit-convention))
8. Push and open a Pull Request

### Development Setup

```bash
# Prerequisites: Go 1.22+, golangci-lint
make build   # Build binary to ./bin/reach
make test    # Run all tests
make lint    # Run linter
```

### Commit Convention

We use conventional commit messages:

- `feat:` — New feature
- `fix:` — Bug fix
- `docs:` — Documentation only
- `test:` — Adding or updating tests
- `refactor:` — Code change that neither fixes a bug nor adds a feature
- `security:` — Security improvements
- `chore:` — Build process, tooling, etc.

### Code Guidelines

- Follow standard Go conventions (`gofmt`, `go vet`)
- All exported functions must have doc comments
- Add tests for new functionality
- Keep PRs focused — one feature or fix per PR

### Reporting Issues

- Use the [Bug Report](https://github.com/agent-0x/reach/issues/new?template=bug_report.md) template for bugs
- Use the [Feature Request](https://github.com/agent-0x/reach/issues/new?template=feature_request.md) template for ideas

---

## 中文

感谢你对 Reach 的贡献兴趣！

### 开始

1. Fork 本仓库
2. 克隆你的 Fork：`git clone https://github.com/<you>/reach.git`
3. 创建分支：`git checkout -b feat/my-feature`
4. 进行修改
5. 运行测试：`make test`
6. 运行 lint：`make lint`
7. 使用描述性消息提交（参见 [提交规范](#提交规范)）
8. Push 并创建 Pull Request

### 开发环境

```bash
# 前置条件：Go 1.22+, golangci-lint
make build   # 构建到 ./bin/reach
make test    # 运行全部测试
make lint    # 运行 linter
```

### 提交规范

使用约定式提交消息：

- `feat:` — 新功能
- `fix:` — Bug 修复
- `docs:` — 仅文档变更
- `test:` — 添加或更新测试
- `refactor:` — 既非修复 Bug 也非添加功能的代码变更
- `security:` — 安全改进
- `chore:` — 构建流程、工具等

### 代码规范

- 遵循 Go 标准规范（`gofmt`、`go vet`）
- 所有导出函数需有文档注释
- 新功能需添加测试
- PR 保持专注 —— 每个 PR 只做一件事

### 报告问题

- Bug 使用 [Bug Report](https://github.com/agent-0x/reach/issues/new?template=bug_report.md) 模板
- 功能建议使用 [Feature Request](https://github.com/agent-0x/reach/issues/new?template=feature_request.md) 模板
