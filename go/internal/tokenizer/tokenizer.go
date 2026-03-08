package tokenizer

import (
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

var (
	enc     tokenizer.Codec
	encErr  error
	encOnce sync.Once
)

func initEncoder() {
	var e tokenizer.Codec
	e, encErr = tokenizer.Get(tokenizer.Cl100kBase)
	enc = e
}

// CountTokens returns the number of tokens in the given text using cl100k_base encoding.
// Uses lazy init of the encoder (singleton).
func CountTokens(text string) (int, error) {
	encOnce.Do(initEncoder)
	if encErr != nil {
		return 0, encErr
	}
	ids, _, err := enc.Encode(text)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}
