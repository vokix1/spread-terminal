// Package kucoin implements the Exchange interface for KuCoin.
package kucoin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges"
	"github.com/vokix1/spread-terminal/backend/internal/httpclient"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

const (
	spotBase    = "https://api.kucoin.com"
	futuresBase = "https://api-futures.kucoin.com"
)

type Client struct{ http *http.Client }

func New() *Client { return &Client{http: httpclient.New(20 * time.Second)} }
func (c *Client) Name() string { return "kucoin" }

// ─── FetchInstruments ────────────────────────────────────────────────────────

type kcResp[T any] struct {
	Code string `json:"code"`
	Data T      `json:"data"`
}

type kcSpotSymbol struct {
	Symbol          string `json:"symbol"`    // "BTC-USDT"
	BaseCurrency    string `json:"baseCurrency"`
	QuoteCurrency   string `json:"quoteCurrency"`
	EnableTrading   bool   `json:"enableTrading"`
}

type kcFutSymbol struct {
	Symbol       string `json:"symbol"`    // "XBTUSDTM"
	BaseCurrency string `json:"baseCurrency"`
	QuoteCurrency string `json:"quoteCurrency"`
	Status       string `json:"status"`
	IsInverse    bool   `json:"isInverse"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var result []market.Instrument

	var spotResp kcResp[[]kcSpotSymbol]
	if err := c.get(ctx, spotBase+"/api/v2/symbols", &spotResp); err != nil {
		return nil, fmt.Errorf("kucoin spot symbols: %w", err)
	}
	for _, s := range spotResp.Data {
		if !s.EnableTrading || s.QuoteCurrency != "USDT" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange: "kucoin", Symbol: s.Symbol,
			Base: s.BaseCurrency, Quote: s.QuoteCurrency, MarketType: market.Spot,
		})
	}

	var futResp kcResp[[]kcFutSymbol]
	if err := c.get(ctx, futuresBase+"/api/v1/contracts/active", &futResp); err != nil {
		return nil, fmt.Errorf("kucoin futures symbols: %w", err)
	}
	for _, s := range futResp.Data {
		if s.Status != "Open" || s.IsInverse {
			continue
		}
		if s.QuoteCurrency != "USDT" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange: "kucoin", Symbol: s.Symbol,
			Base: s.BaseCurrency, Quote: s.QuoteCurrency, MarketType: market.Perp,
		})
	}
	return result, nil
}

// ─── FetchSlowData ───────────────────────────────────────────────────────────

type kcTicker struct {
	Symbol    string `json:"symbol"`
	Vol       string `json:"volValue"` // quote volume
	Bid       string `json:"buy"`
	Ask       string `json:"sell"`
}

type kcTickersData struct {
	Ticker []kcTicker `json:"ticker"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	now := time.Now()

	var resp kcResp[kcTickersData]
	if err := c.get(ctx, spotBase+"/api/v1/market/allTickers", &resp); err != nil {
		return fmt.Errorf("kucoin allTickers: %w", err)
	}
	idx := make(map[string]kcTicker, len(resp.Data.Ticker))
	for _, t := range resp.Data.Ticker {
		idx[t.Symbol] = t
	}

	for _, inst := range instruments {
		if inst.MarketType != market.Spot {
			continue
		}
		key := market.InstrumentKey{Exchange: "kucoin", Symbol: inst.Symbol, MarketType: market.Spot}
		slow, _ := ca.GetSlow(key)
		if t, ok := idx[inst.Symbol]; ok {
			slow.Volume24h, _ = strconv.ParseFloat(t.Vol, 64)
		}
		slow.UpdatedAt = now
		ca.SetSlow(key, slow)
	}
	return nil
}

// ─── FetchFastData ───────────────────────────────────────────────────────────

func (c *Client) FetchFastData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	spotSet := make(map[string]bool)
	for _, inst := range instruments {
		if inst.MarketType == market.Spot {
			spotSet[inst.Symbol] = true
		}
	}
	now := time.Now()

	if len(spotSet) > 0 {
		var resp kcResp[kcTickersData]
		if err := c.get(ctx, spotBase+"/api/v1/market/allTickers", &resp); err != nil {
			return fmt.Errorf("kucoin fast tickers: %w", err)
		}
		for _, t := range resp.Data.Ticker {
			if !spotSet[t.Symbol] {
				continue
			}
			bid, e1 := strconv.ParseFloat(t.Bid, 64)
			ask, e2 := strconv.ParseFloat(t.Ask, 64)
			if e1 != nil || e2 != nil || bid <= 0 || ask <= 0 {
				continue
			}
			// Derive base from symbol "BTC-USDT" → "BTC"
			_ = strings.Split(t.Symbol, "-")
			key := market.InstrumentKey{Exchange: "kucoin", Symbol: t.Symbol, MarketType: market.Spot}
			ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, UpdatedAt: now})
		}
	}
	return nil
}

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "kucoin"}
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
