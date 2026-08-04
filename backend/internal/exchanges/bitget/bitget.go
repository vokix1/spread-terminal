// Package bitget implements the Exchange interface for Bitget.
package bitget

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

const baseURL = "https://api.bitget.com"

type Client struct{ http *http.Client }

func New() *Client { return &Client{http: httpclient.New(20 * time.Second)} }
func (c *Client) Name() string { return "bitget" }

// ─── Generic response wrapper ─────────────────────────────────────────────────

type bgResp[T any] struct {
	Code string `json:"code"`
	Data T      `json:"data"`
}

// ─── FetchInstruments ────────────────────────────────────────────────────────

type bgSpotSymbol struct {
	Symbol      string `json:"symbol"`      // "BTCUSDT"
	BaseCoin    string `json:"baseCoin"`
	QuoteCoin   string `json:"quoteCoin"`
	Status      string `json:"status"`     // "online"
}

type bgPerpSymbol struct {
	Symbol        string `json:"symbol"`       // "BTCUSDT"
	BaseCoin      string `json:"baseCoin"`
	QuoteCoin     string `json:"quoteCoin"`
	ContractType  string `json:"contractType"` // "perpetual"
	Status        string `json:"status"`        // "normal"
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var result []market.Instrument

	// Spot
	var spotResp bgResp[[]bgSpotSymbol]
	if err := c.get(ctx, baseURL+"/api/v2/spot/public/symbols", &spotResp); err != nil {
		return nil, fmt.Errorf("bitget spot symbols: %w", err)
	}
	for _, s := range spotResp.Data {
		if s.Status != "online" || s.QuoteCoin != "USDT" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange: "bitget", Symbol: s.Symbol,
			Base: s.BaseCoin, Quote: s.QuoteCoin, MarketType: market.Spot,
		})
	}

	// USDT-M perpetuals
	var perpResp bgResp[[]bgPerpSymbol]
	if err := c.get(ctx, baseURL+"/api/v2/mix/market/contracts?productType=usdt-futures", &perpResp); err != nil {
		return nil, fmt.Errorf("bitget perp contracts: %w", err)
	}
	for _, s := range perpResp.Data {
		if s.Status != "normal" || s.QuoteCoin != "USDT" {
			continue
		}
		if s.ContractType != "perpetual" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange: "bitget", Symbol: s.Symbol,
			Base: s.BaseCoin, Quote: s.QuoteCoin, MarketType: market.Perp,
		})
	}
	return result, nil
}

// ─── FetchSlowData ───────────────────────────────────────────────────────────

type bgSpotTicker struct {
	Symbol      string `json:"symbol"`
	QuoteVolume string `json:"quoteVolume"`
	BidPr       string `json:"bidPr"`
	AskPr       string `json:"askPr"`
}

type bgPerpTicker struct {
	Symbol      string `json:"symbol"`
	QuoteVolume string `json:"quoteVolume"`
	HoldingAmount string `json:"holdingAmount"` // OI
	FundingRate string `json:"fundingRate"`
	BidPr       string `json:"bidPr"`
	AskPr       string `json:"askPr"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	now := time.Now()

	var spotResp bgResp[[]bgSpotTicker]
	if err := c.get(ctx, baseURL+"/api/v2/spot/market/tickers", &spotResp); err != nil {
		return fmt.Errorf("bitget spot tickers: %w", err)
	}
	for _, t := range spotResp.Data {
		vol, _ := strconv.ParseFloat(t.QuoteVolume, 64)
		key := market.InstrumentKey{Exchange: "bitget", Symbol: t.Symbol, MarketType: market.Spot}
		slow, _ := ca.GetSlow(key)
		slow.Volume24h = vol
		slow.UpdatedAt = now
		ca.SetSlow(key, slow)
	}

	var perpResp bgResp[[]bgPerpTicker]
	if err := c.get(ctx, baseURL+"/api/v2/mix/market/tickers?productType=usdt-futures", &perpResp); err != nil {
		return fmt.Errorf("bitget perp tickers: %w", err)
	}
	for _, t := range perpResp.Data {
		vol, _ := strconv.ParseFloat(t.QuoteVolume, 64)
		oi, _ := strconv.ParseFloat(t.HoldingAmount, 64)
		fr, _ := strconv.ParseFloat(t.FundingRate, 64)
		key := market.InstrumentKey{Exchange: "bitget", Symbol: t.Symbol, MarketType: market.Perp}
		slow, _ := ca.GetSlow(key)
		slow.Volume24h = vol
		slow.OpenInt = oi
		slow.FundingRate = fr
		slow.UpdatedAt = now
		ca.SetSlow(key, slow)
	}
	return nil
}

// ─── FetchFastData ───────────────────────────────────────────────────────────

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

	if len(spotSet) > 0 {
		var resp bgResp[[]bgSpotTicker]
		if err := c.get(ctx, baseURL+"/api/v2/spot/market/tickers", &resp); err != nil {
			return fmt.Errorf("bitget spot fast: %w", err)
		}
		for _, t := range resp.Data {
			if !spotSet[t.Symbol] {
				continue
			}
			bid, e1 := strconv.ParseFloat(t.BidPr, 64)
			ask, e2 := strconv.ParseFloat(t.AskPr, 64)
			if e1 != nil || e2 != nil || bid <= 0 || ask <= 0 {
				continue
			}
			key := market.InstrumentKey{Exchange: "bitget", Symbol: t.Symbol, MarketType: market.Spot}
			ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, UpdatedAt: now})
		}
	}

	if len(perpSet) > 0 {
		var resp bgResp[[]bgPerpTicker]
		if err := c.get(ctx, baseURL+"/api/v2/mix/market/tickers?productType=usdt-futures", &resp); err != nil {
			return fmt.Errorf("bitget perp fast: %w", err)
		}
		for _, t := range resp.Data {
			if !perpSet[t.Symbol] {
				continue
			}
			bid, e1 := strconv.ParseFloat(t.BidPr, 64)
			ask, e2 := strconv.ParseFloat(t.AskPr, 64)
			if e1 != nil || e2 != nil || bid <= 0 || ask <= 0 {
				continue
			}
			key := market.InstrumentKey{Exchange: "bitget", Symbol: t.Symbol, MarketType: market.Perp}
			ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, UpdatedAt: now})
		}
	}
	return nil
}

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "bitget"}
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
