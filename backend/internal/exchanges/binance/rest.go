package binance

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

type restBookTicker struct {
	Symbol   string `json:"symbol"`
	BidPrice string `json:"bidPrice"`
	BidQty   string `json:"bidQty"`
	AskPrice string `json:"askPrice"`
	AskQty   string `json:"askQty"`
}

func (c *Client) FetchFastData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	spot := make(map[string]struct{})
	perp := make(map[string]struct{})
	for _, inst := range instruments {
		switch inst.MarketType {
		case market.Spot:
			spot[inst.Symbol] = struct{}{}
		case market.Perp:
			perp[inst.Symbol] = struct{}{}
		}
	}

	if len(spot) > 0 {
		var tickers []restBookTicker
		if err := c.get(ctx, restBaseURL+"/api/v3/ticker/bookTicker", &tickers); err != nil {
			return fmt.Errorf("binance spot bookTicker: %w", err)
		}
		writeBookTickers(ca, tickers, spot, market.Spot)
	}
	if len(perp) > 0 {
		var tickers []restBookTicker
		if err := c.get(ctx, frestBaseURL+"/fapi/v1/ticker/bookTicker", &tickers); err != nil {
			return fmt.Errorf("binance perp bookTicker: %w", err)
		}
		writeBookTickers(ca, tickers, perp, market.Perp)
	}
	return nil
}

func writeBookTickers(ca *cache.Cache, tickers []restBookTicker, allowed map[string]struct{}, mt market.MarketType) {
	now := time.Now()
	for _, t := range tickers {
		if _, ok := allowed[t.Symbol]; !ok {
			continue
		}
		bid, e1 := strconv.ParseFloat(t.BidPrice, 64)
		ask, e2 := strconv.ParseFloat(t.AskPrice, 64)
		bidQty, e3 := strconv.ParseFloat(t.BidQty, 64)
		askQty, e4 := strconv.ParseFloat(t.AskQty, 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || bid <= 0 || ask <= 0 {
			continue
		}
		key := market.InstrumentKey{Exchange: "binance", Symbol: t.Symbol, MarketType: mt}
		ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, BidQty: bidQty, AskQty: askQty, UpdatedAt: now})
	}
}
