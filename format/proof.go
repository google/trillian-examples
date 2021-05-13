package format

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Proof represents a common format proof.
//
// Interpretation of the proof bytes is ecosystem dependent.
type Proof [][]byte

// Unmarshal parses common proof format data and stores the result in the
// Proof struct.
func (p *Proof) Unmarshal(data []byte) error {
	const delim = "\n"
	lines := strings.Split(strings.TrimRight(string(data), delim), delim)
	r := make([][]byte, len(lines))
	for i, l := range lines {
		b, err := base64.StdEncoding.DecodeString(l)
		if err != nil {
			return fmt.Errorf("failed to decode proof line %d: %w", i, err)
		}
		r[i] = b
	}
	(*p) = r
	return nil
}
