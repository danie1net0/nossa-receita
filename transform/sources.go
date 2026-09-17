package transform

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

type sourceKind uint8

const (
	CompanySrc sourceKind = iota
	TaxSrc
	IBGESrc
)

type source struct {
	prefix       string
	filePrefix   string // empty means loaded from URL
	key          string
	sep          rune
	hasHeader    bool
	isCumulative bool
	kind         sourceKind
	counter      atomic.Uint32

	// values, when not nil, keeps an in-memory copy of the rows by id. Only the
	// small reference files checked for every establishment use it, so those
	// checks don't hit the key-value store millions of times.
	mu     sync.RWMutex
	values map[string][]string
}

func (s *source) remember(id string, row []string) {
	if s.values == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[id] = row
}

// value returns column idx of the row with the given id, and whether that row
// exists with enough columns.
func (s *source) value(id string, idx int) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.values[id]
	if !ok || len(row) <= idx {
		return "", false
	}
	return row[idx], true
}

func (s *source) keyFor(id string) []byte {
	if !s.isCumulative {
		return fmt.Appendf([]byte{}, "%s::%s", id, s.key)
	}
	c := s.counter.Add(1)
	return fmt.Appendf([]byte{}, "%s::%s::%d", id, s.key, c)
}

func newCompanySrc(prefix string, sep rune, hasHeader, isCumulative bool) *source {
	key := strings.ToLower(strings.TrimPrefix(prefix, "Lucro ")[0:3])
	return &source{
		prefix:       prefix,
		key:          key,
		sep:          sep,
		hasHeader:    hasHeader,
		isCumulative: isCumulative,
		kind:         CompanySrc,
	}
}

func newTaxSrc(prefix, filePrefix string, sep rune, hasHeader, isCumulative bool) *source {
	key := strings.ToLower(strings.TrimPrefix(prefix, "Lucro ")[0:3])
	return &source{
		prefix:       prefix,
		filePrefix:   filePrefix,
		key:          key,
		sep:          sep,
		hasHeader:    hasHeader,
		isCumulative: isCumulative,
		kind:         TaxSrc,
	}
}

func newIBGESrc(prefix string, sep rune, hasHeader, isCumulative bool) *source {
	key := strings.ToLower(strings.TrimPrefix(prefix, "Lucro ")[0:3])
	return &source{
		prefix:       prefix,
		key:          key,
		sep:          sep,
		hasHeader:    hasHeader,
		isCumulative: isCumulative,
		kind:         IBGESrc,
	}
}
