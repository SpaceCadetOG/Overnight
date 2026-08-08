package execution

import (
	"fmt"
	"hash/fnv"
)

const maxLighterClientOrderIndex uint64 = (1 << 48) - 1

// ClientOrderIndex deterministically maps the frozen daily intent ID to
// Lighter's 48-bit client index. A restart therefore rebuilds the same ID.
func ClientOrderIndex(intentID string) (int64, error) {
	if intentID == "" {
		return 0, fmt.Errorf("intent ID is required")
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(intentID))
	value := h.Sum64() & maxLighterClientOrderIndex
	if value == 0 {
		value = 1
	}
	return int64(value), nil
}
