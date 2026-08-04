// Package mexc implements the Exchange interface for MEXC.
package mexc

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

const baseURL = "https://api.mexc.com"

type Client struct{ http *http.Client }

func New() *Client { return &Client{http: httpclient.New(20 * time.Second)} }
func (c *Client) Name() string { return "mexc" }

// ─── FetchInstruments ────────────────────────────────────────────────────────

type mexcExchangeInfo struct {
	Symbols []struct {
		Symbol      string `json:"symbol"`
		BaseAsset   string `json:"baseAsset"`
		QuoteAsset  string `json:"quoteAsset"`
		Status      string `json:"status"` // "1" = trading
		IsSpotTradingAllowed bool `json:"isSpotTradingAllowed"`
	} `json:"symbols"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var info mexcExchangeInfo
	if err := c.get(ctx, baseURL+"/api/v3/exchangeInfo", &info); err != nil {
		return nil, fmt.Errorf("mexc exchangeInfo: %w", err)
	}
	result := make([]market.Instrument, 0, len(info.Symbols))
	for _, s := range info.Symbols {
		if s.Status != "1" || !s.IsSpotTradingAllowed {
			continue
		}
		if s.QuoteAsset != "USDT" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange:   "mexc",
			Symbol:     s.Symbol,
			Base:       s.BaseAsset,
			Quote:      s.QuoteAsset,
			MarketType: market.Spot,
		})
	}
	return result, nil
}

// ─── FetchSlowData ───────────────────────────────────────────────────────────

type mexcTicker24h struct {
	Symbol      string `json:"symbol"`
	QuoteVolume string `json:"quoteVolume"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	var tickers []mexcTicker24h
	if err := c.get(ctx, baseURL+"/api/v3/ticker/24hr", &tickers); err != nil {
		return fmt.Errorf("mexc 24hr: %w", err)
	}
	volIdx := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		v, _ := strconv.ParseFloat(t.QuoteVolume, 64)
		volIdx[t.Symbol] = v
	}
	now := time.Now()
	for _, inst := range instruments {
		key := market.InstrumentKey{Exchange: "mexc", Symbol: inst.Symbol, MarketType: inst.MarketType}
		slow, _ := ca.GetSlow(key)
		slow.Volume24h = volIdx[inst.Symbol]
		slow.UpdatedAt = now
		ca.SetSlow(key, slow)
	}
	return nil
}

// ─── FetchFastData ───────────────────────────────────────────────────────────

type mexcBookTicker struct {
	Symbol   string `json:"symbol"`
	BidPrice string `json:"bidPrice"`
	BidQty   string `json:"bidQty"`
	AskPrice string `json:"askPrice"`
	AskQty   string `json:"askQty"`
}

func (c *Client) FetchFastData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	spotSet := make(map[string]bool)
	for _, inst := range instruments {
		if inst.MarketType == market.Spot {
			spotSet[inst.Symbol] = true
		}
	}
	if len(spotSet) == 0 {
		return nil
	}

	var tickers []mexcBookTicker
	if err := c.get(ctx, baseURL+"/api/v3/ticker/bookTicker", &tickers); err != nil {
		return fmt.Errorf("mexc bookTicker: %w", err)
	}
	now := time.Now()
	for _, t := range tickers {
		if !spotSet[t.Symbol] {
			continue
		}
		bid, e1 := strconv.ParseFloat(t.BidPrice, 64)
		ask, e2 := strconv.ParseFloat(t.AskPrice, 64)
		bidQty, _ := strconv.ParseFloat(t.BidQty, 64)
		askQty, _ := strconv.ParseFloat(t.AskQty, 64)
		if e1 != nil || e2 != nil || bid <= 0 || ask <= 0 {
			continue
		}
		key := market.InstrumentKey{Exchange: "mexc", Symbol: t.Symbol, MarketType: market.Spot}
		ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, BidQty: bidQty, AskQty: askQty, UpdatedAt: now})
	}
	return nil
}

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "mexc"}
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
