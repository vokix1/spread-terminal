package engine

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

// AdaptiveStreamManager subscribes to WS for top-N instruments per exchange.
// For exchanges with no WS, RestPoller handles them instead.
type AdaptiveStreamManager struct {
	exchanges      []exchanges.Exchange
	cache          *cache.Cache
	topN           int
	reRankInterval time.Duration
	scanner        *Scanner

	mu       sync.RWMutex
	wsActive map[string]bool
}

func NewAdaptiveStreamManager(
	cache *cache.Cache,
	scanner *Scanner,
	topN int,
	reRankInterval time.Duration,
	exs ...exchanges.Exchange,
) *AdaptiveStreamManager {
	return &AdaptiveStreamManager{
		exchanges:      exs,
		cache:          cache,
		topN:           topN,
		reRankInterval: reRankInterval,
		scanner:        scanner,
		wsActive:       make(map[string]bool),
	}
}

func (sm *AdaptiveStreamManager) IsWSActive(name string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.wsActive[name]
}

func (sm *AdaptiveStreamManager) setWSActive(name string, active bool) {
	sm.mu.Lock()
	sm.wsActive[name] = active
	sm.mu.Unlock()
}

func (sm *AdaptiveStreamManager) Run(ctx context.Context) {
	// Wait for RestPoller to fill the cache first
	select {
	case <-ctx.Done():
		return
	case <-time.After(8 * time.Second):
	}

	cancelByExchange := make(map[string]context.CancelFunc)

	doRerank := func() {
		topByExchange := sm.topInstrumentsByExchange()
		for _, ex := range sm.exchanges {
			instruments, ok := topByExchange[ex.Name()]
			if !ok || len(instruments) == 0 {
				continue
			}
			if cancel, exists := cancelByExchange[ex.Name()]; exists {
				cancel()
				// Don't mark inactive here — the goroutine will do it on exit
			}
			exCtx, exCancel := context.WithCancel(ctx)
			cancelByExchange[ex.Name()] = exCancel
			go sm.runExchangeWS(exCtx, ex, instruments)
		}
	}

	doRerank()

	ticker := time.NewTicker(sm.reRankInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, cancel := range cancelByExchange {
				cancel()
			}
			return
		case <-ticker.C:
			doRerank()
		}
	}
}

func (sm *AdaptiveStreamManager) topInstrumentsByExchange() map[string][]market.Instrument {
	opps := sm.scanner.Scan()

	seen := make(map[market.InstrumentKey]bool)
	byExchange := make(map[string][]market.Instrument)

	for _, opp := range opps {
		pairs := []struct {
			exchange string
			symbol   string
			mtype    market.MarketType
		}{
			{opp.BuyExchange, opp.BuySymbol, opp.BuyType},
			{opp.SellExchange, opp.SellSymbol, opp.SellType},
		}
		for _, p := range pairs {
			key := market.InstrumentKey{Exchange: p.exchange, Symbol: p.symbol, MarketType: p.mtype}
			if seen[key] {
				continue
			}
			seen[key] = true
			byExchange[p.exchange] = append(byExchange[p.exchange], market.Instrument{
				Exchange:   p.exchange,
				Symbol:     p.symbol,
				MarketType: p.mtype,
			})
		}
		allFull := true
		for _, ex := range sm.exchanges {
			if len(byExchange[ex.Name()]) < sm.topN {
				allFull = false
				break
			}
		}
		if allFull {
			break
		}
	}

	for name, insts := range byExchange {
		log.Printf("stream_manager: %s → subscribing WS for top %d instruments", name, len(insts))
	}
	return byExchange
}

func (sm *AdaptiveStreamManager) runExchangeWS(
	ctx context.Context,
	ex exchanges.Exchange,
	instruments []market.Instrument,
) {
	// Mark as connecting — RestPoller will stop polling this exchange
	sm.setWSActive(ex.Name(), true)
	defer sm.setWSActive(ex.Name(), false)

	backoff := 2 * time.Second

	for {
		err := ex.StreamFast(ctx, instruments, sm.cache)

		var wsErr exchanges.ErrWSNotImplemented
		if errors.As(err, &wsErr) {
			log.Printf("stream_manager: %s has no WS — REST poller handles it", ex.Name())
			// Release so RestPoller takes over immediately
			sm.setWSActive(ex.Name(), false)
			return
		}

		if ctx.Err() != nil {
			return
		}

		log.Printf("stream_manager: %s stream ended (%v), retry in %s", ex.Name(), err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 60*time.Second {
			backoff *= 2
		}
	}
}
