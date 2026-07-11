# Embedded BPE Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Package `cl100k_base.tiktoken` inside the executable by default while preserving `--bpe-file` as an external override.

**Architecture:** Embed the BPE data in `internal/bench`, materialize it to an automatically removed temporary file only during tokenizer initialization, and keep the existing offline-only `tiktoken-go` loader. An optional non-empty path continues to select a user-supplied BPE file. The Makefile stops copying a BPE sidecar.

**Tech Stack:** Go 1.24, `embed`, `os.CreateTemp`, `github.com/pkoukk/tiktoken-go`, GNU Make.

---

### Task 1: Test Embedded Default Initialization

**Files:**
- Modify: `internal/bench/tiktoken_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestInitTiktokenUsesEmbeddedBPEByDefault(t *testing.T) {
	tkm, err := InitTiktoken("")
	if err != nil {
		t.Fatalf("InitTiktoken: %v", err)
	}
	if got := len(tkm.EncodeOrdinary("embedded tokenizer works")); got == 0 {
		t.Fatal("embedded tokenizer produced no tokens")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/bench -run TestInitTiktokenUsesEmbeddedBPEByDefault -v`

Expected: FAIL because the current implementation attempts to resolve an empty BPE path.

### Task 2: Materialize Embedded BPE for the Existing Loader

**Files:**
- Modify: `internal/bench/tiktoken.go:3-57`
- Test: `internal/bench/tiktoken_test.go`

- [ ] **Step 1: Embed and write the data to a temporary file**

Add the BPE directive and create a helper with this behavior:

```go
//go:embed cl100k_base.tiktoken
var embeddedBPE []byte

func writeEmbeddedBPE() (string, func(), error) {
	f, err := os.CreateTemp("", "embedding-benchmark-cl100k-*.tiktoken")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary BPE file: %w", err)
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := f.Write(embeddedBPE); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, fmt.Errorf("failed to write embedded BPE file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to close embedded BPE file: %w", err)
	}
	return path, cleanup, nil
}
```

- [ ] **Step 2: Make the BPE path optional**

Change `InitTiktoken` so an empty path calls `writeEmbeddedBPE`, defers cleanup until after `tiktoken.GetEncoding("cl100k_base")`, and otherwise retains the existing absolute-path and existence validation.

- [ ] **Step 3: Run focused tests**

Run: `go test ./internal/bench -run 'TestInitTiktokenUsesEmbeddedBPEByDefault|TestGenerateMeaningfulTextByTokensExactCounts' -v`

Expected: PASS.

### Task 3: Make Embedded BPE the Application Default

**Files:**
- Modify: `main.go:3-30`

- [ ] **Step 1: Remove mandatory sidecar validation**

Change the flag default and startup call:

```go
bpeFile := flag.String("bpe-file", "", "Optional path to an external cl100k_base.tiktoken file")
// ...
tkm, err := bench.InitTiktoken(*bpeFile)
```

Remove the `os.Stat` startup block; `InitTiktoken` returns actionable errors for an invalid explicit override.

- [ ] **Step 2: Verify the application builds**

Run: `go build -o /tmp/embedding_benchmark .`

Expected: PASS.

### Task 4: Simplify Release Artifacts and Documentation

**Files:**
- Modify: `Makefile:1-13`
- Modify: `README.md:18-65,181-184`
- Modify: `AGENTS.md:12-15,72,94`

- [ ] **Step 1: Stop copying the BPE file during builds**

Remove `BPE := cl100k_base.tiktoken` and `cp $(BPE) $(DIR)/$(BPE)` from the Makefile.

- [ ] **Step 2: Update user-facing documentation**

State that the default executable embeds `cl100k_base.tiktoken`, works offline without a sidecar file, and accepts `--bpe-file /path/to/cl100k_base.tiktoken` only to override the embedded vocabulary. Update release-layout examples to contain only the executable.

- [ ] **Step 3: Run full verification**

Run: `go test ./... && go test -race ./... && make clean && make macos && test ! -e build/embedding_benchmark-darwin-arm64/cl100k_base.tiktoken && git diff --check`

Expected: all Go tests pass, both macOS executables build, BPE sidecars are absent, and diff check reports no output.
