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
- 无额外运行时文件：`cl100k_base.tiktoken` 已嵌入可执行文件，工具不会联网下载

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

每个平台独立子目录仅包含可执行文件，解压即可离线运行：

```
build/
├── embedding_benchmark-darwin-arm64/
│   └── embedding_benchmark
├── embedding_benchmark-linux-amd64/
│   └── embedding_benchmark
└── ...
```

版本号自动从 git tag 注入。

> **Windows 注意事项**：TUI 界面依赖 VT/ANSI 转义序列。Windows 10 1903+ 及 Windows 11 的 Windows Terminal、PowerShell 7+、WSL 均支持；旧版 cmd.exe 显示可能异常，建议使用 Windows Terminal。

---

## 运行

```bash
# 使用内嵌 BPE 词表
./embedding_benchmark

# 可选：指定外部 BPE 文件覆盖内嵌词表
./embedding_benchmark --bpe-file /path/to/cl100k_base.tiktoken
```

> 默认 BPE 词表已嵌入可执行文件。仅在指定 `--bpe-file` 时，外部文件必须存在。

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

## 并发模型（Load Model）

Chat Completion 模式支持两种并发模型，通过配置界面的 **Load Model** 单选切换。Embedding 和 Anthropic Messages 模式仅支持 Closed-loop。

### Closed-loop（闭环，默认）

固定数量的 worker 持续消费任务队列。每个 worker 发送请求 → 等待响应 → 立即发送下一个请求。

- **Concurrency**：同时工作的客户端 worker 数
- 请求发送速率受服务端延迟影响：服务端越快，客户端发送越快；服务端越慢，发送自然下降
- 实际吞吐 ≈ `Concurrency / 平均请求延迟`
- 适合测试**固定客户端并发下的吞吐与延迟**

```
示例：Concurrency = 10，平均延迟 = 2s
→ 大约维持 10 个请求在途，理论吞吐 ≈ 5 req/s
```

### Open-loop（开环，SGLang 风格）

请求生成器按 Poisson 分布以固定速率产生请求，semaphore 限制最大在途请求数。请求到达率与服务端响应速度解耦。

- **Request Rate**：请求到达速率（req/s），间隔服从指数分布
- **Max In-Flight**：最大同时在途请求数（相当于 closed-loop 的 Concurrency）
- 当请求到达率超过服务端处理能力时，超出部分在 semaphore 前排队等待，产生 **Queue Time**
- 适合模拟**真实流量、观察排队行为、测量过载表现**

```
示例：Request Rate = 20 req/s，Max In-Flight = 8
→ 每秒约产生 20 个请求，但最多 8 个同时执行
→ 若服务端只能处理 5 req/s，请求排队，Queue Time 增长
```

### 两种模型的核心区别

| 维度 | Closed-loop | Open-loop |
|------|-------------|-----------|
| 并发控制 | 固定 worker 数 | Semaphore 限制最大在途数 |
| 请求速率 | 受服务端延迟影响（自适应） | 独立可控（Request Rate） |
| 到达模式 | 上一个完成 → 立即发下一个 | Poisson 分布间隔 |
| 排队行为 | 无排队（worker 阻塞在请求上） | 有排队（semaphore 满时等待） |
| Queue Time | 无 | 有（Avg/P50/P90/P99） |
| 典型用途 | 固定并发吞吐测试 | 真实流量模拟、过载测试 |

> **注意**：Open-loop 的 generator 在 semaphore 满时会阻塞，因此当服务端处理不过来时，实际到达率会低于设定的 Request Rate。Queue Time 指标反映的是每个请求从"生成"到"实际发出"的等待时间。

---

## 指标说明

### 输入 Prompt 生成

Embedding、Chat Completion、Anthropic Messages 压测会根据用户填写的 `Input Tokens` 自动生成输入文本。工具内置大规模英文候选语料，运行时按确定性顺序拼接候选文本；目标超过语料长度时会循环使用语料，不会退化为重复单 token 填充。

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
| **Queue Time Avg/P50/P90/P99** | 请求从生成到实际发送的排队等待时间（ms），仅 Open-loop 模式显示 |

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

- 默认 BPE 词表已嵌入可执行文件，工具不会联网下载；可通过 `--bpe-file` 指定外部文件覆盖它
- Completion / Anthropic Messages 压测使用流式输出（`"stream": true`），目标 API 须支持 SSE
- Anthropic Messages 模式 SSE 以 `event: message_stop` 结束，`max_tokens` 为必填字段（默认 4096）
- Response Compare / Single Response View 使用非流式请求（`"stream": false`），响应头与 JSON body 均展示
- 所有压测统计仅包含**成功请求**，失败请求不计入延迟分布
- 非 200 响应的错误信息会包含服务端响应体（最多 512 字节），并按状态码归类，便于排查认证、限流问题
