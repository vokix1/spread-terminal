package market

import "sync"

// Bus is a fan-out pub/sub channel for Opportunity updates.
// It replaces the old MarketEvent bus — the frontend now receives
// computed opportunities, not raw quotes.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[chan Opportunity]struct{}
}

func NewBus() *Bus {
	return &Bus{subscribers: make(map[chan Opportunity]struct{})}
}

// Subscribe returns a buffered channel that will receive Opportunity updates.
func (b *Bus) Subscribe(buffer int) chan Opportunity {
	if buffer < 1 {
		buffer = 512
	}
	ch := make(chan Opportunity, buffer)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Bus) Unsubscribe(ch chan Opportunity) {
	b.mu.Lock()
	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Publish sends an opportunity to all subscribers (non-blocking; slow clients are dropped).
func (b *Bus) Publish(opp Opportunity) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for sub := range b.subscribers {
		select {
		case sub <- opp:
		default:
		}
	}
}

// PublishAll fans out a batch of opportunities. Each is sent individually so
// the frontend can process them as a stream.
func (b *Bus) PublishAll(opps []Opportunity) {
	for _, opp := range opps {
		b.Publish(opp)
	}
}

func (b *Bus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
