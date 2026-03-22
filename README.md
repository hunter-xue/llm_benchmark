# embedding_benchmark

针对 OpenAI 兼容 API 的压测工具，支持两种模式：

- **Embedding 模式**：测试 `/v1/embeddings` 端点的吞吐与延迟
- **Chat Completion 模式**：测试 `/v1/chat/completions` 端点的流式生成性能，包含 TTFT、TPOT、E2E 等指标

工具根据 `--url` 参数中的路径**自动判断测试模式**，无需手动指定。

---

## 编译

```bash
go build -o embedding_benchmark .
```

### 交叉编译

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o embedding_benchmark_linux_amd64 .

# Linux arm64
GOOS=linux GOARCH=arm64 go build -o embedding_benchmark_linux_arm64 .

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o embedding_benchmark_darwin_arm64 .

# Windows amd64
GOOS=windows GOARCH=amd64 go build -o embedding_benchmark_windows_amd64.exe .
```

---

## 参数说明

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--url` | `https://api.openai.com/v1/embeddings` | API 地址。路径含 `chat/completions` 时自动切换为 Completion 模式 |
| `--key` | _(空)_ | API Key，填入后自动添加 `Authorization: Bearer <key>` 请求头 |
| `--model` | `text-embedding-3-small` | 模型名称 |
| `--bpe-file` | `./cl100k_base.tiktoken` | 本地 tiktoken BPE 词表文件路径（离线环境必须提供） |
| `--c` | `10` | 并发 worker 数量 |
| `--n` | `100` | 总请求数 |
| `--tokens` | `500` | 每个请求的输入 Token 数量 |
| `--max-tokens` | `0` | **[Completion 模式]** 最大输出 Token 数，0 表示不限制 |
| `--system-prompt` | _(空)_ | **[Completion 模式]** 可选的 system 消息内容 |

---

## 使用示例

### Embedding 测试

```bash
./embedding_benchmark \
  --url   https://api.openai.com/v1/embeddings \
  --key   sk-xxxxxxxx \
  --model text-embedding-3-small \
  --c     20 \
  --n     200 \
  --tokens 256
```

### Chat Completion 测试

```bash
./embedding_benchmark \
  --url        http://127.0.0.1:8080/v1/chat/completions \
  --key        sk-xxxxxxxx \
  --model      qwen2.5-72b-instruct \
  --c          10 \
  --n          50 \
  --tokens     512 \
  --max-tokens 256 \
  --system-prompt "You are a helpful assistant."
```

> 工具会根据 URL 路径自动选择模式，无需额外参数。

---

## 指标说明

### Embedding 模式

| 指标 | 说明 |
|---|---|
| **RPS** | 每秒成功完成的请求数 `= 成功请求数 / 总墙钟时间` |
| **TPS（输入）** | 每秒处理的输入 Token 数 `= 成功请求总 Token 数 / 总墙钟时间` |
| **TPM（输入）** | 每分钟处理的输入 Token 数 `= TPS × 60` |
| **E2E 延迟** | 单次请求从发出到完整响应体接收完毕的耗时，统计平均 / P50 / P90 / P99 |

### Chat Completion 模式

所有指标**完全基于客户端本地时间测量**，不依赖 API 响应体中的 `usage` 字段。

#### TTFT（Time To First Token）—— 首 Token 延迟

```
TTFT = 收到第一个非空 delta.content 的时刻 - 发送请求的时刻
```

衡量模型从接到请求到开始产出内容的响应速度，主要受 prefill（预填充）阶段耗时影响。

#### TPOT（Time Per Output Token）—— 每 Token 生成时间

```
TPOT = (收到最后一个 token 的时刻 - 收到第一个 token 的时刻) / (输出 token 数 - 1)
```

即 decode 阶段生成每个 token 平均耗费的时间，反映模型的 decode 吞吐能力。当输出仅有 1 个 token 时，退化为 E2E 延迟。

#### E2E（End-to-End Latency）—— 端到端延迟

```
E2E = 收到最后一个 token 的时刻 - 发送请求的时刻
```

用户视角下的完整等待时间，等于 `TTFT + TPOT × (输出 token 数 - 1)`。

#### 输出 Token 计数

输出文本通过 **tiktoken（cl100k_base）本地编码**计数，不使用 API 返回的 `usage.completion_tokens`，保证在任何不返回 `usage` 的 API 实现上也能正确统计。

#### 吞吐指标

| 指标 | 计算方式 |
|---|---|
| **RPS** | `成功请求数 / 总墙钟时间` |
| **输入 TPS** | `所有成功请求的输入 token 总数 / 总墙钟时间` |
| **输出 TPS** | `所有成功请求的输出 token 总数 / 总墙钟时间` |
| **输入 TPM** | `所有成功请求的输入 token 总数 / 总墙钟时间（分钟）` |
| **输出 TPM** | `所有成功请求的输出 token 总数 / 总墙钟时间（分钟）` |
| **平均输出 Token 数** | `输出 token 总数 / 成功请求数` |

> **总墙钟时间**为第一个 goroutine 启动到所有请求处理完毕的实际挂钟时间，反映整体并发吞吐能力。

---

## 输出示例

### Embedding 模式

```
🚀 测试启动: text-embedding-3-small  [模式: Embedding]
📊 参数: 并发=20, 总请求=200, 输入Token=256 (实际=256)
------------------------------------------------------------

🏁 Embedding 性能测试报告:
------------------------------------------------------------
  总请求数:              200
  成功请求数:            200 (100.00%)
  失败请求数:            0 (0.00%)
  总运行耗时:            8.43 s
  RPS:                   23.73 req/s
  TPS (输入):            6074.88 tokens/s
  TPM (输入):            364492.88 tokens/min
------------------------------------------------------------
  端到端延迟 (ms):
    平均:  832.15 ms
    P50:   801.42 ms
    P90:   1024.67 ms
    P99:   1312.08 ms
------------------------------------------------------------
```

### Chat Completion 模式

```
🚀 测试启动: qwen2.5-72b-instruct  [模式: Chat Completion (stream)]
📊 参数: 并发=10, 总请求=50, 输入Token=512 (实际=512), 最大输出Token=256
------------------------------------------------------------

🏁 Chat Completion (stream) 性能测试报告:
------------------------------------------------------------
  总请求数:              50
  成功请求数:            50 (100.00%)
  失败请求数:            0 (0.00%)
  总运行耗时:            42.17 s
  RPS:                   1.19 req/s
  输入 TPS:              607.62 tokens/s
  输出 TPS:              284.31 tokens/s
  输入 TPM:              36457.20 tokens/min
  输出 TPM:              17058.60 tokens/min
  平均输出 Token 数:     238.6 tokens/req
------------------------------------------------------------
  TTFT - 首 Token 延迟 (ms):
    平均:  312.48 ms
    P50:   298.11 ms
    P90:   401.23 ms
    P99:   512.67 ms
------------------------------------------------------------
  TPOT - 每 Token 生成时间 (ms/token):
    平均:  35.21 ms/token
    P50:   34.88 ms/token
    P90:   38.12 ms/token
    P99:   44.03 ms/token
------------------------------------------------------------
  E2E 延迟 - 端到端延迟 (ms):
    平均:  8724.33 ms
    P50:   8501.12 ms
    P90:   9812.44 ms
    P99:   11203.07 ms
------------------------------------------------------------
```

---

## 注意事项

- BPE 词表文件 `cl100k_base.tiktoken` 需在本地可访问，工具不会联网下载
- Completion 测试使用 `"stream": true`，若目标 API 不支持流式输出，请改用 Embedding 模式
- 所有统计仅包含**成功请求**，失败请求不计入延迟分布
- 并发数 `--c` 应根据服务端承载能力调整，过高并发可能导致大量超时错误
- `--c`、`--n`、`--tokens` 均须大于 0，否则工具会报错退出
- 非 200 响应的错误信息会包含服务端响应体内容（最多 512 字节），便于排查认证、限流等问题
