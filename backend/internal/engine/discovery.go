package engine

import (
	"context"
	"log"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

// DiscoveryService periodically fetches instruments and slow data from all
// exchanges and populates the registry and slow cache layer.
type DiscoveryService struct {
	exchanges    []exchanges.Exchange
	registry     *market.Registry
	cache        *cache.Cache
	slowInterval time.Duration // how often to refresh volume/funding/OI
}

func NewDiscoveryService(
	registry *market.Registry,
	cache *cache.Cache,
	slowInterval time.Duration,
	exs ...exchanges.Exchange,
) *DiscoveryService {
	return &DiscoveryService{
		exchanges:    exs,
		registry:     registry,
		cache:        cache,
		slowInterval: slowInterval,
	}
}

// BootstrapAndRun runs the initial discovery (instruments + slow data) and
// then loops at slowInterval to refresh slow data.
// Blocks until ctx is cancelled.
func (d *DiscoveryService) BootstrapAndRun(ctx context.Context) error {
	// Initial instrument discovery (once on startup).
	if err := d.discoverInstruments(ctx); err != nil {
		return err
	}

	log.Printf("discovery: registry populated with %d instruments", d.registry.Size())

	// Initial slow data fetch.
	d.refreshSlow(ctx)

	// Periodic slow refresh.
	ticker := time.NewTicker(d.slowInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			d.refreshSlow(ctx)
		}
	}
}

func (d *DiscoveryService) discoverInstruments(ctx context.Context) error {
	for _, ex := range d.exchanges {
		instruments, err := ex.FetchInstruments(ctx)
		if err != nil {
			log.Printf("discovery: %s FetchInstruments error: %v", ex.Name(), err)
			continue // non-fatal: one exchange down should not block others
		}
		for _, inst := range instruments {
			d.registry.Add(inst)
		}
		log.Printf("discovery: %s → %d instruments", ex.Name(), len(instruments))
	}
	return nil
}

func (d *DiscoveryService) refreshSlow(ctx context.Context) {
	instruments := d.registry.All()
	for _, ex := range d.exchanges {
		mine := filterByExchange(instruments, ex.Name())
		if err := ex.FetchSlowData(ctx, mine, d.cache); err != nil {
			log.Printf("discovery: %s FetchSlowData error: %v", ex.Name(), err)
		}
	}
}

func filterByExchange(all []market.Instrument, name string) []market.Instrument {
	result := make([]market.Instrument, 0)
	for _, inst := range all {
		if inst.Exchange == name {
			result = append(result, inst)
		}
	}
	return result
}
