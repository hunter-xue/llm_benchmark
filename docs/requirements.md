# 需求描述文档

**项目**：embedding_benchmark
**文档版本**：v2.2
**最后更新**：2026-03-26

---

## 1. 项目背景

针对 OpenAI 兼容 API 的性能压测工具，用于评估不同 LLM Provider 在 Embedding 和 Chat Completion 场景下的吞吐量、延迟等关键性能指标。

---

## 2. 功能需求

### 2.1 API 类型支持

| ID | 需求 | 状态 |
|----|------|------|
| F-01 | 支持 Embedding API（`POST /v1/embeddings`）压测 | ✅ 已实现 |
| F-02 | 支持 Chat Completion API（`POST /v1/chat/completions`）流式压测 | ✅ 已实现 |
| F-03 | 所有 UI 文本以英文显示，确保在不支持中文的 Linux 终端上正常渲染 | ✅ 已实现 |

### 2.2 测试模式

| ID | 需求 | 状态 |
|----|------|------|
| F-10 | 支持**单 Provider 模式**：对单个 API 端点进行压测 | ✅ 已实现 |
| F-11 | 支持 **PK 模式（双 Provider 对比）**：同时对两个 Provider 发起压测 | ✅ 已实现 |
| F-12 | PK 模式中两个 Provider 的 name、base URL、model、API key 各自独立配置 | ✅ 已实现 |
| F-13 | PK 模式中并发数、总请求数、输入 token 数、max tokens、system prompt 等参数对两个 Provider 保持一致，确保测试公平性 | ✅ 已实现 |
| F-14 | PK 模式下两个 Provider 同时启动压测 | ✅ 已实现 |

### 2.3 TUI 交互界面

| ID | 需求 | 状态 |
|----|------|------|
| F-20 | 使用 [bubbletea](https://github.com/charmbracelet/bubbletea) 框架实现 TUI 界面 | ✅ 已实现 |
| F-21 | 启动后用户首先选择 API 类型（Embedding / Chat Completion） | ✅ 已实现 |
| F-22 | 选择 API 类型后，选择测试模式（Single Provider / PK Mode / Response Compare） | ✅ 已实现 |
| F-23 | 通过表单界面输入压测参数，`tab` / `shift+tab` 切换字段 | ✅ 已实现 |
| F-24 | 输入框以 placeholder 形式展示示例值，字段初始为空 | ✅ 已实现 |
| F-25 | `ctrl+s` 提交配置并启动压测 | ✅ 已实现 |
| F-26 | `esc` 返回上一步，`ctrl+c` 全局退出，各界面快捷键提示保持一致 | ✅ 已实现 |

### 2.4 压测执行与进度展示

| ID | 需求 | 状态 |
|----|------|------|
| F-30 | 压测过程中显示实时进度（已完成请求数 / 总数、错误数） | ✅ 已实现 |
| F-31 | 显示进度条（单 Provider 一条，PK 模式每个 Provider 各一条） | ✅ 已实现 |
| F-32 | 显示转动的 spinner 表示测试正在进行 | ✅ 已实现 |
| F-33 | `esc` 可取消正在进行的压测并返回配置界面 | ✅ 已实现 |

### 2.5 结果展示

| ID | 需求 | 状态 |
|----|------|------|
| F-40 | 单 Provider 模式以表格形式展示所有性能指标 | ✅ 已实现 |
| F-41 | PK 模式以三列表格并排展示两个 Provider 的指标（Metric \| Provider A \| Provider B） | ✅ 已实现 |
| F-42 | PK 模式中每行的优胜值（数值更优的一方）以绿色高亮显示 | ✅ 已实现 |
| F-43 | 判断优劣方向：RPS、TPS、TPM 越高越好；延迟（Latency、TTFT、TPOT、E2E）越低越好 | ✅ 已实现 |
| F-44 | 若某 Provider 所有请求均失败（无有效指标），对应列显示"N/A" | ✅ 已实现 |
| F-45 | 结果页按 `r` 返回配置界面重新运行 | ✅ 已实现 |

### 2.6 错误日志

| ID | 需求 | 状态 |
|----|------|------|
| F-50 | 压测过程中记录所有请求错误及其出现次数 | ✅ 已实现 |
| F-51 | 压测中或结果页，有错误时提示用户按 `e` 查看错误日志 | ✅ 已实现 |
| F-52 | 错误日志以可滚动的浮层（overlay）展示，使用 `bubbles/viewport` 组件 | ✅ 已实现 |
| F-53 | 错误日志按出现次数降序排列 | ✅ 已实现 |
| F-54 | 按 `esc` 或再次按 `e` 关闭错误日志浮层 | ✅ 已实现 |

### 2.7 性能指标计算

| ID | 需求 | 状态 |
|----|------|------|
| F-60 | Embedding：统计 RPS、Input TPS、Input TPM、E2E Latency（Avg/P50/P90/P99） | ✅ 已实现 |
| F-61 | Completion：统计 RPS、Input/Output TPS、Input/Output TPM、Avg Output Tokens、TTFT、TPOT、E2E（各含 Avg/P50/P90/P99） | ✅ 已实现 |
| F-62 | 输入 / 输出 token 数均通过 tiktoken（cl100k_base）本地计数，不依赖 API 响应中的 `usage` 字段 | ✅ 已实现 |
| F-63 | TPOT = `(E2E - TTFT) / (输出 token 数 - 1)`；输出仅 1 token 时退化为 E2E | ✅ 已实现 |
| F-64 | 百分位数使用最近秩（nearest-rank）方法：`idx = ceil(n × p) - 1` | ✅ 已实现 |
| F-65 | 吞吐量统计基于整体墙钟时间（wall time），反映真实并发吞吐能力 | ✅ 已实现 |

### 2.8 Token 生成与离线要求

| ID | 需求 | 状态 |
|----|------|------|
| F-70 | BPE 词表文件（`cl100k_base.tiktoken`）必须本地可访问，工具不得联网下载 | ✅ 已实现 |
| F-71 | 根据目标 token 数精确生成等长测试文本，确保每次请求输入 token 数一致 | ✅ 已实现 |
| F-72 | 通过 `--bpe-file` 参数指定 BPE 文件路径，默认为 `./cl100k_base.tiktoken` | ✅ 已实现 |
| F-73 | 启动时若 BPE 文件不存在，输出明确错误信息（包含文件路径和使用方式提示）后退出 | ✅ 已实现 |

### 2.9 Response Compare（响应质量对比）

| ID | 需求 | 状态 |
|----|------|------|
| F-80 | 提供独立的 Response Compare 模式入口，仅在选择 Chat Completion 时显示 | ✅ 已实现 |
| F-81 | 用户独立配置两个 Provider（Name、URL、API Key、Model）及请求内容（User Message、System Prompt） | ✅ 已实现 |
| F-82 | 向两个 Provider 各发送一次非流式请求（`"stream": false`），使用 API 默认输出长度限制 | ✅ 已实现 |
| F-83 | 两个请求并发发出，互不等待 | ✅ 已实现 |
| F-84 | 响应以原始 JSON 格式（`json.MarshalIndent` 美化）左右分栏展示 | ✅ 已实现 |
| F-85 | 每列使用独立的可滚动 viewport，`tab` 切换焦点，`↑/↓/pgup/pgdn` 滚动当前面板 | ✅ 已实现 |
| F-86 | 请求进行中在对应面板显示 spinner 动画及提示文字，避免用户误认为界面卡死；请求完成后自动填充内容 | ✅ 已实现 |
| F-87 | `esc` 返回配置界面，允许修改参数后重新发送 | ✅ 已实现 |
| F-88 | Response Compare 及压测模式（Single / PK）均支持 **Custom Params**：每个 Provider 可独立填写可选 JSON 对象，key-value 会合并到请求体中，覆盖同名标准字段；支持 OpenAI 标准字段（如 `temperature`）及任意非标准字段（如 `enable_thinking`）；留空则不影响请求 | ✅ 已实现 |

---

## 3. 非功能需求

| ID | 需求 | 状态 |
|----|------|------|
| NF-01 | 跨平台支持：macOS（amd64/arm64）、Linux（amd64/arm64）、Windows（amd64/arm64）；提供 Makefile 一键构建，产物含 BPE 文件，解压即用 | ✅ 已实现 |
| NF-02 | 使用 Go 标准并发模型（goroutine + channel）实现并发压测，无第三方并发框架依赖 | ✅ 已实现 |
| NF-03 | Completion 模式 HTTP 超时 300s，Embedding 模式 120s | ✅ 已实现 |
| NF-04 | 非 200 响应错误信息包含最多 512 字节的响应体，便于排查认证、限流问题 | ✅ 已实现 |
| NF-05 | SSE 解析失败的 chunk 单独计数（SkippedChunks），不影响成功请求的指标统计 | ✅ 已实现 |
| NF-06 | 压测逻辑（`internal/bench`）与 TUI 逻辑（`internal/tui`）完全解耦，bench 包可独立复用 | ✅ 已实现 |

---

## 4. 配置参数

### 4.1 启动参数（CLI flag）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--bpe-file` | `./cl100k_base.tiktoken` | 本地 BPE 词表文件路径 |

### 4.2 TUI 表单参数（单 Provider）

| 字段 | 适用模式 | 说明 |
|------|----------|------|
| API URL | 全部 | 完整 API 端点地址 |
| API Key | 全部 | Bearer Token，留空则不添加 Authorization 头 |
| Model | 全部 | 模型名称 |
| Custom Params | 全部 | 可选 JSON 对象，合并至请求体（如 `{"temperature":0.7}`） |
| Concurrency | 全部 | 并发 worker 数，须 > 0 |
| Total Requests | 全部 | 总请求数，须 > 0 |
| Input Tokens | 全部 | 每请求输入 token 数，须 > 0 |
| Max Output Tokens | Completion | 最大输出 token 数，0 表示不限制 |
| System Prompt | Completion | 可选的 system 消息内容 |

### 4.3 TUI 表单参数（PK 模式）

Provider A / B 各自独立配置：Name、URL、API Key、Model、Custom Params；其余参数与单 Provider 模式相同，两侧共享。

### 4.4 TUI 表单参数（Response Compare 模式）

| 字段 | 必填 | 说明 |
|------|------|------|
| Provider A Name | 否 | 显示名称 |
| Provider A URL | 是 | 完整 API 端点地址 |
| Provider A API Key | 否 | Bearer Token |
| Provider A Model | 是 | 模型名称 |
| Custom Params | 否 | 可选 JSON 对象，合并至 Provider A 的请求体 |
| Provider B Name | 否 | 显示名称 |
| Provider B URL | 是 | 完整 API 端点地址 |
| Provider B API Key | 否 | Bearer Token |
| Provider B Model | 是 | 模型名称 |
| Custom Params | 否 | 可选 JSON 对象，合并至 Provider B 的请求体 |
| User Message | 是 | 发送给两个 Provider 的用户消息 |
| System Prompt | 否 | 可选 system 消息，留空跳过 |

输出长度不设限制，使用各 Provider API 的默认值。

---

## 5. 界面流程

```
启动
  └─ 选择 API 类型（Embedding / Chat Completion）
       └─ 选择测试模式
            ├─ Single Provider / PK Mode
            │    └─ 配置参数表单
            │         └─ 压测进行中（进度条 + spinner）
            │              └─ 结果展示
            │                   ├─ [r] 返回配置重新测试
            │                   └─ [esc] 返回上级菜单
            └─ Response Compare（仅 Chat Completion）
                 └─ 配置两个 Provider + 请求内容
                      └─ 左右分栏响应展示
                           └─ [esc] 返回配置
```

---

## 6. 变更记录

| 版本 | 日期 | 变更摘要 |
|------|------|----------|
| v1.0 | 2025 | 初版 CLI 工具，支持 Embedding 和 Chat Completion 单 Provider 压测 |
| v2.0 | 2026-03-25 | 重构为 TUI 界面（bubbletea）；新增 PK 双 Provider 对比模式；新增错误日志 viewport；全部 UI 文本改为英文；benchmark 逻辑拆分至 `internal/bench` 包 |
| v2.1 | 2026-03-25 | 新增 Response Compare 独立模式（双 Provider 响应质量对比，左右分栏 viewport）；添加 Makefile 多平台一键构建，产物子目录含 BPE 文件；启动时 BPE 文件缺失给出明确错误提示 |
| v2.2 | 2026-03-26 | 新增 Custom Params（每 Provider 独立 JSON，合并至请求体，支持任意标准/非标准字段）；压测 / PK / Response Compare 三种模式均已支持；Response Compare 等待期间显示 spinner；结果页 `esc` 返回上级菜单（统一全局导航：`esc` 返回，`ctrl+c` 退出）；Completion 结果展示中 TTFT / TPOT / E2E 三组指标之间添加分隔线 |
