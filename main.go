package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkoukk/tiktoken-go"
	"github.com/spf13/cobra"

	"embedding_benchmark/internal/bench"
	"embedding_benchmark/internal/tui"
)

// ── Shared flags ──────────────────────────────────────────────────────

var flagBPEFile string

// ── Common benchmark flags (used by bench & pk) ───────────────────────

var (
	flagMode         string
	flagURL          string
	flagAPIKey       string
	flagModel        string
	flagName         string
	flagCustomParams string
	flagConcurrency  int
	flagRequests     int
	flagTokens       int
	flagMaxOutTokens int
	flagSystemPrompt string
	flagOutput       string
	flagOutputFile   string
)

// ── PK mode second-provider flags ──────────────────────────────────────

var (
	flagURLB          string
	flagAPIKeyB       string
	flagModelB        string
	flagNameB         string
	flagCustomParamsB string
)

// ── Compare mode flags ────────────────────────────────────────────────

var (
	flagCmpURLA          string
	flagCmpAPIKeyA       string
	flagCmpModelA        string
	flagCmpNameA         string
	flagCmpCustomParamsA string
	flagCmpURLB          string
	flagCmpAPIKeyB       string
	flagCmpModelB        string
	flagCmpNameB         string
	flagCmpCustomParamsB string
	flagCmpUserMessage   string
	flagCmpSystemPrompt  string
	flagCmpMaxOutTokens  int
	flagCmpOutput        string
	flagCmpOutputFile    string
	flagCmpMode          string
)

// ── Cache-hit flags ──────────────────────────────────────────────────

var (
	flagCacheURL          string
	flagCacheAPIKey       string
	flagCacheModel        string
	flagCacheName         string
	flagCacheCustomParams string
	flagCacheCount        int
	flagCacheInterval     time.Duration
	flagCacheUserPrompt   string
	flagCacheSystemPrompt string
	flagCacheMaxOutTokens int
	flagCacheOutput       string
	flagCacheOutputFile   string
)

// ── Root command ──────────────────────────────────────────────────────

var rootCmd = &cobra.Command{
	Use:   "embedding_benchmark",
	Short: "LLM API benchmark tool",
	Long:  "A benchmark tool for OpenAI-compatible embedding and chat completion APIs.\nLaunches interactive TUI when no subcommand is given.",
	RunE: func(cmd *cobra.Command, args []string) error {
		tkm, err := initTokenizer(flagBPEFile)
		if err != nil {
			return err
		}
		m := tui.NewModel(tkm)
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
		tui.SetProgram(p)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("TUI error: %v", err)
		}
		return nil
	},
}

// ── single subcommand ──────────────────────────────────────────────────

var singleCmd = &cobra.Command{
	Use:   "single",
	Short: "Benchmark a single provider",
	Long:  "Run a latency/throughput benchmark against a single API provider.\nOutputs results as JSON or XML.",
	Example: `  embedding_benchmark single --mode completion --url https://api.openai.com/v1/chat/completions \
      --api-key sk-xxx --model gpt-4o-mini --requests 20 --concurrency 5 --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateOutput(flagOutput); err != nil {
			return err
		}
		if flagMode == "" {
			return fmt.Errorf("--mode is required (embedding, completion, anthropic)")
		}
		if flagURL == "" {
			return fmt.Errorf("--url is required")
		}

		tkm, err := initTokenizer(flagBPEFile)
		if err != nil {
			return err
		}

		normalizedURL, err := bench.NormalizeURL(flagURL)
		if err != nil {
			return fmt.Errorf("invalid URL: %v", err)
		}

		provider := bench.ProviderConfig{
			Name:         flagName,
			URL:          normalizedURL,
			APIKey:       flagAPIKey,
			Model:        flagModel,
			CustomParams: flagCustomParams,
		}
		benchCfg := bench.BenchConfig{
			Mode:            flagMode,
			Concurrency:     flagConcurrency,
			TotalRequests:   flagRequests,
			TargetTokens:    flagTokens,
			MaxOutputTokens: flagMaxOutTokens,
			SystemPrompt:    flagSystemPrompt,
		}

		testText, actualTokens, err := prepareText(tkm, flagTokens)
		if err != nil {
			return err
		}

		ctx := context.Background()
		printBenchmarkHeader(flagMode, flagRequests, flagConcurrency, flagTokens)

		progress := newProgressPrinter(flagRequests, flagName)
		var result bench.BenchmarkResult

		switch flagMode {
		case bench.ModeEmbedding:
			report := bench.RunEmbeddingBench(ctx, provider, benchCfg, testText, actualTokens, progress.onProgress)
			progress.done()
			result = bench.SingleEmbeddingResult(flagMode, "single", report)

		case bench.ModeCompletion:
			report := bench.RunCompletionBench(ctx, provider, benchCfg, testText, actualTokens, tkm, progress.onProgress)
			progress.done()
			result = bench.SingleCompletionResult(flagMode, "single", report)

		case bench.ModeAnthropic:
			report := bench.RunAnthropicMessagesBench(ctx, provider, benchCfg, testText, actualTokens, tkm, progress.onProgress)
			progress.done()
			result = bench.SingleCompletionResult(flagMode, "single", report)

		default:
			return fmt.Errorf("unsupported mode: %s", flagMode)
		}

		return writeResult(result, flagOutput, flagOutputFile)
	},
}

// ── pk subcommand ──────────────────────────────────────────────────────

var pkCmd = &cobra.Command{
	Use:   "pk",
	Short: "PK mode: benchmark two providers side by side",
	Long:  "Run the same benchmark against two providers simultaneously and compare results.\nThe winner on each metric is highlighted in the output.",
	Example: `  embedding_benchmark pk --mode completion \
      --url https://provider-a/v1/chat/completions --api-key key-a --model gpt-4o --name "OpenAI" \
      --url-b https://provider-b/v1/chat/completions --api-key-b key-b --model-b deepseek-chat --name-b "DeepSeek" \
      --requests 20 --concurrency 5 --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateOutput(flagOutput); err != nil {
			return err
		}
		if flagMode == "" {
			return fmt.Errorf("--mode is required (embedding, completion, anthropic)")
		}
		if flagURL == "" {
			return fmt.Errorf("--url is required")
		}
		if flagURLB == "" {
			return fmt.Errorf("--url-b is required for PK mode")
		}

		tkm, err := initTokenizer(flagBPEFile)
		if err != nil {
			return err
		}

		normalizedURLA, err := bench.NormalizeURL(flagURL)
		if err != nil {
			return fmt.Errorf("invalid provider A URL: %v", err)
		}
		normalizedURLB, err := bench.NormalizeURL(flagURLB)
		if err != nil {
			return fmt.Errorf("invalid provider B URL: %v", err)
		}

		providerA := bench.ProviderConfig{
			Name:         flagName,
			URL:          normalizedURLA,
			APIKey:       flagAPIKey,
			Model:        flagModel,
			CustomParams: flagCustomParams,
		}
		providerB := bench.ProviderConfig{
			Name:         flagNameB,
			URL:          normalizedURLB,
			APIKey:       flagAPIKeyB,
			Model:        flagModelB,
			CustomParams: flagCustomParamsB,
		}
		benchCfg := bench.BenchConfig{
			Mode:            flagMode,
			Concurrency:     flagConcurrency,
			TotalRequests:   flagRequests,
			TargetTokens:    flagTokens,
			MaxOutputTokens: flagMaxOutTokens,
			SystemPrompt:    flagSystemPrompt,
		}

		testText, actualTokens, err := prepareText(tkm, flagTokens)
		if err != nil {
			return err
		}

		ctx := context.Background()
		printBenchmarkHeader(fmt.Sprintf("PK %s", flagMode), flagRequests, flagConcurrency, flagTokens)

		var result bench.BenchmarkResult

		switch flagMode {
		case bench.ModeEmbedding:
			pA := newProgressPrinter(flagRequests, flagName)
			pB := newProgressPrinter(flagRequests, flagNameB)
			reportA := bench.RunEmbeddingBench(ctx, providerA, benchCfg, testText, actualTokens, pA.onProgress)
			pA.done()
			reportB := bench.RunEmbeddingBench(ctx, providerB, benchCfg, testText, actualTokens, pB.onProgress)
			pB.done()
			result = bench.PKEmbeddingResult(flagMode, reportA, reportB, flagName, flagNameB)

		case bench.ModeCompletion, bench.ModeAnthropic:
			pA := newProgressPrinter(flagRequests, flagName)
			pB := newProgressPrinter(flagRequests, flagNameB)
			var reportA, reportB bench.CompletionReport
			if flagMode == bench.ModeAnthropic {
				reportA = bench.RunAnthropicMessagesBench(ctx, providerA, benchCfg, testText, actualTokens, tkm, pA.onProgress)
				pA.done()
				reportB = bench.RunAnthropicMessagesBench(ctx, providerB, benchCfg, testText, actualTokens, tkm, pB.onProgress)
				pB.done()
			} else {
				reportA = bench.RunCompletionBench(ctx, providerA, benchCfg, testText, actualTokens, tkm, pA.onProgress)
				pA.done()
				reportB = bench.RunCompletionBench(ctx, providerB, benchCfg, testText, actualTokens, tkm, pB.onProgress)
				pB.done()
			}
			result = bench.PKCompletionResult(flagMode, reportA, reportB, flagName, flagNameB)

		default:
			return fmt.Errorf("unsupported mode: %s", flagMode)
		}

		return writeResult(result, flagOutput, flagOutputFile)
	},
}

// ── compare subcommand ─────────────────────────────────────────────────

var compareCmd = &cobra.Command{
	Use:   "compare",
	Short: "Compare single responses from two providers",
	Long:  "Send the same prompt to two providers and compare their full responses\n(headers + body). Useful for checking format, latency differences, and content quality.",
	Example: `  embedding_benchmark compare --mode completion \
      --url https://provider-a/v1/chat/completions --api-key key-a --model gpt-4o --name "OpenAI" \
      --url-b https://provider-b/v1/chat/completions --api-key-b key-b --model-b claude-3 --name-b "Anthropic" \
      --user-message "Explain quantum computing in 3 sentences" --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateOutput(flagCmpOutput); err != nil {
			return err
		}
		if flagCmpMode == "" {
			return fmt.Errorf("--mode is required (completion, anthropic)")
		}
		if flagCmpURLA == "" {
			return fmt.Errorf("--url is required")
		}
		if flagCmpURLB == "" {
			return fmt.Errorf("--url-b is required for compare mode")
		}
		if flagCmpUserMessage == "" {
			return fmt.Errorf("--user-message is required")
		}

		tkm, err := initTokenizer(flagBPEFile)
		if err != nil {
			return err
		}
		_ = tkm

		normalizedURLA, err := bench.NormalizeURL(flagCmpURLA)
		if err != nil {
			return fmt.Errorf("invalid provider A URL: %v", err)
		}
		normalizedURLB, err := bench.NormalizeURL(flagCmpURLB)
		if err != nil {
			return fmt.Errorf("invalid provider B URL: %v", err)
		}

		providerA := bench.ProviderConfig{
			Name:         flagCmpNameA,
			URL:          normalizedURLA,
			APIKey:       flagCmpAPIKeyA,
			Model:        flagCmpModelA,
			CustomParams: flagCmpCustomParamsA,
		}
		providerB := bench.ProviderConfig{
			Name:         flagCmpNameB,
			URL:          normalizedURLB,
			APIKey:       flagCmpAPIKeyB,
			Model:        flagCmpModelB,
			CustomParams: flagCmpCustomParamsB,
		}

		benchCfg := bench.BenchConfig{
			Mode:            flagCmpMode,
			MaxOutputTokens: flagCmpMaxOutTokens,
			SystemPrompt:    flagCmpSystemPrompt,
		}

		ctx := context.Background()
		mustFprint(os.Stderr, "Comparing responses...\n")

		type cmpResult struct {
			headers string
			body    string
			err     error
		}
		resA := make(chan cmpResult, 1)
		resB := make(chan cmpResult, 1)

		go func() {
			var h, b string
			var e error
			if flagCmpMode == bench.ModeAnthropic {
				h, b, e = bench.DoAnthropicCompareRequest(ctx, providerA, benchCfg, flagCmpUserMessage, flagCmpCustomParamsA)
			} else {
				h, b, e = bench.DoCompareRequest(ctx, providerA, benchCfg, flagCmpUserMessage, flagCmpCustomParamsA)
			}
			resA <- cmpResult{h, b, e}
		}()
		go func() {
			var h, b string
			var e error
			if flagCmpMode == bench.ModeAnthropic {
				h, b, e = bench.DoAnthropicCompareRequest(ctx, providerB, benchCfg, flagCmpUserMessage, flagCmpCustomParamsB)
			} else {
				h, b, e = bench.DoCompareRequest(ctx, providerB, benchCfg, flagCmpUserMessage, flagCmpCustomParamsB)
			}
			resB <- cmpResult{h, b, e}
		}()

		rA := <-resA
		rB := <-resB

		pA := bench.CompareProviderResult{
			Name:    flagCmpNameA,
			Headers: rA.headers,
			Body:    rA.body,
		}
		pB := bench.CompareProviderResult{
			Name:    flagCmpNameB,
			Headers: rB.headers,
			Body:    rB.body,
		}
		if rA.err != nil {
			pA.Error = rA.err.Error()
		}
		if rB.err != nil {
			pB.Error = rB.err.Error()
		}

		result := bench.CompareResult(flagCmpMode, flagCmpUserMessage, pA, pB)
		return writeResult(result, flagCmpOutput, flagCmpOutputFile)
	},
}

// ── cache-hit subcommand ──────────────────────────────────────────────

var cacheHitCmd = &cobra.Command{
	Use:   "cache-hit",
	Short: "Test prompt cache hit rate",
	Long:  "Send repeated non-streaming requests and measure prompt cache hit rate\nfrom usage.prompt_tokens_details.cached_tokens.",
	Example: `  embedding_benchmark cache-hit \
      --url https://api.openai.com/v1/chat/completions --api-key sk-xxx --model gpt-4o-mini \
      --count 5 --interval 3s --tokens 1024 --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateOutput(flagCacheOutput); err != nil {
			return err
		}
		if flagCacheURL == "" {
			return fmt.Errorf("--url is required")
		}
		if flagCacheCount <= 0 {
			return fmt.Errorf("--count must be > 0")
		}

		tkm, err := initTokenizer(flagBPEFile)
		if err != nil {
			return err
		}

		normalizedURL, err := bench.NormalizeURL(flagCacheURL)
		if err != nil {
			return fmt.Errorf("invalid URL: %v", err)
		}

		provider := bench.ProviderConfig{
			Name:         flagCacheName,
			URL:          normalizedURL,
			APIKey:       flagCacheAPIKey,
			Model:        flagCacheModel,
			CustomParams: flagCacheCustomParams,
		}

		cacheCfg := bench.CacheHitConfig{
			TestCount:       flagCacheCount,
			MaxOutputTokens: flagCacheMaxOutTokens,
			SystemPrompt:    flagCacheSystemPrompt,
			Interval:        flagCacheInterval,
		}

		userPrompt := flagCacheUserPrompt
		if userPrompt == "" {
			userPrompt, _, err = prepareText(tkm, flagTokens)
			if err != nil {
				return err
			}
		}

		ctx := context.Background()
		mustFprint(os.Stderr, fmt.Sprintf("Running cache-hit test: count=%d interval=%s tokens=%d\n",
			flagCacheCount, flagCacheInterval, flagTokens))

		progress := newProgressPrinter(flagCacheCount, flagCacheName)
		report := bench.RunCacheHitTest(ctx, provider, cacheCfg, userPrompt, progress.onProgress)
		progress.done()

		result := bench.SingleCacheHitResult(report)
		return writeResult(result, flagCacheOutput, flagCacheOutputFile)
	},
}

// ── init: register flags ──────────────────────────────────────────────

func init() {
	rootCmd.PersistentFlags().StringVar(&flagBPEFile, "bpe-file", "./cl100k_base.tiktoken", "Path to local cl100k_base.tiktoken file")

	// bench subcommand flags
	singleCmd.Flags().StringVar(&flagMode, "mode", "", "Benchmark mode: embedding, completion, anthropic (required)")
	singleCmd.Flags().StringVar(&flagURL, "url", "", "API endpoint URL (required)")
	singleCmd.Flags().StringVar(&flagAPIKey, "api-key", "", "API key")
	singleCmd.Flags().StringVar(&flagModel, "model", "", "Model name (required)")
	singleCmd.Flags().StringVar(&flagName, "name", "Provider A", "Provider display name")
	singleCmd.Flags().StringVar(&flagCustomParams, "custom-params", "", "Custom JSON params to merge into request body")
	singleCmd.Flags().IntVar(&flagConcurrency, "concurrency", 1, "Concurrent requests")
	singleCmd.Flags().IntVar(&flagRequests, "requests", 10, "Total requests")
	singleCmd.Flags().IntVar(&flagTokens, "tokens", 128, "Target input tokens")
	singleCmd.Flags().IntVar(&flagMaxOutTokens, "max-output-tokens", 256, "Max output tokens for completion/anthropic (0=unlimited)")
	singleCmd.Flags().StringVar(&flagSystemPrompt, "system-prompt", "", "System prompt for completion/anthropic")
	singleCmd.Flags().StringVar(&flagOutput, "output", "json", "Output format: json, xml")
	singleCmd.Flags().StringVar(&flagOutputFile, "output-file", "", "Write output to file instead of stdout")

	// pk subcommand flags — reuse bench flags for provider A + add provider B
	pkCmd.Flags().StringVar(&flagMode, "mode", "", "Benchmark mode: embedding, completion, anthropic (required)")
	pkCmd.Flags().StringVar(&flagURL, "url", "", "Provider A API endpoint URL (required)")
	pkCmd.Flags().StringVar(&flagAPIKey, "api-key", "", "Provider A API key")
	pkCmd.Flags().StringVar(&flagModel, "model", "", "Provider A model name")
	pkCmd.Flags().StringVar(&flagName, "name", "Provider A", "Provider A display name")
	pkCmd.Flags().StringVar(&flagCustomParams, "custom-params", "", "Provider A custom JSON params")
	pkCmd.Flags().StringVar(&flagURLB, "url-b", "", "Provider B API endpoint URL (required)")
	pkCmd.Flags().StringVar(&flagAPIKeyB, "api-key-b", "", "Provider B API key")
	pkCmd.Flags().StringVar(&flagModelB, "model-b", "", "Provider B model name")
	pkCmd.Flags().StringVar(&flagNameB, "name-b", "Provider B", "Provider B display name")
	pkCmd.Flags().StringVar(&flagCustomParamsB, "custom-params-b", "", "Provider B custom JSON params")
	pkCmd.Flags().IntVar(&flagConcurrency, "concurrency", 1, "Concurrent requests")
	pkCmd.Flags().IntVar(&flagRequests, "requests", 10, "Total requests per provider")
	pkCmd.Flags().IntVar(&flagTokens, "tokens", 128, "Target input tokens")
	pkCmd.Flags().IntVar(&flagMaxOutTokens, "max-output-tokens", 256, "Max output tokens for completion/anthropic (0=unlimited)")
	pkCmd.Flags().StringVar(&flagSystemPrompt, "system-prompt", "", "System prompt for completion/anthropic")
	pkCmd.Flags().StringVar(&flagOutput, "output", "json", "Output format: json, xml")
	pkCmd.Flags().StringVar(&flagOutputFile, "output-file", "", "Write output to file instead of stdout")

	// compare subcommand flags
	compareCmd.Flags().StringVar(&flagCmpMode, "mode", "", "API mode: completion, anthropic (required)")
	compareCmd.Flags().StringVar(&flagCmpURLA, "url", "", "Provider A API endpoint URL (required)")
	compareCmd.Flags().StringVar(&flagCmpAPIKeyA, "api-key", "", "Provider A API key")
	compareCmd.Flags().StringVar(&flagCmpModelA, "model", "", "Provider A model name")
	compareCmd.Flags().StringVar(&flagCmpNameA, "name", "Provider A", "Provider A display name")
	compareCmd.Flags().StringVar(&flagCmpCustomParamsA, "custom-params", "", "Provider A custom JSON params")
	compareCmd.Flags().StringVar(&flagCmpURLB, "url-b", "", "Provider B API endpoint URL (required)")
	compareCmd.Flags().StringVar(&flagCmpAPIKeyB, "api-key-b", "", "Provider B API key")
	compareCmd.Flags().StringVar(&flagCmpModelB, "model-b", "", "Provider B model name")
	compareCmd.Flags().StringVar(&flagCmpNameB, "name-b", "Provider B", "Provider B display name")
	compareCmd.Flags().StringVar(&flagCmpCustomParamsB, "custom-params-b", "", "Provider B custom JSON params")
	compareCmd.Flags().StringVar(&flagCmpUserMessage, "user-message", "", "User message to send (required)")
	compareCmd.Flags().StringVar(&flagCmpSystemPrompt, "system-prompt", "", "System prompt")
	compareCmd.Flags().IntVar(&flagCmpMaxOutTokens, "max-output-tokens", 256, "Max output tokens (0=unlimited)")
	compareCmd.Flags().StringVar(&flagCmpOutput, "output", "json", "Output format: json, xml")
	compareCmd.Flags().StringVar(&flagCmpOutputFile, "output-file", "", "Write output to file instead of stdout")

	// cache-hit subcommand flags
	cacheHitCmd.Flags().StringVar(&flagCacheURL, "url", "", "API endpoint URL (required)")
	cacheHitCmd.Flags().StringVar(&flagCacheAPIKey, "api-key", "", "API key")
	cacheHitCmd.Flags().StringVar(&flagCacheModel, "model", "", "Model name")
	cacheHitCmd.Flags().StringVar(&flagCacheName, "name", "Provider A", "Provider display name")
	cacheHitCmd.Flags().StringVar(&flagCacheCustomParams, "custom-params", "", "Custom JSON params to merge into request body")
	cacheHitCmd.Flags().IntVar(&flagCacheCount, "count", 5, "Number of test requests")
	cacheHitCmd.Flags().DurationVar(&flagCacheInterval, "interval", 3*time.Second, "Interval between requests")
	cacheHitCmd.Flags().StringVar(&flagCacheUserPrompt, "user-prompt", "", "Custom user prompt (default: auto-generated from --tokens)")
	cacheHitCmd.Flags().IntVar(&flagTokens, "tokens", 128, "Target input tokens (when --user-prompt is empty)")
	cacheHitCmd.Flags().StringVar(&flagCacheSystemPrompt, "system-prompt", "", "System prompt")
	cacheHitCmd.Flags().IntVar(&flagCacheMaxOutTokens, "max-output-tokens", 256, "Max output tokens (0=unlimited)")
	cacheHitCmd.Flags().StringVar(&flagCacheOutput, "output", "json", "Output format: json, xml")
	cacheHitCmd.Flags().StringVar(&flagCacheOutputFile, "output-file", "", "Write output to file instead of stdout")

	rootCmd.AddCommand(singleCmd, pkCmd, compareCmd, cacheHitCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

// bpeSearchPaths lists locations to search for cl100k_base.tiktoken
// when the user does not explicitly specify --bpe-file.
var bpeSearchPaths = []string{
	"./cl100k_base.tiktoken",
	"/etc/cl100k_base.tiktoken",
	"/usr/local/etc/cl100k_base.tiktoken",
	"/usr/share/embedding_benchmark/cl100k_base.tiktoken",
}

func resolveBPEFile(flag string) string {
	if flag != "" && flag != "./cl100k_base.tiktoken" {
		return flag
	}
	for _, p := range bpeSearchPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return flag
}

func initTokenizer(bpeFile string) (*tiktoken.Tiktoken, error) {
	bpeFile = resolveBPEFile(bpeFile)
	if _, err := os.Stat(bpeFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("BPE file not found: %s\n\nSearched locations:\n  %s\n\nSet --bpe-file to the correct path.", bpeFile, strings.Join(bpeSearchPaths, "\n  "))
	}
	tkm, err := bench.InitTiktoken(bpeFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load tokenizer: %v", err)
	}
	return tkm, nil
}

func validateOutput(format string) error {
	if format != "json" && format != "xml" {
		return fmt.Errorf("--output must be 'json' or 'xml', got: %s", format)
	}
	return nil
}

func prepareText(tkm *tiktoken.Tiktoken, tokens int) (string, int, error) {
	text, err := bench.GenerateTextByTokens(tkm, tokens)
	if err != nil {
		return "", 0, fmt.Errorf("failed to generate test text: %v", err)
	}
	actual := len(tkm.EncodeOrdinary(text))
	return text, actual, nil
}

func printBenchmarkHeader(mode string, requests, concurrency, tokens int) {
	mustFprint(os.Stderr, fmt.Sprintf("Running benchmark: mode=%s requests=%d concurrency=%d tokens=%d\n",
		mode, requests, concurrency, tokens))
}

func writeResult(result bench.BenchmarkResult, format, outputFile string) error {
	output, err := bench.MarshalBenchmarkResult(result, format)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %v", err)
	}
	return writeOutput(output, outputFile)
}

func writeOutput(data []byte, outputFile string) error {
	if outputFile != "" {
		if err := os.WriteFile(outputFile, data, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %v", err)
		}
		mustFprint(os.Stderr, fmt.Sprintf("Output written to %s\n", outputFile))
	} else {
		fmt.Println(string(data))
	}
	return nil
}

// ── Progress printer ──────────────────────────────────────────────────

type progressPrinter struct {
	total     int
	label     string
	width     int
	completed atomic.Int64
	errors    atomic.Int64
	lastPct   atomic.Int64
}

func newProgressPrinter(total int, label string) *progressPrinter {
	return &progressPrinter{
		total: total,
		label: label,
		width: 30,
	}
}

func (p *progressPrinter) onProgress(completed, errors int) {
	p.completed.Store(int64(completed))
	p.errors.Store(int64(errors))
	pct := int64(float64(completed) / float64(p.total) * 100)
	if pct == p.lastPct.Load() && completed < p.total {
		return
	}
	p.lastPct.Store(pct)
	p.render(completed, errors)
}

func (p *progressPrinter) render(completed, errors int) {
	pct := float64(completed) / float64(p.total)
	filled := int(pct * float64(p.width))
	if filled > p.width {
		filled = p.width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.width-filled)
	errPart := ""
	if errors > 0 {
		errPart = fmt.Sprintf(" err:%d", errors)
	}
	mustFprint(os.Stderr, fmt.Sprintf("\r  %s [%s] %d/%d (%.0f%%)%s",
		p.label, bar, completed, p.total, pct*100, errPart))
}

func (p *progressPrinter) done() {
	completed := int(p.completed.Load())
	errors := int(p.errors.Load())
	p.render(completed, errors)
	mustFprint(os.Stderr, "\n")
}

func mustFprint(w *os.File, s string) {
	_, _ = fmt.Fprint(w, s)
}
