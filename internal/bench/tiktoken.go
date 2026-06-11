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

type meaningfulSentence struct {
	text   string
	tokens int
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

// InitTiktoken sets up the offline BPE loader and returns a Tiktoken encoder.
func InitTiktoken(bpePath string) (*tiktoken.Tiktoken, error) {
	absBpePath, err := filepath.Abs(bpePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve bpe file path: %w", err)
	}
	if _, err := os.Stat(absBpePath); err != nil {
		return nil, fmt.Errorf("bpe file not available: %s: %w", absBpePath, err)
	}

	tiktoken.SetBpeLoader(&offlineOnlyBpeLoader{
		fallback: tiktoken.NewDefaultBpeLoader(),
		files: map[string]string{
			"cl100k_base.tiktoken": absBpePath,
		},
	})

	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tokenizer: %w", err)
	}
	return tkm, nil
}

// GenerateMeaningfulTextByTokens generates natural benchmark text with exactly count tokens.
func GenerateMeaningfulTextByTokens(tkm *tiktoken.Tiktoken, count int) (string, error) {
	if count <= 0 {
		return "", nil
	}
	sentences := parseMeaningfulSentences()
	if len(sentences) == 0 {
		return GenerateTextByTokens(tkm, count)
	}

	candidates := calculateMeaningfulSentenceTokens(tkm, sentences)
	text := buildMeaningfulText(tkm, candidates, count)
	ids := tkm.EncodeOrdinary(text)
	if len(ids) < count {
		for i := 0; len(ids) < count && i < len(candidates)*2; i++ {
			candidate := appendSentence(text, candidates[(count+i)%len(candidates)].text)
			ids = tkm.EncodeOrdinary(candidate)
			text = candidate
		}
	}
	if len(ids) >= count {
		text = tkm.Decode(ids[:count])
		if len(tkm.EncodeOrdinary(text)) == count {
			return text, nil
		}
	}

	return GenerateTextByTokens(tkm, count)
}

// GenerateTextByTokens generates text with exactly count tokens.
func GenerateTextByTokens(tkm *tiktoken.Tiktoken, count int) (string, error) {
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

func calculateMeaningfulSentenceTokens(tkm *tiktoken.Tiktoken, sentences []string) []meaningfulSentence {
	candidates := make([]meaningfulSentence, 0, len(sentences))
	for _, sentence := range sentences {
		candidates = append(candidates, meaningfulSentence{
			text:   sentence,
			tokens: len(tkm.EncodeOrdinary(sentence)),
		})
	}
	return candidates
}

func buildMeaningfulText(tkm *tiktoken.Tiktoken, sentences []meaningfulSentence, count int) string {
	start := count % len(sentences)
	text := ""
	used := make([]bool, len(sentences))

	for picked := 0; picked < len(sentences); picked++ {
		bestIdx := -1
		bestTokens := -1

		for offset := 0; offset < len(sentences); offset++ {
			idx := (start + picked + offset) % len(sentences)
			if used[idx] {
				continue
			}
			candidate := appendSentence(text, sentences[idx].text)
			tokenCount := len(tkm.EncodeOrdinary(candidate))
			if tokenCount <= count && tokenCount > bestTokens {
				bestIdx = idx
				bestTokens = tokenCount
			}
		}

		if bestIdx == -1 {
			break
		}
		text = appendSentence(text, sentences[bestIdx].text)
		used[bestIdx] = true
		if bestTokens == count {
			return text
		}
	}

	if text != "" {
		return text
	}
	return sentences[start].text
}

func appendSentence(text string, sentence string) string {
	if text == "" {
		return sentence
	}
	return text + "\n" + sentence
}
