package bench

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

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
