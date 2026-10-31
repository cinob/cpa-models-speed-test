# CPA Models Speed Test Plugin (`cpa-models-speed-test`)

High-performance model latency benchmarking plugin for [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI). Directly integrated into the CPA Web Management Panel, providing a Clash / v2rayN-style proxy node latency testing experience.

[中文文档 (Chinese Documentation)](./README_CN.md)

---

## 🌟 Highlights

- **Multi-language Support (i18n)**: Seamless language switcher (English 🌐, Russian 🌐, Simplified Chinese 🌐) with auto-detection and persistence.
- **Seamless Web UI Integration**: Automatically injects a **"Speed Test"** menu into CPA Management Panel.
- **CGO Panic Hardening**: CGO boundary panic recovery prevents crashes of the host CLIProxyAPI core.
- **Reasoning Models Compatibility**: Automatically handles models requiring `max_completion_tokens` (e.g., `o1`, `o3`).
- **Auto Channel & Model Discovery**: Uses host callbacks (`host.auth.models` and `host.auth.list`) to list all upstream accounts and models (OpenAI, Claude, Gemini, Codex, DeepSeek, etc.).
- **Batch & Single Speed Testing**:
  - Top **Batch Test** button triggers concurrent or serial backend testing.
  - Model card right **Test** button for individual re-tests.
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
