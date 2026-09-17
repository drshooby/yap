package budget

import (
	"context"
	"sync"
)

type Budget struct {
	tokenCeiling int
	tokensSpent  int
	mu           sync.Mutex
	cancel       context.CancelFunc
}

func NewBudget(ceiling int, cancel context.CancelFunc) *Budget {
	return &Budget{
		tokenCeiling: ceiling,
		cancel:       cancel,
	}
}

func (b *Budget) Add(tokensIn, tokensOut int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.tokensSpent += (tokensIn + tokensOut)
	if b.tokensSpent >= b.tokenCeiling {
		b.cancel()
	}
}

func (b *Budget) Spent() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokensSpent
}
