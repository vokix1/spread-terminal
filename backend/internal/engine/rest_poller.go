package engine

import (
	"context"
	"log"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

// RestPoller periodically fetches bid/ask via REST for exchanges that either
// don't have a WS implementation yet, or as a warm-up before WS connects.
//
// It runs all exchanges on the same interval because most REST rate-limits
// for public ticker endpoints are generous (hundreds of requests/minute).
type RestPoller struct {
	exchanges []exchanges.Exchange
	cache     *cache.Cache
	interval  time.Duration
}

func NewRestPoller(cache *cache.Cache, interval time.Duration, exs ...exchanges.Exchange) *RestPoller {
	return &RestPoller{
		exchanges: exs,
		cache:     cache,
		interval:  interval,
	}
}

// Run fetches fast data immediately, then repeats at interval.
// It only fetches from exchanges whose WS is NOT running (tracked via wsActive).
// wsActive is a set of exchange names that have an active WS connection.
func (rp *RestPoller) Run(ctx context.Context, registry *market.Registry, wsActive func(name string) bool) {
	rp.poll(ctx, registry, wsActive)
	ticker := time.NewTicker(rp.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rp.poll(ctx, registry, wsActive)
		}
	}
}

func (rp *RestPoller) poll(ctx context.Context, registry *market.Registry, wsActive func(name string) bool) {
	all := registry.All()
	for _, ex := range rp.exchanges {
		// Skip if this exchange has an active WS — WS data is fresher.
		if wsActive(ex.Name()) {
			continue
		}
		mine := filterByExchange(all, ex.Name())
		if len(mine) == 0 {
			continue
		}
		if err := ex.FetchFastData(ctx, mine, rp.cache); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("rest_poller: %s FetchFastData error: %v", ex.Name(), err)
		} else {
			log.Printf("rest_poller: %s polled %d instruments", ex.Name(), len(mine))
		}
	}
}
