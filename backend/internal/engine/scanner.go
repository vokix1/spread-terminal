// Package engine contains the opportunity scanning and scoring logic.
// The Scanner knows nothing about specific exchanges — it works with the
// Registry and Cache only.
package engine

import (
	"math"
	"sort"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

// Scanner builds cross-exchange opportunities from the registry + cache.
type Scanner struct {
	registry *market.Registry
	cache    *cache.Cache
}

func NewScanner(r *market.Registry, c *cache.Cache) *Scanner {
	return &Scanner{registry: r, cache: c}
}

// Scan iterates all base assets, builds all market combinations,
// and returns all opportunities with their scores.
func (s *Scanner) Scan() []market.Opportunity {
	bases := s.registry.Bases()
	result := make([]market.Opportunity, 0, len(bases)*4)

	for _, base := range bases {
		markets := s.registry.MarketsFor(base)
		if len(markets) < 2 {
			continue
		}

		// Collect live tickers for all markets of this base asset.
		type liveMarket struct {
			inst   market.Instrument
			ticker market.Ticker
		}
		live := make([]liveMarket, 0, len(markets))
		for _, inst := range markets {
			t, ok := s.cache.Ticker(inst)
			if !ok {
				continue
			}
			if t.Bid <= 0 || t.Ask <= 0 {
				continue
			}
			live = append(live, liveMarket{inst, t})
		}

		if len(live) < 2 {
			continue
		}

		// Build all pairs (i, j) where i is the buy side, j is the sell side.
		for i := 0; i < len(live); i++ {
			for j := 0; j < len(live); j++ {
				if i == j {
					continue
				}
				buy := live[i]
				sell := live[j]

				// Spread: buy at ask on buy side, sell at bid on sell side.
				spreadPct := (sell.ticker.Bid - buy.ticker.Ask) / buy.ticker.Ask * 100
				if spreadPct < 0 {
					continue // no positive opportunity
				}

				opp := market.Opportunity{
					Base:         base,
					BuyExchange:  buy.inst.Exchange,
					BuySymbol:    buy.inst.Symbol,
					BuyType:      buy.inst.MarketType,
					BuyAsk:       buy.ticker.Ask,
					SellExchange: sell.inst.Exchange,
					SellSymbol:   sell.inst.Symbol,
					SellType:     sell.inst.MarketType,
					SellBid:      sell.ticker.Bid,
					SpreadPct:    spreadPct,
					Volume24h:    (buy.ticker.Volume24h + sell.ticker.Volume24h) / 2,
					OpenInt:      math.Max(buy.ticker.OpenInt, sell.ticker.OpenInt),
					FundingRate:  sell.ticker.FundingRate - buy.ticker.FundingRate,
					Score:        score(spreadPct, buy.ticker, sell.ticker),
					UpdatedAt:    time.Now(),
				}
				result = append(result, opp)
			}
		}
	}

	// Sort by score descending.
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	return result
}

// score computes a composite opportunity score (0-100).
// Weights are tunable; current defaults reflect practical experience:
//   - spread contributes the most (trading alpha)
//   - volume ensures the pair is liquid enough to trade
//   - OI signals market depth for perp legs
//   - funding adds carry premium/cost context
func score(spreadPct float64, buy, sell market.Ticker) float64 {
	// Normalize spread: 0% → 0, 2%+ → 100
	spreadScore := math.Min(spreadPct/2.0*100, 100)

	// Normalize volume: log scale, 0 → 0, $100M+ → 100
	avgVol := (buy.Volume24h + sell.Volume24h) / 2
	volScore := 0.0
	if avgVol > 0 {
		volScore = math.Min(math.Log10(avgVol/1000)*25, 100)
	}

	// OI signal (perp legs only)
	oiScore := 0.0
	maxOI := math.Max(buy.OpenInt, sell.OpenInt)
	if maxOI > 0 {
		oiScore = math.Min(math.Log10(maxOI/1000)*20, 100)
	}

	// Funding contribution: absolute value of rate differential
	fundingScore := math.Min(math.Abs(buy.FundingRate-sell.FundingRate)*10000, 100)

	// Liquidity at best price (USD value of top-of-book)
	topBook := buy.Bid*buy.BidQty + sell.Ask*sell.AskQty
	liqScore := 0.0
	if topBook > 0 {
		liqScore = math.Min(math.Log10(topBook/100)*20, 100)
	}

	// Weighted sum
	return 0.40*spreadScore +
		0.25*volScore +
		0.15*oiScore +
		0.10*fundingScore +
		0.10*liqScore
}
