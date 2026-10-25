# CPA 模型延迟测速插件 (cpa-models-speed-test)

专为 [CLIProxyAPI (CPA)](https://github.com/router-for-me/CLIProxyAPI) 打造的高性能模型延迟基准测试插件。深度集成至 CPA 管理面板中，提供类似代理软件（如 Clash、v2rayN、Clash Verge）的直观测速体验。

---

## 🌟 核心特性

- 🚀 **管理面板无缝集成**：作为 CPA 原生动态库插件运行，自动在 CPA Web 管理面板注入 **【模型测速】** 菜单，开箱即用。
- 📋 **全渠道模型自动探测**：自动通过宿主回调（`host.auth.models` 和 `host.auth.list`）拉取所有已配置的上游渠道账号（OpenAI、Claude、Gemini、Codex、DeepSeek 等）及可用模型列表，包含渠道账号名称和提供商类型。
- ⚡ **一键并发测速与单项测速**：
  - 顶部 **【一键测速】** 按钮触发后端全局高并发测试；
  - 每个模型右侧提供 **【单独测速】** 按钮，便于针对单个节点快速复测。
- ⏱️ **流式轻量探测 (TTFT) 与极速断开**：
  - 测速请求开启 `stream: true` 并发送最简 Prompt（请求仅生成 1 个 Token）；
  - 在收到官方返回首个 Token / 数据块时**立即主动断开连接**，测得最精准的首字延迟（Time To First Token），同时**绝不浪费账号额度与 Token**。
- 🛡️ **后端代发与规避 CORS**：所有测速请求由 CPA 后端统一发起并高精度计算耗时，严禁浏览器直连，完全规避跨域限制。
- 🏎️ **Goroutine 高并发请求**：后端接收指令后，采用 Goroutine + 信号量并发池并行请求各个上游渠道，杜绝多账号串行排队。
- 🎨 **色彩分级高亮与状态反馈**：
  - **绿灯优**：`TTFT < 800ms`
  - **黄灯良**：`800ms ≤ TTFT ≤ 1500ms`
  - **橙灯差**：`TTFT > 1500ms`
  - **紫灯限流**：触发官方限流或额度耗尽（`Quota Limit / HTTP 429`）
  - **红灯失败**：网络异常或鉴权失败（`Error`）
- 🎛️ **双视图与丰富筛选**：
  - 支持 **卡片网格**（类似 Clash 节点卡片）与 **紧凑表格**（类似 v2rayN 列表）一键切换；
  - 支持按渠道类型（Provider）、测试状态（正常/限流/失败/未测）筛选及按延迟高低排序；
  - 支持一键导出 Markdown 格式测速报告。
- 🤖 **标准化 WebMCP 调试接口**：
  - 基于 Vue 3 Composition API 与 VueUse (`@vueuse/core`) 构建；
  - 遵循 WebMCP 规范将核心状态、筛选和测速方法暴露至 `window.__web_mcp__`，支持浏览器端 AI Agent 及控制台直接调用调试。

---

## 🏗️ 架构与测速流程

```
浏览器管理面板 (Vue 3 + VueUse)
       │
       │ 1. 触发【一键测速】 POST /v0/management/plugins/cpa-models-speed-test/batch-test
       ▼
CPA 插件进程 (Go + Goroutines)
       │
       ├─► Goroutine 1 (Channel A / Model 1) ──► host.model.execute_stream (stream: true)
       ├─► Goroutine 2 (Channel B / Model 2) ──► host.model.execute_stream (stream: true)
       └─► Goroutine N (Channel C / Model N) ──► host.model.execute_stream (stream: true)
                                                        │
                      ┌─────────────────────────────────┘
                      ▼
               收到第 1 个 Token
                      │
                      ├─► 记录精确 TTFT (毫秒)
                      ├─► 立即调用 host.model.stream_close 主动关闭流 (节省 Token)
                      └─► 返回结果给前端高亮呈现
```

---

## 📦 安装与配置

### 方式一：从 CPA 插件商店一键安装（推荐）

本插件遵循 CPA 官方动态库插件规范与商店发布规范。

#### 1. 提交官方商店或添加插件源
若已收录至官方插件商店（或配置了自定义源），在 CPA `config.yaml` 中配置插件源：
```yaml
plugins:
  enabled: true
  dir: "plugins"
  store_sources:
    - "https://raw.githubusercontent.com/router-for-me/CLIProxyAPI-Plugins-Store/main/registry.json"
```

> **Registry 索引信息**：
> ```json
> {
>   "id": "cpa-models-speed-test",
>   "name": "CPA 模型延迟测速",
>   "description": "在管理面板中一键并发批量测试所有渠道模型的首字延迟 (TTFT) 与可用性，提供类似代理软件的直观测速体验。",
>   "author": "cinob",
>   "version": "1.0.0",
>   "repository": "https://github.com/cinob/cpa-models-speed-test",
>   "homepage": "https://github.com/cinob/cpa-models-speed-test",
>   "license": "MIT",
>   "tags": ["Management", "SpeedTest", "TTFT", "Benchmark", "Monitoring"]
> }
> ```

#### 2. 在 Web 管理面板中安装
1. 打开 CPA Web 管理面板（`http://localhost:8080/management.html`）。
2. 点击左侧菜单 **【插件商店】**。
3. 找到 **CPA 模型延迟测速 (cpa-models-speed-test)**，点击 **【安装】**。
4. CPA 将自动识别当前系统架构（Linux / macOS / Windows / amd64 / arm64），下载对应动态库、校验 SHA-256、解压至 `plugins/` 目录并自动激活配置。

---

### 方式二：手动编译与部署

#### 1. 放置插件动态库

将编译好的动态库文件放入 CPA 的 `plugins/` 目录中：

| 操作系统 | 文件放置路径 |
| :--- | :--- |
| **Linux** | `plugins/cpa-models-speed-test.so` |
| **macOS** | `plugins/cpa-models-speed-test.dylib` |
| **Windows** | `plugins/cpa-models-speed-test.dll` |

> 也可放置在平台架构子目录下，例如：`plugins/linux/amd64/cpa-models-speed-test.so`。

#### 2. 在 CPA 配置中启用插件

在 CPA 的 `config.yaml` 配置文件中添加插件配置：

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    cpa-models-speed-test:
      enabled: true
      priority: 1
      concurrency: 20  # 并发测速 Goroutine 数量限制 (默认 20)
```

启动 CPA 服务后，控制台将输出插件注册成功日志。在浏览器打开 CPA 管理中心（例如 `http://localhost:8080/management.html`），左侧菜单将出现 **【模型测速】** 入口。

---

## 🛠️ 编译与打包

本插件前端采用独立内嵌单文件打包技术，前端 HTML、CSS、JS（Vue 3 + VueUse）全部自动内嵌于动态库中，**无需外部 CDN 网络请求，完全支持离线和内网环境**。

### 编译步骤

1. **打包前端静态资产**（需安装 Bun 或 Node.js）：
   ```bash
   cd web
   bun install
   bun run build
   cd ..
   ```
   前端单文件将被构建至 `web/dist/index.html`。

2. **编译 Go CGO 插件共享库**：
   ```bash
   # Linux (.so)
   go build -buildmode=c-shared -o bin/cpa-models-speed-test.so .

   # macOS (.dylib)
   go build -buildmode=c-shared -o bin/cpa-models-speed-test.dylib .

   # Windows (.dll)
   go build -buildmode=c-shared -o bin/cpa-models-speed-test.dll .
   ```
   也可以直接执行根目录 Makefile：
   ```bash
   make all
   ```

---

## 🌐 接口列表

严格遵循 CPA 插件安全规范，动态操作均走 Management API（需携带管理密钥），Resource 路由仅用于静态界面呈现：

| 方法 | 路由路径 | 说明 | 鉴权要求 |
| :--- | :--- | :--- | :--- |
| `GET` | `/v0/resource/plugins/cpa-models-speed-test/speedtest` | 模型测速主面板界面 (HTML 静态资产) | 无需鉴权 |
| `GET` | `/v0/management/plugins/cpa-models-speed-test/channels` | 获取后端配置的渠道与模型列表 | CPA 管理密钥 |
| `POST` | `/v0/management/plugins/cpa-models-speed-test/test` | 对单个模型发起后端代发流式测速 | CPA 管理密钥 |
| `POST` | `/v0/management/plugins/cpa-models-speed-test/batch-test` | 后端并发 / 串行批量测速 | CPA 管理密钥 |

---

## 🤖 WebMCP 调试接口

在测速页面中按 `F12` 打开控制台，插件已挂载标准化 WebMCP 调试对象 `window.__web_mcp__`：

```javascript
// 触发全局并发测速
await window.__web_mcp__.runAll();

// 单独测速指定模型
await window.__web_mcp__.runSingle("channel-id", "gpt-4o");

// 获取当前所有测速数据
console.table(window.__web_mcp__.getResults());

// 获取当前统计看板汇总指标
console.log(window.__web_mcp__.getStats());
```

---

## 📄 开源许可证

MIT License.
