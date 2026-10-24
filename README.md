# CPA Models Speed Test Plugin (`cpa-models-speed-test`)

High-performance model latency benchmarking plugin for [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI). Directly integrated into the CPA Web Management Panel, providing a Clash / v2rayN-style proxy node latency testing experience.

[中文文档 (Chinese Documentation)](./README_CN.md)

---

## 🌟 Highlights

- **Seamless Web UI Integration**: Automatically injects a **"模型测速" (Model Speed Test)** menu into CPA Management Panel.
- **Auto Channel & Model Discovery**: Uses host callbacks (`host.auth.models` and `host.auth.list`) to list all upstream accounts and models (OpenAI, Claude, Gemini, Codex, DeepSeek, etc.).
- **Batch & Single Speed Testing**:
  - Top **【一键测速】(Batch Test)** button triggers full concurrent backend testing.
  - Model card right **【单独测速】(Test)** button for individual re-tests.
- **Lightweight Streaming Probe (TTFT)**:
  - Sends a minimal 1-token prompt with `stream: true`.
  - Disconnects immediately upon receiving the first token/chunk (`host.model.stream_close`), measuring accurate TTFT without wasting account tokens.
- **Backend Proxied (No CORS issues)**: Requests are dispatched and measured exclusively by the backend Go process.
- **High Concurrency via Goroutines**: Worker pool with semaphore concurrency limits avoids sequential bottlenecks across channels.
- **Visual Feedback & Color Thresholds**:
  - Green (< 800ms): Optimal
  - Amber (800ms - 1500ms): Good
  - Orange (> 1500ms): Slow
  - Purple: Quota Limit / 429
  - Red: Error
- **WebMCP Debugging Interface**: Standardized browser agent hooks exposed on `window.__web_mcp__` using Vue 3 and VueUse.

---

## 📦 Quick Start

### 1. Place the binary

Place the compiled shared library into your CPA `plugins/` directory:
- Linux: `plugins/cpa-models-speed-test.so`
- macOS: `plugins/cpa-models-speed-test.dylib`
- Windows: `plugins/cpa-models-speed-test.dll`

### 2. Enable in `config.yaml`

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    cpa-models-speed-test:
      enabled: true
      priority: 1
      concurrency: 20
```

---

## 🛠️ Build

```bash
# 1. Build frontend bundle
cd web && bun install && bun run build && cd ..

# 2. Build Go plugin library
go build -buildmode=c-shared -o bin/cpa-models-speed-test.so .
```

---

## License

MIT
