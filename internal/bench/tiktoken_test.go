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

	text, err := buildCyclicMeaningfulText(tkm, []string{"First sentence.", "Second sentence."}, 100)
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
