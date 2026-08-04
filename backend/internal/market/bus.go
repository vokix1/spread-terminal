package market

import "sync"

type Bus struct {
	mu          sync.RWMutex
	subscribers map[chan MarketEvent]struct{}
}

func NewBus() *Bus {
	return &Bus{subscribers: make(map[chan MarketEvent]struct{})}
}

func (b *Bus) Subscribe(buffer int) chan MarketEvent {
	if buffer < 1 {
		buffer = 256
	}
	ch := make(chan MarketEvent, buffer)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Bus) Unsubscribe(ch chan MarketEvent) {
	b.mu.Lock()
	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *Bus) Publish(event MarketEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		default:
			// A slow browser client must not block the exchange feed.
		}
	}
}

func (b *Bus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}
