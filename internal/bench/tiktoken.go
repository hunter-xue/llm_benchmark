package bench

import (
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

//go:embed meaningful_sentences.txt
var meaningfulSentencesText string

//go:embed cl100k_base.tiktoken
var embeddedBPE []byte

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

// InitTiktoken sets up the offline BPE loader and returns a Tiktoken encoder.
// An empty bpePath uses the BPE file embedded in the executable.
func InitTiktoken(bpePath string) (*tiktoken.Tiktoken, error) {
	cleanup := func() {}
	if bpePath == "" {
		var err error
		bpePath, cleanup, err = writeEmbeddedBPE()
		if err != nil {
			return nil, err
		}
		defer cleanup()
	} else {
		absBpePath, err := filepath.Abs(bpePath)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve bpe file path: %w", err)
		}
		if _, err := os.Stat(absBpePath); err != nil {
			return nil, fmt.Errorf("bpe file not available: %s: %w", absBpePath, err)
		}
		bpePath = absBpePath
	}

	tiktoken.SetBpeLoader(&offlineOnlyBpeLoader{
		fallback: tiktoken.NewDefaultBpeLoader(),
		files: map[string]string{
			"cl100k_base.tiktoken": bpePath,
		},
	})

	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tokenizer: %w", err)
	}
	return tkm, nil
}

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

// GenerateMeaningfulTextByTokens generates natural benchmark text with exactly count tokens.
func GenerateMeaningfulTextByTokens(tkm *tiktoken.Tiktoken, count int) (string, error) {
	return GenerateMeaningfulTextByTokensVariant(tkm, count, 0)
}

// GenerateMeaningfulTextByTokensVariant generates natural benchmark text with exactly
// count tokens. variant selects a deterministic cyclic shift of the sentence pool
// so different variants share length but differ in prefix/content.
func GenerateMeaningfulTextByTokensVariant(tkm *tiktoken.Tiktoken, count, variant int) (string, error) {
	if count <= 0 {
		return "", nil
	}
	if variant < 0 {
		variant = -variant
	}
	sentences := parseMeaningfulSentences()
	if len(sentences) == 0 {
		return GenerateTextByTokensVariant(tkm, count, variant)
	}

	return buildCyclicMeaningfulText(tkm, sentences, count, variant)
}

// GenerateTextByTokens generates text with exactly count tokens.
func GenerateTextByTokens(tkm *tiktoken.Tiktoken, count int) (string, error) {
	return GenerateTextByTokensVariant(tkm, count, 0)
}

// GenerateTextByTokensVariant generates filler text with exactly count tokens.
// variant perturbs the seed so unique-input mode does not collapse to identical text
// when the meaningful sentence pool is unavailable.
func GenerateTextByTokensVariant(tkm *tiktoken.Tiktoken, count, variant int) (string, error) {
	if count <= 0 {
		return "", nil
	}
	if variant < 0 {
		variant = -variant
	}

	if variant == 0 {
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
	}

	seed := "physics "
	if variant > 0 {
		seed = fmt.Sprintf("variant%d ", variant)
	}
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

func parseMeaningfulSentences() []string {
	lines := strings.Split(meaningfulSentencesText, "\n")
	sentences := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			sentences = append(sentences, line)
		}
	}
	return sentences
}

// buildCyclicMeaningfulText joins candidates in a deterministic order and repeats
// the corpus when a requested benchmark prompt exceeds its length.
// variant rotates the starting sentence so callers can request distinct prefixes.
func buildCyclicMeaningfulText(tkm *tiktoken.Tiktoken, sentences []string, count, variant int) (string, error) {
	text := ""
	ids := []int(nil)
	start := (count + variant) % len(sentences)
	for i := 0; len(ids) < count; i++ {
		text = appendSentence(text, sentences[(start+i)%len(sentences)])
		ids = tkm.EncodeOrdinary(text)
	}

	text = tkm.Decode(ids[:count])
	if len(tkm.EncodeOrdinary(text)) != count {
		return "", fmt.Errorf("unable to generate %d meaningful tokens", count)
	}
	return text, nil
}

func appendSentence(text string, sentence string) string {
	if text == "" {
		return sentence
	}
	return text + "\n" + sentence
}
