# 需求描述文档

**项目**：embedding_benchmark
**文档版本**：v2.3
**最后更新**：2026-03-28

---

## 1. 项目背景

针对 OpenAI 兼容 API 及 Anthropic Messages API 的性能压测工具，用于评估不同 LLM Provider 在 Embedding、Chat Completion、Anthropic Messages 场景下的吞吐量、延迟等关键性能指标。

---

## 2. 功能需求

### 2.1 API 类型支持

| ID | 需求 | 状态 |
|----|------|------|
| F-01 | 支持 Embedding API（`POST /v1/embeddings`）压测 | ✅ 已实现 |
| F-02 | 支持 Chat Completion API（`POST /v1/chat/completions`）流式压测 | ✅ 已实现 |
| F-03 | 所有 UI 文本以英文显示，确保在不支持中文的 Linux 终端上正常渲染 | ✅ 已实现 |
| F-04 | 支持 Anthropic Messages API（`POST /v1/messages`），使用 `x-api-key` + `anthropic-version` 头鉴权；SSE 解析适配 `event: content_block_delta` / `message_stop`；`max_tokens` 为必填字段，默认 4096；功能与 Chat Completion 完全一致 | ✅ 已实现 |

### 2.2 测试模式

| ID | 需求 | 状态 |
|----|------|------|
| F-10 | 支持**单 Provider 模式**：对单个 API 端点进行压测 | ✅ 已实现 |
| F-11 | 支持 **PK 模式（双 Provider 对比）**：同时对两个 Provider 发起压测 | ✅ 已实现 |
| F-12 | PK 模式中两个 Provider 的 name、base URL、model、API key 各自独立配置 | ✅ 已实现 |
| F-13 | PK 模式中并发数、总请求数、输入 token 数、max tokens、system prompt 等参数对两个 Provider 保持一致，确保测试公平性 | ✅ 已实现 |
| F-14 | PK 模式下两个 Provider 同时启动压测 | ✅ 已实现 |
| F-15 | 支持 **Single Response View 模式**（仅 Chat Completion / Anthropic Messages）：向单个 Provider 发送一条 Prompt，查看原始 JSON 响应及 HTTP 响应头；使用非流式请求 | ✅ 已实现 |

### 2.3 TUI 交互界面

| ID | 需求 | 状态 |
|----|------|------|
| F-20 | 使用 [bubbletea](https://github.com/charmbracelet/bubbletea) 框架实现 TUI 界面 | ✅ 已实现 |
| F-21 | 启动后用户首先选择 API 类型（Embedding / Chat Completion / Anthropic Messages） | ✅ 已实现 |
| F-22 | 选择 API 类型后，选择测试模式；Chat Completion / Anthropic Messages 显示 4 项（Single Provider / Single Response View / PK Mode / Response Compare），Embedding 显示 2 项（Single Provider / PK Mode） | ✅ 已实现 |
| F-23 | 通过表单界面输入压测参数，`tab` / `shift+tab` 切换字段 | ✅ 已实现 |
| F-24 | 输入框以 placeholder 形式展示示例值，字段初始为空；占位符根据 API 类型自动切换（如 Anthropic 显示 `sk-ant-...`、`https://api.anthropic.com/v1/messages`） | ✅ 已实现 |
| F-25 | `ctrl+s` 提交配置并启动压测 / 发送请求 | ✅ 已实现 |
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
| F-46 | Completion / Anthropic Messages 结果展示中，TTFT / TPOT / E2E 三组指标之间添加分隔线 | ✅ 已实现 |

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
| F-61 | Completion / Anthropic Messages：统计 RPS、Input/Output TPS、Input/Output TPM、Avg Output Tokens、TTFT、TPOT、E2E（各含 Avg/P50/P90/P99） | ✅ 已实现 |
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
| F-80 | 提供独立的 Response Compare 模式入口，仅在选择 Chat Completion / Anthropic Messages 时显示 | ✅ 已实现 |
| F-81 | 用户独立配置两个 Provider（Name、URL、API Key、Model）及请求内容（User Message、System Prompt） | ✅ 已实现 |
| F-82 | 向两个 Provider 各发送一次非流式请求（`"stream": false`） | ✅ 已实现 |
| F-83 | 两个请求并发发出，互不等待 | ✅ 已实现 |
| F-84 | 响应以原始 JSON 格式（`json.MarshalIndent` 美化）左右分栏展示；响应头显示在 JSON body 上方，中间以分隔线分隔 | ✅ 已实现 |
| F-85 | 每列使用独立的可滚动 viewport；使用 vim 风格快捷键：`j/k` 逐行滚动，`ctrl+d/u` 翻半页；`tab` 切换面板焦点 | ✅ 已实现 |
| F-86 | 请求进行中在对应面板显示 spinner 动画及提示文字，避免用户误认为界面卡死；请求完成后自动填充内容 | ✅ 已实现 |
| F-87 | `esc` 返回配置界面，允许修改参数后重新发送 | ✅ 已实现 |
| F-88 | Response Compare 及压测模式（Single / PK）均支持 **Custom Params**：每个 Provider 可独立填写可选 JSON 对象，key-value 会合并到请求体中，覆盖同名标准字段；支持 OpenAI 标准字段（如 `temperature`）及任意非标准字段（如 `enable_thinking`）；留空则不影响请求 | ✅ 已实现 |
| F-89 | Response Compare 及 Single Response View 展示 HTTP 响应头（状态行 + 所有响应头按字母序排列），位于 JSON body 上方，以分隔线分隔 | ✅ 已实现 |

### 2.10 Single Response View（单 Provider 响应查看）

| ID | 需求 | 状态 |
|----|------|------|
| F-90 | 提供独立的 Single Response View 模式入口，仅在选择 Chat Completion / Anthropic Messages 时显示，位于 Single Provider 之后 | ✅ 已实现 |
| F-91 | 配置界面包含：API URL、API Key、Model、Custom Params、User Message、System Prompt；无需输入 Provider Name | ✅ 已实现 |
| F-92 | 向 Provider 发送一次非流式请求（`"stream": false`）；等待期间显示 spinner | ✅ 已实现 |
| F-93 | 响应以全宽可滚动 viewport 展示，内容格式与 Response Compare 一致（响应头 + 分隔线 + JSON body） | ✅ 已实现 |
| F-94 | 使用 vim 风格快捷键：`j/k` 逐行滚动，`ctrl+d/u` 翻半页；`esc` 返回配置界面 | ✅ 已实现 |

### 2.11 结果导出

| ID | 需求 | 状态 |
|----|------|------|
| F-95 | 在压测结果页、Response Compare 页、Single Response View 页，按 `ctrl+e` 触发导出 | ✅ 已实现 |
| F-96 | 触发后在底部显示文件名输入框；`enter` 保存，`esc` 取消 | ✅ 已实现 |
| F-97 | 导出内容为纯文本（去除 ANSI 颜色码），格式与屏幕显示一致 | ✅ 已实现 |
| F-98 | 保存成功后显示 `✓ saved → <filename>` 提示；保存失败显示错误信息；导航离开当前页后提示自动消失 | ✅ 已实现 |

---

## 3. 非功能需求

| ID | 需求 | 状态 |
|----|------|------|
| NF-01 | 跨平台支持：macOS（amd64/arm64）、Linux（amd64/arm64）、Windows（amd64/arm64）；提供 Makefile 一键构建，产物含 BPE 文件，解压即用 | ✅ 已实现 |
| NF-02 | 使用 Go 标准并发模型（goroutine + channel）实现并发压测，无第三方并发框架依赖 | ✅ 已实现 |
| NF-03 | Completion / Anthropic Messages 模式 HTTP 超时 300s，Embedding 模式 120s | ✅ 已实现 |
| NF-04 | 非 200 响应错误信息包含最多 512 字节的响应体，便于排查认证、限流问题 | ✅ 已实现 |
| NF-05 | SSE 解析失败的 chunk 单独计数（SkippedChunks），不影响成功请求的指标统计 | ✅ 已实现 |
| NF-06 | 压测逻辑（`internal/bench`）与 TUI 逻辑（`internal/tui`）完全解耦，bench 包可独立复用 | ✅ 已实现 |

---

## 4. 配置参数

### 4.1 启动参数（CLI flag）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--bpe-file` | `./cl100k_base.tiktoken` | 本地 BPE 词表文件路径 |

### 4.2 TUI 表单参数（单 Provider 压测）

| 字段 | 适用模式 | 说明 |
|------|----------|------|
| API URL | 全部 | 完整 API 端点地址 |
| API Key | 全部 | Bearer Token（Anthropic 为 `x-api-key`），留空则不添加鉴权头 |
| Model | 全部 | 模型名称 |
| Custom Params | 全部 | 可选 JSON 对象，合并至请求体（如 `{"temperature":0.7}`） |
| Concurrency | 全部 | 并发 worker 数，须 > 0 |
| Total Requests | 全部 | 总请求数，须 > 0 |
| Input Tokens | 全部 | 每请求输入 token 数，须 > 0 |
| Max Output Tokens | Completion / Anthropic | 最大输出 token 数；0 表示不限制（Chat Completion）或默认 4096（Anthropic） |
| System Prompt | Completion / Anthropic | 可选的 system 消息内容 |

### 4.3 TUI 表单参数（PK 模式）

Provider A / B 各自独立配置：Name、URL、API Key、Model、Custom Params；其余参数与单 Provider 模式相同，两侧共享。

### 4.4 TUI 表单参数（Response Compare 模式）

| 字段 | 必填 | 说明 |
|------|------|------|
| Provider A Name | 否 | 显示名称 |
| Provider A URL | 是 | 完整 API 端点地址 |
| Provider A API Key | 否 | 鉴权 Token |
| Provider A Model | 是 | 模型名称 |
| Custom Params | 否 | 可选 JSON 对象，合并至 Provider A 的请求体 |
| Provider B Name | 否 | 显示名称 |
| Provider B URL | 是 | 完整 API 端点地址 |
| Provider B API Key | 否 | 鉴权 Token |
| Provider B Model | 是 | 模型名称 |
| Custom Params | 否 | 可选 JSON 对象，合并至 Provider B 的请求体 |
| User Message | 是 | 发送给两个 Provider 的用户消息 |
| System Prompt | 否 | 可选 system 消息，留空跳过 |

### 4.5 TUI 表单参数（Single Response View 模式）

| 字段 | 必填 | 说明 |
|------|------|------|
| API URL | 是 | 完整 API 端点地址 |
| API Key | 否 | 鉴权 Token |
| Model | 是 | 模型名称 |
| Custom Params | 否 | 可选 JSON 对象，合并至请求体 |
| User Message | 是 | 发送的用户消息 |
| System Prompt | 否 | 可选 system 消息，留空跳过 |

---

## 5. 界面流程

```
启动
  └─ 选择 API 类型（Embedding / Chat Completion / Anthropic Messages）
       └─ 选择测试模式
            ├─ Single Provider / PK Mode
            │    └─ 配置参数表单
            │         └─ 压测进行中（进度条 + spinner）
            │              └─ 结果展示
            │                   ├─ [r] 返回配置重新测试
            │                   ├─ [ctrl+e] 导出结果到文件
            │                   └─ [esc] 返回上级菜单
            ├─ Single Response View（仅 Chat Completion / Anthropic Messages）
            │    └─ 配置表单（URL / Key / Model / Custom Params / Prompt）
            │         └─ spinner 等待响应
            │              └─ 响应头 + JSON body 全宽 viewport
            │                   ├─ [ctrl+e] 导出响应到文件
            │                   └─ [esc] 返回配置
            └─ Response Compare（仅 Chat Completion / Anthropic Messages）
                 └─ 配置两个 Provider + 请求内容
                      └─ 左右分栏响应展示（响应头 + JSON body）
                           ├─ [ctrl+e] 导出响应到文件
                           └─ [esc] 返回配置
```

---

## 6. 变更记录

| 版本 | 日期 | 变更摘要 |
|------|------|----------|
| v1.0 | 2025 | 初版 CLI 工具，支持 Embedding 和 Chat Completion 单 Provider 压测 |
| v2.0 | 2026-03-25 | 重构为 TUI 界面（bubbletea）；新增 PK 双 Provider 对比模式；新增错误日志 viewport；全部 UI 文本改为英文；benchmark 逻辑拆分至 `internal/bench` 包 |
| v2.1 | 2026-03-25 | 新增 Response Compare 独立模式（双 Provider 响应质量对比，左右分栏 viewport）；添加 Makefile 多平台一键构建，产物子目录含 BPE 文件；启动时 BPE 文件缺失给出明确错误提示 |
| v2.2 | 2026-03-26 | 新增 Custom Params（每 Provider 独立 JSON，合并至请求体）；新增 spinner；Completion 结果三组指标添加分隔线；统一导航快捷键（`esc` 返回，`ctrl+c` 退出） |
| v2.3 | 2026-03-28 | 新增 Anthropic Messages API 支持（完整功能与 Chat Completion 一致）；新增 Single Response View 模式（单 Provider 非流式响应查看）；Response Compare / Single Response View 展示 HTTP 响应头；滚动快捷键改为 vim 风格（`j/k`、`ctrl+d/u`）；所有结果页支持 `ctrl+e` 导出到文本文件 |
