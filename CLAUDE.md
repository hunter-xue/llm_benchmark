# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build
go build -o embedding_benchmark .

# Run (no args shows usage)
./embedding_benchmark

# Embedding benchmark
./embedding_benchmark --url https://api.openai.com/v1/embeddings --key sk-xxx --model text-embedding-3-small --c 20 --n 200 --tokens 256

# Chat completion benchmark (mode auto-detected from URL path)
./embedding_benchmark --url http://127.0.0.1:8080/v1/chat/completions --key sk-xxx --model qwen2.5-72b-instruct --c 10 --n 50 --tokens 512 --max-tokens 256
```

## Architecture

Single-file Go program (`main.go`) with no subpackages. The two benchmark modes share the same CLI and entry point but diverge at execution:

- **Mode detection** (`detectTestMode`): URL path containing `chat/completions` → completion mode; otherwise → embedding mode.
- **Embedding mode** (`runEmbeddingBench`): Fires `POST /v1/embeddings` requests concurrently, measures end-to-end latency per request.
- **Completion mode** (`runCompletionBench` + `doCompletionRequest`): Fires streaming `POST /v1/chat/completions` requests, measures TTFT (Time To First Token), TPOT (Time Per Output Token), E2E latency, and TPM by parsing SSE stream events client-side. Malformed SSE JSON chunks are counted in `SkippedChunks` and reported as a warning.
- **Token counting**: Uses `tiktoken-go` with `cl100k_base` encoding. The BPE file (`cl100k_base.tiktoken`) must be present locally — the `offlineOnlyBpeLoader` blocks any network download attempts.
- **Concurrency model**: A buffered `taskQueue` channel pre-filled with N tasks; `concurrency` goroutines each pull from it until exhausted, then results are collected from a results channel.

All token counts (input and output) are computed locally via tiktoken — never from `usage.completion_tokens` — so the tool works with any OpenAI-compatible API regardless of whether it returns usage data. TPM (tokens per minute) is derived from the same local counts divided by wall time.

## Key Flags

| Flag | Default | Notes |
|------|---------|-------|
| `--url` | `https://api.openai.com/v1/embeddings` | Path determines mode |
| `--bpe-file` | `./cl100k_base.tiktoken` | Must exist locally |
| `--c` | 10 | Concurrent workers |
| `--n` | 100 | Total requests |
| `--tokens` | 500 | Input tokens per request |
| `--max-tokens` | 0 | Completion mode only; 0 = unlimited |
| `--system-prompt` | _(empty)_ | Completion mode only |

## Validation & Error Behaviour

- `--c`, `--n`, `--tokens` must all be > 0; the tool exits with an error message if not.
- Non-200 HTTP responses include up to 512 bytes of the response body in the error string, making auth/rate-limit failures easier to diagnose.
- The `percentile` function uses the nearest-rank method: `idx = ceil(n × p) − 1`.
