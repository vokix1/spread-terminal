// Package cache provides a two-level market data cache:
//   - Slow layer: REST-sourced data (volume, funding, OI) refreshed every 30-60s.
//   - Fast layer: WebSocket-sourced bid/ask, updated many times per second.
package cache

import (
	"sync"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/market"
)

// SlowData holds REST-sourced data for one instrument.
type SlowData struct {
	Volume24h   float64
	OpenInt     float64
	FundingRate float64
	UpdatedAt   time.Time
}

// FastData holds WebSocket bid/ask for one instrument.
type FastData struct {
	Bid       float64
	Ask       float64
	BidQty    float64
	AskQty    float64
	UpdatedAt time.Time
}

type cacheKey = market.InstrumentKey

// Cache is the unified two-level market data store.
type Cache struct {
	slowMu sync.RWMutex
	slow   map[cacheKey]SlowData

	fastMu sync.RWMutex
	fast   map[cacheKey]FastData
}

func New() *Cache {
	return &Cache{
		slow: make(map[cacheKey]SlowData),
		fast: make(map[cacheKey]FastData),
	}
}

func (c *Cache) SetSlow(key cacheKey, d SlowData) {
	c.slowMu.Lock()
	c.slow[key] = d
	c.slowMu.Unlock()
}

func (c *Cache) GetSlow(key cacheKey) (SlowData, bool) {
	c.slowMu.RLock()
	d, ok := c.slow[key]
	c.slowMu.RUnlock()
	return d, ok
}

func (c *Cache) SetFast(key cacheKey, d FastData) {
	c.fastMu.Lock()
	c.fast[key] = d
	c.fastMu.Unlock()
}

func (c *Cache) GetFast(key cacheKey) (FastData, bool) {
	c.fastMu.RLock()
	d, ok := c.fast[key]
	c.fastMu.RUnlock()
	return d, ok
}

// Ticker merges slow + fast layers into a Ticker for the given instrument.
// Returns false if no fast data is available yet.
func (c *Cache) Ticker(inst market.Instrument) (market.Ticker, bool) {
	key := cacheKey{
		Exchange:   inst.Exchange,
		Symbol:     inst.Symbol,
		MarketType: inst.MarketType,
	}
	fast, ok := c.GetFast(key)
	if !ok {
		return market.Ticker{}, false
	}
	slow, _ := c.GetSlow(key)
	return market.Ticker{
		Exchange:    inst.Exchange,
		Symbol:      inst.Symbol,
		MarketType:  inst.MarketType,
		Bid:         fast.Bid,
		Ask:         fast.Ask,
		BidQty:      fast.BidQty,
		AskQty:      fast.AskQty,
		Volume24h:   slow.Volume24h,
		OpenInt:     slow.OpenInt,
		FundingRate: slow.FundingRate,
		Timestamp:   fast.UpdatedAt,
	}, true
}
