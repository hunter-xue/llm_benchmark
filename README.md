# embedding_benchmark

针对 OpenAI 兼容 API 及 Anthropic Messages API 的交互式压测工具（TUI），支持三种 API 类型：

- **Embedding 模式**：测试 `/v1/embeddings` 端点的吞吐与延迟
- **Chat Completion 模式**：测试 `/v1/chat/completions` 端点的流式生成性能，包含 TTFT、TPOT、E2E 等指标
- **Anthropic Messages 模式**：测试 Anthropic Messages API（`/v1/messages`），使用 `x-api-key` 鉴权，功能与 Chat Completion 完全一致

支持以下测试模式（Embedding 仅支持前两种）：

- **Single Provider**：单 Provider 压测
- **Single Response View**：向单个 Provider 发送一条 Prompt，查看原始 JSON 响应及 HTTP 响应头
- **PK Mode**：两个 Provider 同时压测，结果并排对比，优胜指标绿色高亮
- **Response Compare**：向两个 Provider 发送同一条 Prompt，左右分栏展示原始 JSON 响应及 HTTP 响应头
- **Prompt Cache Hit Test**：仅 Chat Completion 支持，重复发送同一大段 Prompt，并从 `usage.prompt_tokens_details.cached_tokens` 展示缓存命中情况

---

## 依赖

- Go 1.23+
- 本地 BPE 词表文件 `cl100k_base.tiktoken`（工具不会联网下载，需提前准备）

---

## 编译

项目提供 Makefile，构建产物统一输出到 `build/` 目录。

```bash
make          # 构建全部平台（macOS / Linux / Windows，各含 amd64 + arm64）
make macos    # 仅构建 macOS
make linux    # 仅构建 Linux
make windows  # 仅构建 Windows
make clean    # 删除 build/ 目录
```

每个平台独立子目录，包含可执行文件和所需的 `cl100k_base.tiktoken` 文件，解压即可运行：

```
build/
├── embedding_benchmark-darwin-arm64/
│   ├── embedding_benchmark
│   └── cl100k_base.tiktoken
├── embedding_benchmark-linux-amd64/
│   ├── embedding_benchmark
│   └── cl100k_base.tiktoken
└── ...
```

版本号自动从 git tag 注入。

> **Windows 注意事项**：TUI 界面依赖 VT/ANSI 转义序列。Windows 10 1903+ 及 Windows 11 的 Windows Terminal、PowerShell 7+、WSL 均支持；旧版 cmd.exe 显示可能异常，建议使用 Windows Terminal。

---

## 运行

```bash
# 使用默认 BPE 文件路径（与可执行文件同目录的 cl100k_base.tiktoken）
./embedding_benchmark

# 指定 BPE 文件路径
./embedding_benchmark --bpe-file /path/to/cl100k_base.tiktoken
```

> 若 BPE 文件不存在，启动时会输出明确的错误提示和使用说明后退出。通过 Makefile 构建的发布包已将该文件包含在同一目录中，无需额外配置。

启动后进入 TUI 界面，按提示依次选择：

1. **API 类型**：Embedding / Chat Completion / Anthropic Messages
2. **测试模式**：Single Provider / Single Response View / PK Mode / Response Compare
3. **参数配置**：填写 URL、API Key、模型名等（占位符根据 API 类型自动切换）
4. **`ctrl+s`** 启动，实时显示进度（压测模式）或直接进入响应查看界面
5. 压测完成后查看结果；**`r`** 重新配置，**`esc`** 返回上级菜单，**`ctrl+c`** 退出

### 通用快捷键

| 快捷键 | 说明 |
|--------|------|
| `↑` / `↓` | 在列表中导航 |
| `enter` | 确认选择 |
| `tab` / `shift+tab` | 在配置项之间切换 |
| `ctrl+s` | 开始压测 / 发送请求 |
| `esc` | 返回上一步 / 取消压测 |
| `ctrl+c` | 退出 |

### 压测结果页快捷键

| 快捷键 | 说明 |
|--------|------|
| `r` | 重新配置并再次压测 |
| `e` | 查看错误日志（有错误时可用） |
| `ctrl+e` | 导出结果到文本文件 |
| `esc` | 返回上级菜单 |
| `ctrl+c` | 退出 |

### Response Compare / Single Response View 快捷键

| 快捷键 | 说明 |
|--------|------|
| `j` / `k` | 逐行向下 / 向上滚动 |
| `ctrl+d` / `ctrl+u` | 向下 / 向上翻半页 |
| `tab` | 切换左/右面板焦点（仅 Response Compare） |
| `ctrl+e` | 导出响应内容到文本文件 |
| `esc` | 返回配置页 |

---

## 指标说明

### 输入 Prompt 生成

Embedding、Chat Completion、Anthropic Messages 压测会根据用户填写的 `Input Tokens` 自动生成输入文本。工具内置 100 个不同长度的英文句子，运行时使用 `tiktoken（cl100k_base）` 计算每句 token 数，并确定性地选择句子拼接成自然 prompt。

最终输入会严格校准到用户指定的 token 数；如果句子拼接略微超过目标，会按 token 截断到目标长度。Single Response View、Response Compare、Prompt Cache Hit Test 使用用户直接输入的 prompt，不走自动生成逻辑。

### Embedding 模式

| 指标 | 说明 |
|---|---|
| **RPS** | 每秒成功完成的请求数 |
| **Input TPS** | 每秒处理的输入 Token 数 |
| **Input TPM** | 每分钟处理的输入 Token 数 |
| **Latency Avg/P50/P90/P99** | E2E 延迟分布（ms） |

### Chat Completion / Anthropic Messages 模式

延迟与吞吐指标**完全基于客户端本地时间测量**。用于 TPOT、Output TPS / TPM、Avg Output Tokens 的输出 token 数仍通过本地 `tiktoken（cl100k_base）` 统计，保证在 Provider 不返回 `usage` 时也能计算性能指标。

| 指标 | 说明 |
|---|---|
| **TTFT** | Time To First Token，从发送请求到收到第一个输出 token 的延迟 |
| **TPOT** | Time Per Output Token，decode 阶段每生成一个 token 的平均耗时 |
| **E2E** | End-to-End Latency，从发送请求到收到最后一个 token 的总延迟 |
| **Output TPS / TPM** | 输出 token 吞吐量（每秒 / 每分钟） |
| **Avg Output Tokens** | 每次请求平均输出的 token 数 |
| **API Prompt Tokens** | API 响应 `usage` 中输入 token 的累计值，仅统计成功且返回 usage 的请求 |
| **API Completion Tokens** | API 响应 `usage` 中输出 token 的累计值，仅统计成功且返回 usage 的请求 |
| **API Total Tokens** | API 响应 `usage` 中总 token 的累计值；Anthropic Messages 按 input + output 汇总 |
| **API Usage Samples** | 成功请求中实际返回 `usage` 的次数，例如 `3 / 10` 表示 10 次成功请求里 3 次返回 usage |

> OpenAI 兼容 Chat Completions 流式请求默认附带 `stream_options: {"include_usage": true}` 以请求 usage chunk；如 Provider 不支持或不返回 `usage`，结果页会显示 `N/A` / missing 计数，不影响性能指标统计。Custom Params 仍可覆盖该字段。

### Prompt Cache Hit Test

该模式使用非流式 Chat Completions 请求，每隔 3 秒重复发送用户粘贴的同一段 Prompt，并读取响应体中的 `usage.prompt_tokens_details.cached_tokens`。`Max Output Tokens` 为可选参数，留空时请求体不发送 `max_tokens`。

| 指标 | 说明 |
|---|---|
| **Cached Tokens** | 响应 `usage.prompt_tokens_details.cached_tokens` 的累计值 |
| **Overall Hit Rate** | 累计 cached tokens / 累计 prompt tokens |
| **Avg Request Hit Rate** | 每次请求命中率的平均值 |

若 Provider 不返回 `usage` 或不返回 `cached_tokens` 字段，结果中会显示 `N/A` / missing 计数，不会按 0 命中处理。可通过 Custom Params 传入 `prompt_cache_key`、`prompt_cache_retention` 等兼容参数。

---

## 错误处理与分类

压测过程中失败请求不会计入延迟、RPS、TPS/TPM 等成功指标，但会单独计入失败数。结果页会展示错误分类摘要，按 **`e`** 可打开错误日志查看分类汇总和原始错误详情。

当前错误分类包括：

| 分类 | 说明 |
|---|---|
| `rate_limit` | HTTP 429 限流 |
| `auth` | HTTP 401 / 403 认证或权限问题 |
| `bad_request` | HTTP 400 请求参数问题 |
| `server_error` | HTTP 5xx 服务端错误 |
| `timeout` | 请求超时或 context deadline |
| `connection` | DNS、连接失败、连接重置等网络连接问题 |
| `empty_output` | 流式响应成功但没有收到非空 content |
| `stream_error` | SSE 流读取失败 |
| `parse_error` | 响应 JSON 解析失败 |
| `client_error` | 本地请求构造、序列化等客户端错误 |
| `other` | 未匹配到以上类型的错误 |

---

## 注意事项

- BPE 词表文件 `cl100k_base.tiktoken` 须在本地可访问，工具不会联网下载；文件缺失时启动报错并退出
- Completion / Anthropic Messages 压测使用流式输出（`"stream": true`），目标 API 须支持 SSE
- Anthropic Messages 模式 SSE 以 `event: message_stop` 结束，`max_tokens` 为必填字段（默认 4096）
- Response Compare / Single Response View 使用非流式请求（`"stream": false`），响应头与 JSON body 均展示
- 所有压测统计仅包含**成功请求**，失败请求不计入延迟分布
- 非 200 响应的错误信息会包含服务端响应体（最多 512 字节），并按状态码归类，便于排查认证、限流问题
