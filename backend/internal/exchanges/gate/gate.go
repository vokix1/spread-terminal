// Package gate implements the Exchange interface for Gate.io.
package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges"
	"github.com/vokix1/spread-terminal/backend/internal/httpclient"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

const baseURL = "https://api.gateio.ws/api/v4"

type Client struct{ http *http.Client }

func New() *Client { return &Client{http: httpclient.New(20 * time.Second)} }
func (c *Client) Name() string { return "gate" }

// ─── FetchInstruments ────────────────────────────────────────────────────────

type spotPair struct {
	ID          string `json:"id"`           // e.g. "BTC_USDT"
	Base        string `json:"base"`
	Quote       string `json:"quote"`
	TradeStatus string `json:"trade_status"` // "tradable"
}

type perpContract struct {
	Name         string `json:"name"`         // e.g. "BTC_USDT"
	QuantoMultiplier string `json:"quanto_multiplier"`
	SettleCcy    string `json:"settle"` // "usdt"
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var result []market.Instrument

	// Spot
	var pairs []spotPair
	if err := c.get(ctx, baseURL+"/spot/currency_pairs", &pairs); err != nil {
		return nil, fmt.Errorf("gate spot pairs: %w", err)
	}
	for _, p := range pairs {
		if p.TradeStatus != "tradable" || p.Quote != "USDT" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange: "gate", Symbol: p.ID, Base: p.Base, Quote: p.Quote, MarketType: market.Spot,
		})
	}

	// Perp (USDT-settled)
	var contracts []perpContract
	if err := c.get(ctx, baseURL+"/futures/usdt/contracts", &contracts); err != nil {
		return nil, fmt.Errorf("gate perp contracts: %w", err)
	}
	for _, ct := range contracts {
		// Gate perp name like "BTC_USDT" — extract base
		base := ""
		for i := 0; i < len(ct.Name); i++ {
			if ct.Name[i] == '_' {
				base = ct.Name[:i]
				break
			}
		}
		if base == "" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange: "gate", Symbol: ct.Name, Base: base, Quote: "USDT", MarketType: market.Perp,
		})
	}

	return result, nil
}

// ─── FetchSlowData ───────────────────────────────────────────────────────────

type spotTicker struct {
	CurrencyPair string `json:"currency_pair"`
	QuoteVolume  string `json:"quote_volume"` // USDT volume
	HighestBid   string `json:"highest_bid"`
	LowestAsk    string `json:"lowest_ask"`
}

type perpTicker struct {
	Contract    string `json:"contract"`
	Volume24hQuote string `json:"volume_24h_quote"`
	FundingRate string `json:"funding_rate"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	now := time.Now()

	// Spot tickers
	var spotTickers []spotTicker
	if err := c.get(ctx, baseURL+"/spot/tickers", &spotTickers); err != nil {
		return fmt.Errorf("gate spot tickers: %w", err)
	}
	spotVol := make(map[string]float64, len(spotTickers))
	for _, t := range spotTickers {
		v, _ := strconv.ParseFloat(t.QuoteVolume, 64)
		spotVol[t.CurrencyPair] = v
	}

	// Perp tickers
	var perpTickers []perpTicker
	if err := c.get(ctx, baseURL+"/futures/usdt/tickers", &perpTickers); err != nil {
		return fmt.Errorf("gate perp tickers: %w", err)
	}
	perpVol := make(map[string]float64)
	perpFund := make(map[string]float64)
	for _, t := range perpTickers {
		v, _ := strconv.ParseFloat(t.Volume24hQuote, 64)
		f, _ := strconv.ParseFloat(t.FundingRate, 64)
		perpVol[t.Contract] = v
		perpFund[t.Contract] = f
	}

	for _, inst := range instruments {
		key := market.InstrumentKey{Exchange: "gate", Symbol: inst.Symbol, MarketType: inst.MarketType}
		slow, _ := ca.GetSlow(key)
		slow.UpdatedAt = now
		if inst.MarketType == market.Spot {
			slow.Volume24h = spotVol[inst.Symbol]
		} else {
			slow.Volume24h = perpVol[inst.Symbol]
			slow.FundingRate = perpFund[inst.Symbol]
		}
		ca.SetSlow(key, slow)
	}
	return nil
}

// ─── FetchFastData ───────────────────────────────────────────────────────────

type spotBookTicker struct {
	CurrencyPair string `json:"currency_pair"`
	HighestBid   string `json:"highest_bid"`
	LowestAsk    string `json:"lowest_ask"`
}

type perpOrderBook struct {
	Contract string `json:"contract"`
	Bids     [][]string `json:"bids"` // [[price, size]]
	Asks     [][]string `json:"asks"`
}

func (c *Client) FetchFastData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	spotSet := make(map[string]bool)
	perpSet := make(map[string]bool)
	for _, inst := range instruments {
		switch inst.MarketType {
		case market.Spot:
			spotSet[inst.Symbol] = true
		case market.Perp:
			perpSet[inst.Symbol] = true
		}
	}
	now := time.Now()

	// Gate spot tickers have bid/ask
	if len(spotSet) > 0 {
		var tickers []spotTicker
		if err := c.get(ctx, baseURL+"/spot/tickers", &tickers); err != nil {
			return fmt.Errorf("gate spot fast tickers: %w", err)
		}
		for _, t := range tickers {
			if !spotSet[t.CurrencyPair] {
				continue
			}
			bid, e1 := strconv.ParseFloat(t.HighestBid, 64)
			ask, e2 := strconv.ParseFloat(t.LowestAsk, 64)
			if e1 != nil || e2 != nil || bid <= 0 || ask <= 0 {
				continue
			}
			key := market.InstrumentKey{Exchange: "gate", Symbol: t.CurrencyPair, MarketType: market.Spot}
			ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, UpdatedAt: now})
		}
	}

	// Gate perp tickers
	if len(perpSet) > 0 {
		type perpBidAsk struct {
			Contract   string `json:"contract"`
			Mark       string `json:"mark_price"`
			IndexPrice string `json:"index_price"`
			Bid1Price  string `json:"highest_bid"`
			Ask1Price  string `json:"lowest_ask"`
		}
		var tickers []perpBidAsk
		if err := c.get(ctx, baseURL+"/futures/usdt/tickers", &tickers); err != nil {
			return fmt.Errorf("gate perp fast tickers: %w", err)
		}
		for _, t := range tickers {
			if !perpSet[t.Contract] {
				continue
			}
			bid, e1 := strconv.ParseFloat(t.Bid1Price, 64)
			ask, e2 := strconv.ParseFloat(t.Ask1Price, 64)
			if e1 != nil || e2 != nil || bid <= 0 || ask <= 0 {
				continue
			}
			key := market.InstrumentKey{Exchange: "gate", Symbol: t.Contract, MarketType: market.Perp}
			ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, UpdatedAt: now})
		}
	}
	return nil
}

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "gate"}
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
