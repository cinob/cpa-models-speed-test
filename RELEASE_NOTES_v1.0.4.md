# Release v1.0.4: Full CPA Internationalization (i18n), CGO Crash Safety, and Reasoning Models Support

A major stability and usability release for the `cpa-models-speed-test` plugin, bringing full parity with CLIProxyAPI's multi-language suite, CGO process hardening, and compatibility fixes for reasoning models.

---

### 🌐 1. Full Internationalization Suite (7 Languages)
- **Full CPA Language Parity**: Integrated native translation dictionaries for all 7 languages supported by CLIProxyAPI / CPA Manager Plus:
  - 🇬🇧 **English (`en`)** — Default
  - 🇷🇺 **Русский (`ru`)**
  - 🇨🇳 **简体中文 (`zh-CN`)**
  - 🇹🇼 **繁體中文 (`zh-TW`)**
  - 🇻🇳 **Tiếng Việt (`vi`)**
  - 🇰🇷 **한국어 (`ko`)**
  - 🇹🇷 **Türkçe (`tr`)**
- **Zero-Dependency Reactive Engine**: Lightweight custom i18n implementation (`web/src/i18n/`) without external runtime bloat, inlined into the single-file Vite HTML bundle.
- **Strict English Default**: The plugin defaults strictly to English (`en`) for any unrecognized language, unsupported browser locale (German, French, Spanish, etc.), or unset state.
- **Bi-Directional Host Sync**:
  - Automatically reads `cli-proxy-language` from host `localStorage` (both root and same-origin parent iframe).
  - Actively listens for `window.addEventListener('storage')` and `message` events: when the user switches languages in the main CPA Manager Plus dashboard, the plugin UI updates instantly without requiring a page reload.
  - Supports explicit URL query overrides (`?lang=ru`, `?locale=en`).
- **Dynamic Document Metadata**:
  - Reactive `watch` dynamically updates `document.title` and `document.documentElement.lang` based on active locale (e.g., `CPA Model Latency Benchmark`, `CPA Бенчмарк Задержки Моделей`, `CPA 模型延迟测速`).
  - Fixed hardcoded `<title>CPA 模型测速面板</title>` in `index.html` preventing unwanted Chinese titles in browser tabs, history, and bookmarks.
- **Responsive Language Selector**:
  - Clean, unified language dropdown in the navigation actions toolbar (`.nav-actions`).
  - Removed duplicate selector from the brand title row to avoid visual redundancy.
  - Enabled `flex-wrap: wrap` and flexible spacing in `.nav-actions` and `.top-nav`, ensuring the selector and management controls remain accessible and never clip off-screen on tablets, mobile devices, laptops, or narrow split panes.

---

### 🛡️ 2. Core Stability & CGO Process Hardening
- **CGO ABI Boundary Panic Recovery**:
  - Exported CGO entry point `cliproxyPluginCall` is wrapped in `defer recover()`, formatting fatal panics into structured JSON envelopes (`{"ok": false, "error": {"code": "plugin_panic"}}`).
  - Prevents unhandled Go runtime panics from crossing the CGO ABI boundary into the C host runtime, eliminating host-wide `SIGABRT` crashes of the CLIProxyAPI daemon.
- **Worker Goroutine Panic Isolation**:
  - In `probeSingleModel`, background worker goroutines are guarded with panic handlers to ensure hung channels or unexpected nil pointers report `Status: Error` rather than crashing the thread pool.
- **Immediate Streaming Cleanup**:
  - Guarantees immediate stream termination via `MethodHostModelStreamClose` as soon as the first token chunk arrives, ensuring TTFT accuracy while preventing socket leaks and token waste.

---

### ⚡ 3. Accuracy & Model Compatibility Fixes
- **Upstream Quota Detection False-Positive Fix (`isQuotaError`)**:
  - Resolved an upstream bug where matching substring `"exceeded"` caused normal network/context timeouts (`"context deadline exceeded"`) to be falsely classified as HTTP 429 quota exhaustion.
  - Added explicit exclusion for `"deadline exceeded"` and refined checks to genuine billing/rate-limit signals (`"quota exceeded"`, `"rate limit"`, `"resource_exhausted"`, etc.).
- **OpenAI Reasoning Models (`o1`, `o3`, etc.) Support**:
  - Addressed HTTP 400 `BadRequestError` on newer reasoning models (e.g., `o1`, `o1-mini`, `o1-preview`, `o3`, `o3-mini`) which forbid the legacy `max_tokens` parameter.
  - Prober automatically selects `max_completion_tokens: 1` when probing reasoning architectures.
- **Backend Neutral Language**:
  - Resource routes and config field descriptions in `main.go` now register in standard English (`"Speed Test"`, `"Concurrent probe worker limit"`, `"Model is disabled"`).

---

### 📦 4. Build System & Packaging
- **Toolchain Alignment**: Bumped `go.mod` to Go 1.26 to match CLIProxyAPI v8 SDK specifications; committed missing `go.sum`.
- **Hybrid Build Automation**: `Makefile` supports both `bun` and `npm` for frontend compilation, with automated CGO shared library stripping (`-s -w -buildvcs=false`).
- **Comprehensive Unit Tests**: Full 9-test suite passing cleanly in CI (`0.011s`).
- **Multi-Platform Releases**: Precompiled standalone `.zip` artifacts available for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`.
