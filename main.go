package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkoukk/tiktoken-go"
)

// ─── 测试模式 ─────────────────────────────────────────────────────────────────

const (
	ModeEmbedding  = "embedding"
	ModeCompletion = "completion"
)

func detectTestMode(apiURL string) string {
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return ModeEmbedding
	}
	p := strings.ToLower(parsed.Path)
	if strings.Contains(p, "chat/completions") || strings.HasSuffix(p, "/completions") {
		return ModeCompletion
	}
	return ModeEmbedding
}

// ─── Embedding 请求/结果 ──────────────────────────────────────────────────────

type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type EmbeddingResult struct {
	Duration time.Duration
	Tokens   int
	Err      error
}

// ─── Completion 请求/结果 ─────────────────────────────────────────────────────

type CompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionRequest struct {
	Model     string              `json:"model"`
	Messages  []CompletionMessage `json:"messages"`
	Stream    bool                `json:"stream"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
}

type StreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type CompletionResult struct {
	TTFT          time.Duration // Time To First Token
	E2E           time.Duration // 端到端延迟（首字节到最后一字节）
	InputTokens   int
	OutputTokens  int // 本地计数
	SkippedChunks int // SSE 解析失败被跳过的 chunk 数
	Err           error
}

type offlineOnlyBpeLoader struct {
	fallback tiktoken.BpeLoader
	files    map[string]string
}

func (l *offlineOnlyBpeLoader) LoadTiktokenBpe(tiktokenBpeFile string) (map[string]int, error) {
	name := tiktokenBpeFile
	if parsed, err := url.Parse(tiktokenBpeFile); err == nil && parsed.Path != "" {
		name = filepath.Base(parsed.Path)
	} else {
		name = filepath.Base(tiktokenBpeFile)
	}

	localPath, ok := l.files[name]
	if !ok {
		return nil, fmt.Errorf("offline bpe file not configured for %q", name)
	}
	return l.fallback.LoadTiktokenBpe(localPath)
}

func main() {
	// ── 命令行参数 ──────────────────────────────────────────────────────────
	apiURL := flag.String("url", "https://api.openai.com/v1/embeddings", "API 终端地址（根据路径自动判断 embedding/completion 模式）")
	apiKey := flag.String("key", "", "API Key")
	modelName := flag.String("model", "text-embedding-3-small", "模型名称")
	bpeFile := flag.String("bpe-file", "./cl100k_base.tiktoken", "cl100k_base.tiktoken 本地文件路径（离线必需）")
	concurrency := flag.Int("c", 10, "并发数量")
	totalRequests := flag.Int("n", 100, "总请求数")
	targetTokens := flag.Int("tokens", 500, "每个请求的输入 Token 数量")
	maxOutputTokens := flag.Int("max-tokens", 0, "[completion 模式] 最大输出 Token 数（0 表示不限制）")
	systemPrompt := flag.String("system-prompt", "", "[completion 模式] 可选 system 消息内容")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "用法: %s [参数]\n\n参数说明：\n", os.Args[0])
		flag.PrintDefaults()
	}

	if len(os.Args) == 1 {
		fmt.Println("未检测到参数，以下是可用参数说明：")
		flag.Usage()
		return
	}

	flag.Parse()

	if *concurrency <= 0 {
		fmt.Println("错误: 并发数量 (--c) 必须大于 0")
		return
	}
	if *totalRequests <= 0 {
		fmt.Println("错误: 总请求数 (--n) 必须大于 0")
		return
	}
	if *targetTokens <= 0 {
		fmt.Println("错误: 输入 Token 数量 (--tokens) 必须大于 0")
		return
	}

	normalizedURL, err := normalizeAPIURL(*apiURL)
	if err != nil {
		fmt.Printf("无效的 API URL: %v\n", err)
		return
	}
	*apiURL = normalizedURL

	mode := detectTestMode(*apiURL)

	absBpePath, err := filepath.Abs(*bpeFile)
	if err != nil {
		fmt.Printf("解析 bpe 文件路径失败: %v\n", err)
		return
	}
	if _, err := os.Stat(absBpePath); err != nil {
		fmt.Printf("离线词表文件不可用: %s, 错误: %v\n", absBpePath, err)
		return
	}

	tiktoken.SetBpeLoader(&offlineOnlyBpeLoader{
		fallback: tiktoken.NewDefaultBpeLoader(),
		files: map[string]string{
			"cl100k_base.tiktoken": absBpePath,
		},
	})

	// ── 初始化 Tiktoken 并生成精确长度的测试文本 ──────────────────────────
	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		fmt.Printf("初始化分词器失败: %v\n", err)
		return
	}

	testText, err := generateTextByTokens(tkm, *targetTokens)
	if err != nil {
		fmt.Printf("生成测试文本失败: %v\n", err)
		return
	}
	actualInputTokens := len(tkm.Encode(testText, nil, nil))

	modeLabel := map[string]string{
		ModeEmbedding:  "Embedding",
		ModeCompletion: "Chat Completion (stream)",
	}[mode]

	fmt.Printf("🚀 测试启动: %s  [模式: %s]\n", *modelName, modeLabel)
	fmt.Printf("📊 参数: 并发=%d, 总请求=%d, 输入Token=%d (实际=%d)",
		*concurrency, *totalRequests, *targetTokens, actualInputTokens)
	if mode == ModeCompletion && *maxOutputTokens > 0 {
		fmt.Printf(", 最大输出Token=%d", *maxOutputTokens)
	}
	fmt.Println()
	fmt.Println(strings.Repeat("-", 60))

	// ── 任务队列 ────────────────────────────────────────────────────────────
	taskQueue := make(chan struct{}, *totalRequests)
	for i := 0; i < *totalRequests; i++ {
		taskQueue <- struct{}{}
	}
	close(taskQueue)

	var wg sync.WaitGroup
	startTime := time.Now()

	switch mode {
	case ModeEmbedding:
		runEmbeddingBench(apiURL, apiKey, modelName, actualInputTokens, testText,
			concurrency, totalRequests, taskQueue, &wg, startTime)

	case ModeCompletion:
		runCompletionBench(apiURL, apiKey, modelName, systemPrompt,
			actualInputTokens, testText, maxOutputTokens,
			concurrency, totalRequests, taskQueue, &wg, startTime, tkm)
	}
}

// ─── Embedding 并发测试 ───────────────────────────────────────────────────────

func runEmbeddingBench(
	apiURL, apiKey, modelName *string,
	actualInputTokens int, testText string,
	concurrency, totalRequests *int,
	taskQueue chan struct{}, wg *sync.WaitGroup,
	startTime time.Time,
) {
	results := make(chan EmbeddingResult, *totalRequests)

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 120 * time.Second}

			for range taskQueue {
				res := EmbeddingResult{Tokens: actualInputTokens}
				reqStart := time.Now()

				payload := EmbeddingRequest{
					Model: *modelName,
					Input: []string{testText},
				}
				body, _ := json.Marshal(payload)

				req, err := http.NewRequest("POST", *apiURL, bytes.NewBuffer(body))
				if err != nil {
					res.Err = fmt.Errorf("创建请求失败: %w", err)
					results <- res
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				if *apiKey != "" {
					req.Header.Set("Authorization", "Bearer "+*apiKey)
				}

				resp, err := client.Do(req)
				if err != nil {
					res.Err = err
					results <- res
					continue
				}
				if resp.Body == nil {
					res.Err = fmt.Errorf("响应为空")
					results <- res
					continue
				}
				if resp.StatusCode != http.StatusOK {
					errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
					resp.Body.Close()
					if len(errBody) > 0 {
						res.Err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
					} else {
						res.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
					}
				} else {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					res.Duration = time.Since(reqStart)
				}
				results <- res
			}
		}()
	}

	wg.Wait()
	close(results)
	totalWallTime := time.Since(startTime)

	var latencies []float64
	totalTokens := 0
	errors := 0
	errorDetails := make(map[string]int)

	for r := range results {
		if r.Err != nil {
			errors++
			errorDetails[r.Err.Error()]++
		} else {
			latencies = append(latencies, float64(r.Duration.Milliseconds()))
			totalTokens += r.Tokens
		}
	}
	sort.Float64s(latencies)
	printEmbeddingReport(latencies, totalTokens, totalWallTime, errors, errorDetails)
}

// ─── Completion 并发流式测试 ──────────────────────────────────────────────────

func runCompletionBench(
	apiURL, apiKey, modelName, systemPrompt *string,
	actualInputTokens int, testText string,
	maxOutputTokens *int,
	concurrency, totalRequests *int,
	taskQueue chan struct{}, wg *sync.WaitGroup,
	startTime time.Time,
	tkm *tiktoken.Tiktoken,
) {
	results := make(chan CompletionResult, *totalRequests)

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 流式请求通常耗时较长，给足超时
			client := &http.Client{Timeout: 300 * time.Second}

			for range taskQueue {
				res := doCompletionRequest(client, apiURL, apiKey, modelName, systemPrompt,
					actualInputTokens, testText, maxOutputTokens, tkm)
				results <- res
			}
		}()
	}

	wg.Wait()
	close(results)
	totalWallTime := time.Since(startTime)

	var (
		ttfts             []float64
		e2es              []float64
		tpots             []float64
		totalInToks       int
		totalOutToks      int
		totalSkippedChunks int
		errors            int
		errorDetails      = make(map[string]int)
	)

	for r := range results {
		if r.Err != nil {
			errors++
			errorDetails[r.Err.Error()]++
		} else {
			ttfts = append(ttfts, float64(r.TTFT.Milliseconds()))
			e2es = append(e2es, float64(r.E2E.Milliseconds()))
			totalInToks += r.InputTokens
			totalOutToks += r.OutputTokens
			totalSkippedChunks += r.SkippedChunks

			// TPOT = (E2E - TTFT) / output_tokens，若 output_tokens <= 1 则用 E2E
			if r.OutputTokens > 1 {
				decodeMs := float64((r.E2E - r.TTFT).Milliseconds())
				tpots = append(tpots, decodeMs/float64(r.OutputTokens-1))
			} else if r.OutputTokens == 1 {
				tpots = append(tpots, float64(r.E2E.Milliseconds()))
			}
		}
	}

	sort.Float64s(ttfts)
	sort.Float64s(e2es)
	sort.Float64s(tpots)

	printCompletionReport(ttfts, e2es, tpots, totalInToks, totalOutToks,
		totalWallTime, errors, errorDetails, totalSkippedChunks)
}

// doCompletionRequest 执行单次流式 chat completion 请求并返回所有测量指标。
func doCompletionRequest(
	client *http.Client,
	apiURL, apiKey, modelName, systemPrompt *string,
	actualInputTokens int, testText string,
	maxOutputTokens *int,
	tkm *tiktoken.Tiktoken,
) CompletionResult {
	res := CompletionResult{InputTokens: actualInputTokens}

	// 构造消息
	messages := []CompletionMessage{}
	if *systemPrompt != "" {
		messages = append(messages, CompletionMessage{Role: "system", Content: *systemPrompt})
	}
	messages = append(messages, CompletionMessage{Role: "user", Content: testText})

	payload := CompletionRequest{
		Model:    *modelName,
		Messages: messages,
		Stream:   true,
	}
	if *maxOutputTokens > 0 {
		payload.MaxTokens = *maxOutputTokens
	}

	body, err := json.Marshal(payload)
	if err != nil {
		res.Err = fmt.Errorf("序列化请求体失败: %w", err)
		return res
	}

	req, err := http.NewRequest("POST", *apiURL, bytes.NewBuffer(body))
	if err != nil {
		res.Err = fmt.Errorf("创建请求失败: %w", err)
		return res
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if *apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+*apiKey)
	}

	sendTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.Err = err
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(errBody) > 0 {
			res.Err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
		} else {
			res.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return res
	}

	// ── 解析 SSE 流 ──────────────────────────────────────────────────────────
	var (
		firstTokenTime time.Time
		lastTokenTime  time.Time
		outputBuf      strings.Builder
		gotFirstToken  bool
		skippedChunks  int
	)

	scanner := bufio.NewScanner(resp.Body)
	// 支持较长的输出行
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		if data == "" {
			continue
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			skippedChunks++
			continue
		}

		for _, choice := range chunk.Choices {
			content := choice.Delta.Content
			if content == "" {
				continue
			}
			now := time.Now()
			if !gotFirstToken {
				firstTokenTime = now
				gotFirstToken = true
			}
			lastTokenTime = now
			outputBuf.WriteString(content)
		}
	}

	if err := scanner.Err(); err != nil {
		res.Err = fmt.Errorf("读取流失败: %w", err)
		return res
	}

	if !gotFirstToken {
		// 模型没有返回任何 content（可能是错误格式或空响应）
		res.Err = fmt.Errorf("未收到任何输出 token")
		return res
	}

	// ── 指标计算（全部本地） ────────────────────────────────────────────────
	res.TTFT = firstTokenTime.Sub(sendTime)
	res.E2E = lastTokenTime.Sub(sendTime)

	// 本地计数输出 token
	outputText := outputBuf.String()
	res.OutputTokens = len(tkm.Encode(outputText, nil, nil))
	if res.OutputTokens == 0 {
		// 极端情况：收到了非空 delta.content 但 tiktoken 编码为 0 token（如纯控制字符），
		// 保底设为 1 以避免后续 TPOT 除零
		res.OutputTokens = 1
	}
	res.SkippedChunks = skippedChunks

	return res
}

func normalizeAPIURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("URL 不能为空")
	}

	if !(strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")) {
		return "", fmt.Errorf("URL 必须以 http:// 或 https:// 开头")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("URL 必须包含协议和主机，例如 http://127.0.0.1:3000/v1/embeddings")
	}

	return parsed.String(), nil
}

// 辅助函数：精确生成指定 Token 长度的文本
func generateTextByTokens(tkm *tiktoken.Tiktoken, count int) (string, error) {
	if count <= 0 {
		return "", nil
	}

	tokenID, ok := selectStableSingleTokenID(tkm)
	if ok {
		tokens := make([]int, count)
		for i := range tokens {
			tokens[i] = tokenID
		}
		text := tkm.Decode(tokens)
		if len(tkm.EncodeOrdinary(text)) == count {
			return text, nil
		}
	}

	// 回退方案：批量编码后按 token 截断，再解码，依然可保证精确 token 数
	seed := "physics "
	repeats := 1
	for {
		ids := tkm.EncodeOrdinary(strings.Repeat(seed, repeats))
		if len(ids) >= count {
			return tkm.Decode(ids[:count]), nil
		}
		repeats *= 2
		if repeats > count*64 {
			return "", fmt.Errorf("unable to generate %d tokens", count)
		}
	}
}

func selectStableSingleTokenID(tkm *tiktoken.Tiktoken) (int, bool) {
	candidates := []string{" a", " the", "hello", ".", " world"}

	for _, text := range candidates {
		ids := tkm.EncodeOrdinary(text)
		if len(ids) != 1 {
			continue
		}
		decoded := tkm.Decode([]int{ids[0]})
		roundtrip := tkm.EncodeOrdinary(decoded)
		if len(roundtrip) == 1 && roundtrip[0] == ids[0] {
			return ids[0], true
		}
	}

	for c := 32; c <= 126; c++ {
		text := string(rune(c))
		ids := tkm.EncodeOrdinary(text)
		if len(ids) != 1 {
			continue
		}
		decoded := tkm.Decode([]int{ids[0]})
		roundtrip := tkm.EncodeOrdinary(decoded)
		if len(roundtrip) == 1 && roundtrip[0] == ids[0] {
			return ids[0], true
		}
	}

	return 0, false
}

// ─── 辅助：错误详情打印 ───────────────────────────────────────────────────────

func printErrorDetails(errors int, errorDetails map[string]int) {
	if errors == 0 {
		return
	}
	fmt.Println("\n❗ 请求错误详情:")
	type errorStat struct {
		Message string
		Count   int
	}
	stats := make([]errorStat, 0, len(errorDetails))
	for msg, count := range errorDetails {
		stats = append(stats, errorStat{Message: msg, Count: count})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count == stats[j].Count {
			return stats[i].Message < stats[j].Message
		}
		return stats[i].Count > stats[j].Count
	})
	for _, item := range stats {
		fmt.Printf("  - %s (次数: %d)\n", item.Message, item.Count)
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// ─── Embedding 报告 ───────────────────────────────────────────────────────────

func printEmbeddingReport(latencies []float64, totalTokens int, wallTime time.Duration, errors int, errorDetails map[string]int) {
	successCount := len(latencies)
	totalCount := successCount + errors
	successRate, failureRate := 0.0, 0.0
	if totalCount > 0 {
		successRate = float64(successCount) * 100 / float64(totalCount)
		failureRate = float64(errors) * 100 / float64(totalCount)
	}

	printErrorDetails(errors, errorDetails)

	if successCount == 0 {
		fmt.Println("\n🏁 请求统计:")
		fmt.Printf("  总请求数:       %d\n", totalCount)
		fmt.Printf("  成功请求数:     %d (%.2f%%)\n", successCount, successRate)
		fmt.Printf("  失败请求数:     %d (%.2f%%)\n", errors, failureRate)
		fmt.Println("  ❌ 所有请求均失败，请检查 URL 或 API Key。")
		return
	}

	fmt.Println("\n🏁 Embedding 性能测试报告:")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("  总请求数:              %d\n", totalCount)
	fmt.Printf("  成功请求数:            %d (%.2f%%)\n", successCount, successRate)
	fmt.Printf("  失败请求数:            %d (%.2f%%)\n", errors, failureRate)
	fmt.Printf("  总运行耗时:            %.2f s\n", wallTime.Seconds())
	fmt.Printf("  RPS:                   %.2f req/s\n", float64(successCount)/wallTime.Seconds())
	fmt.Printf("  TPS (输入):            %.2f tokens/s\n", float64(totalTokens)/wallTime.Seconds())
	fmt.Printf("  TPM (输入):            %.2f tokens/min\n", float64(totalTokens)/wallTime.Minutes())
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("  端到端延迟 (ms):")
	fmt.Printf("    平均:  %.2f ms\n", average(latencies))
	fmt.Printf("    P50:   %.2f ms\n", percentile(latencies, 0.50))
	fmt.Printf("    P90:   %.2f ms\n", percentile(latencies, 0.90))
	fmt.Printf("    P99:   %.2f ms\n", percentile(latencies, 0.99))
	fmt.Println(strings.Repeat("-", 60))
}

// ─── Completion 报告 ──────────────────────────────────────────────────────────

func printCompletionReport(
	ttfts, e2es, tpots []float64,
	totalInToks, totalOutToks int,
	wallTime time.Duration,
	errors int, errorDetails map[string]int,
	skippedChunks int,
) {
	successCount := len(e2es)
	totalCount := successCount + errors
	successRate, failureRate := 0.0, 0.0
	if totalCount > 0 {
		successRate = float64(successCount) * 100 / float64(totalCount)
		failureRate = float64(errors) * 100 / float64(totalCount)
	}

	printErrorDetails(errors, errorDetails)

	if successCount == 0 {
		fmt.Println("\n🏁 请求统计:")
		fmt.Printf("  总请求数:       %d\n", totalCount)
		fmt.Printf("  成功请求数:     %d (%.2f%%)\n", successCount, successRate)
		fmt.Printf("  失败请求数:     %d (%.2f%%)\n", errors, failureRate)
		fmt.Println("  ❌ 所有请求均失败，请检查 URL 或 API Key。")
		return
	}

	outTPS := float64(totalOutToks) / wallTime.Seconds()
	inTPS := float64(totalInToks) / wallTime.Seconds()
	avgOutPerReq := float64(totalOutToks) / float64(successCount)

	fmt.Println("\n🏁 Chat Completion (stream) 性能测试报告:")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("  总请求数:              %d\n", totalCount)
	fmt.Printf("  成功请求数:            %d (%.2f%%)\n", successCount, successRate)
	fmt.Printf("  失败请求数:            %d (%.2f%%)\n", errors, failureRate)
	fmt.Printf("  总运行耗时:            %.2f s\n", wallTime.Seconds())
	fmt.Printf("  RPS:                   %.2f req/s\n", float64(successCount)/wallTime.Seconds())
	fmt.Printf("  输入 TPS:              %.2f tokens/s\n", inTPS)
	fmt.Printf("  输出 TPS:              %.2f tokens/s\n", outTPS)
	fmt.Printf("  输入 TPM:              %.2f tokens/min\n", float64(totalInToks)/wallTime.Minutes())
	fmt.Printf("  输出 TPM:              %.2f tokens/min\n", float64(totalOutToks)/wallTime.Minutes())
	fmt.Printf("  平均输出 Token 数:     %.1f tokens/req\n", avgOutPerReq)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Println("  TTFT - 首 Token 延迟 (ms):")
	fmt.Printf("    平均:  %.2f ms\n", average(ttfts))
	fmt.Printf("    P50:   %.2f ms\n", percentile(ttfts, 0.50))
	fmt.Printf("    P90:   %.2f ms\n", percentile(ttfts, 0.90))
	fmt.Printf("    P99:   %.2f ms\n", percentile(ttfts, 0.99))
	fmt.Println(strings.Repeat("-", 60))
	if len(tpots) > 0 {
		fmt.Println("  TPOT - 每 Token 生成时间 (ms/token):")
		fmt.Printf("    平均:  %.2f ms/token\n", average(tpots))
		fmt.Printf("    P50:   %.2f ms/token\n", percentile(tpots, 0.50))
		fmt.Printf("    P90:   %.2f ms/token\n", percentile(tpots, 0.90))
		fmt.Printf("    P99:   %.2f ms/token\n", percentile(tpots, 0.99))
		fmt.Println(strings.Repeat("-", 60))
	}
	fmt.Println("  E2E 延迟 - 端到端延迟 (ms):")
	fmt.Printf("    平均:  %.2f ms\n", average(e2es))
	fmt.Printf("    P50:   %.2f ms\n", percentile(e2es, 0.50))
	fmt.Printf("    P90:   %.2f ms\n", percentile(e2es, 0.90))
	fmt.Printf("    P99:   %.2f ms\n", percentile(e2es, 0.99))
	fmt.Println(strings.Repeat("-", 60))
	if skippedChunks > 0 {
		fmt.Printf("  ⚠️  SSE 解析: 共 %d 个 chunk 因 JSON 格式异常被跳过\n", skippedChunks)
	}
}
