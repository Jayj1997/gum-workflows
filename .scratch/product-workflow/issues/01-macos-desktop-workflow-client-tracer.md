# 01: macOS Desktop / WorkflowClient tracer

**What to build:** 启动一个真实薄 macOS Desktop 应用，并让它与 Browser Mock 通过同一个 WorkflowClient 调用 fake Product Application。用户能打开应用、看到产品壳和一次真实的客户端到 Application 往返；桌面框架只充当 Adapter，不定义领域或运行语义。

**Blocked by:** None (can start immediately)

**Status:** done

- [x] macOS 应用能够构建、启动并显示最小产品页面。
- [x] Desktop Adapter 与 Browser Mock 实现同一个 WorkflowClient 行为合同。
- [x] 一次用户操作能穿过 UI、WorkflowClient 和 fake Product Application 返回可见结果。
- [x] UI 不直接访问 SQLite、Engine、Secret Store 或协议 Client。
- [x] Desktop Adapter 可被 fake 替换，领域测试不依赖系统 WebView。

## Comments

**2026-08-31（agent 实施记录）：** 使用 Wails v2.14.0 建立 `cmd/gum-desktop` 的 macOS Adapter。`internal/product.WorkflowApplication` 是唯一的 UI 用例边界；本票使用 deterministic `FakeApplication` 完成 tracer。Desktop binding 只暴露 `OpenWorkspace`，浏览器 mock 与桌面 client 均归一为 `WorkflowClient.openWorkspace()`，因而 UI 不触达 SQLite、Engine、Secret Store 或协议 client。

验证：先以 Go/Node 行为测试覆盖 Application、Desktop Adapter、两类 client 和可见 render state；再用 `wails build -clean` 实际生成、自签名并启动 macOS `.app`，手工点击 `Open workspace` 后页面显示 `Product application round-trip complete`。`go vet ./...`、`go test ./...` 与 `go test -race ./...` 已全绿。
