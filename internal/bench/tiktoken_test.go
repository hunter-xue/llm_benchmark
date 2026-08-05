package bench

import (
	"fmt"
	"strings"
	"testing"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

func testTokenizer(t *testing.T) *tiktoken.Tiktoken {
	t.Helper()
	tkm, err := InitTiktoken("cl100k_base.tiktoken")
	if err != nil {
		t.Fatalf("InitTiktoken: %v", err)
	}
	return tkm
}

func TestInitTiktokenUsesEmbeddedBPEByDefault(t *testing.T) {
	tkm, err := InitTiktoken("")
	if err != nil {
		t.Fatalf("InitTiktoken: %v", err)
	}
	if got := len(tkm.EncodeOrdinary("embedded tokenizer works")); got == 0 {
		t.Fatal("embedded tokenizer produced no tokens")
	}
}

func TestGenerateMeaningfulTextByTokensExactCounts(t *testing.T) {
	tkm := testTokenizer(t)
	for _, target := range []int{1, 10, 100, 500} {
		t.Run(fmt.Sprintf("target_%d", target), func(t *testing.T) {
			text, err := GenerateMeaningfulTextByTokens(tkm, target)
			if err != nil {
				t.Fatalf("GenerateMeaningfulTextByTokens(%d): %v", target, err)
			}
			if got := len(tkm.EncodeOrdinary(text)); got != target {
				t.Fatalf("token count = %d, want %d; text=%q", got, target, text)
			}
		})
	}
}

func TestGenerateMeaningfulTextByTokensDeterministicAndVaried(t *testing.T) {
	tkm := testTokenizer(t)

	first, err := GenerateMeaningfulTextByTokens(tkm, 100)
	if err != nil {
		t.Fatalf("GenerateMeaningfulTextByTokens first: %v", err)
	}
	second, err := GenerateMeaningfulTextByTokens(tkm, 100)
	if err != nil {
		t.Fatalf("GenerateMeaningfulTextByTokens second: %v", err)
	}
	if first != second {
		t.Fatalf("same target should produce deterministic text")
	}

	other, err := GenerateMeaningfulTextByTokens(tkm, 101)
	if err != nil {
		t.Fatalf("GenerateMeaningfulTextByTokens other: %v", err)
	}
	if first == other {
		t.Fatalf("different target should produce different text")
	}
	if strings.Count(first, " a") > 20 {
		t.Fatalf("meaningful text appears to have degenerated into repeated single-token filler: %q", first)
	}
	if lines := strings.Count(first, "\n") + 1; lines < 2 {
		t.Fatalf("meaningful text should combine multiple sentence candidates, got %d line(s): %q", lines, first)
	}
}

func TestBuildCyclicMeaningfulTextRepeatsCandidatesInOrder(t *testing.T) {
	tkm := testTokenizer(t)

	text, err := buildCyclicMeaningfulText(tkm, []string{"First sentence.", "Second sentence."}, 100, 0)
	if err != nil {
		t.Fatalf("buildCyclicMeaningfulText: %v", err)
	}
	if got := len(tkm.EncodeOrdinary(text)); got != 100 {
		t.Fatalf("token count = %d, want 100", got)
	}
	if !strings.HasPrefix(text, "First sentence.\nSecond sentence.\nFirst sentence.") {
		t.Fatalf("text does not repeat candidates in order: %q", text)
	}
}

func TestGenerateMeaningfulTextByTokensVariantDiffersAndExactCount(t *testing.T) {
	tkm := testTokenizer(t)
	const target = 100

	base, err := GenerateMeaningfulTextByTokens(tkm, target)
	if err != nil {
		t.Fatalf("GenerateMeaningfulTextByTokens: %v", err)
	}
	v0, err := GenerateMeaningfulTextByTokensVariant(tkm, target, 0)
	if err != nil {
		t.Fatalf("GenerateMeaningfulTextByTokensVariant(0): %v", err)
	}
	if base != v0 {
		t.Fatal("variant 0 should match GenerateMeaningfulTextByTokens")
	}

	v1, err := GenerateMeaningfulTextByTokensVariant(tkm, target, 1)
	if err != nil {
		t.Fatalf("GenerateMeaningfulTextByTokensVariant(1): %v", err)
	}
	if got := len(tkm.EncodeOrdinary(v1)); got != target {
		t.Fatalf("variant 1 token count = %d, want %d", got, target)
	}
	if v0 == v1 {
		t.Fatal("different variants should produce different text")
	}
	// Distinct prefixes matter for prefix-cache avoidance.
	if prefixLen := 40; len(v0) >= prefixLen && len(v1) >= prefixLen && v0[:prefixLen] == v1[:prefixLen] {
		t.Fatalf("expected different prefixes for variants; both start with %q", v0[:prefixLen])
	}
}

func TestGenerateTextByTokensVariantDiffers(t *testing.T) {
	tkm := testTokenizer(t)
	a, err := GenerateTextByTokensVariant(tkm, 200, 1)
	if err != nil {
		t.Fatalf("variant 1: %v", err)
	}
	b, err := GenerateTextByTokensVariant(tkm, 200, 2)
	if err != nil {
		t.Fatalf("variant 2: %v", err)
	}
	if len(tkm.EncodeOrdinary(a)) != 200 || len(tkm.EncodeOrdinary(b)) != 200 {
		t.Fatal("variant filler must keep exact token count")
	}
	if a == b {
		t.Fatal("different filler variants should differ")
	}
}

func TestGenerateMeaningfulTextByTokensLargeTargetAvoidsSingleTokenFiller(t *testing.T) {
	tkm := testTokenizer(t)

	text, err := GenerateMeaningfulTextByTokens(tkm, 10_000)
	if err != nil {
		t.Fatalf("GenerateMeaningfulTextByTokens: %v", err)
	}
	if got := len(tkm.EncodeOrdinary(text)); got != 10_000 {
		t.Fatalf("token count = %d, want 10000", got)
	}
	if strings.Count(text, " a") > 1_000 {
		t.Fatal("large prompt degenerated into repeated single-token filler")
	}
}

func TestGenerateTextByTokensLegacyFallbackExactCount(t *testing.T) {
	tkm := testTokenizer(t)
	text, err := GenerateTextByTokens(tkm, 500)
	if err != nil {
		t.Fatalf("GenerateTextByTokens: %v", err)
	}
	if got := len(tkm.EncodeOrdinary(text)); got != 500 {
		t.Fatalf("token count = %d, want 500", got)
	}
}
