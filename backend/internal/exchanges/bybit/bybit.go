// Package bybit implements the Exchange interface for Bybit.
package bybit

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

const baseURL = "https://api.bybit.com"

type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Name() string { return "bybit" }

// ─── FetchInstruments ────────────────────────────────────────────────────────

type instrResp struct {
	Result struct {
		List []struct {
			Symbol       string `json:"symbol"`
			BaseCoin     string `json:"baseCoin"`
			QuoteCoin    string `json:"quoteCoin"`
			Status       string `json:"status"`
			ContractType string `json:"contractType"`
		} `json:"list"`
	} `json:"result"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var result []market.Instrument

	categories := []struct {
		cat   string
		mtype market.MarketType
	}{
		{"spot", market.Spot},
		{"linear", market.Perp}, // USDT perpetuals
	}

	for _, cat := range categories {
		url := fmt.Sprintf("%s/v5/market/instruments-info?category=%s&limit=1000", baseURL, cat.cat)
		var resp instrResp
		if err := c.get(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("bybit %s instruments: %w", cat.cat, err)
		}
		for _, item := range resp.Result.List {
			if item.Status != "Trading" {
				continue
			}
			if item.QuoteCoin != "USDT" {
				continue
			}
			if cat.mtype == market.Perp && item.ContractType != "LinearPerpetual" {
				continue
			}
			result = append(result, market.Instrument{
				Exchange:   "bybit",
				Symbol:     item.Symbol,
				Base:       item.BaseCoin,
				Quote:      item.QuoteCoin,
				MarketType: cat.mtype,
			})
		}
	}
	return result, nil
}

// ─── FetchSlowData ───────────────────────────────────────────────────────────

type tickerResp struct {
	Result struct {
		List []struct {
			Symbol       string `json:"symbol"`
			Volume24h    string `json:"volume24h"`
			OpenInterest string `json:"openInterest"`
			FundingRate  string `json:"fundingRate"`
			Bid1Price    string `json:"bid1Price"`
			Bid1Size     string `json:"bid1Size"`
			Ask1Price    string `json:"ask1Price"`
			Ask1Size     string `json:"ask1Size"`
		} `json:"list"`
	} `json:"result"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	now := time.Now()
	for _, cat := range []string{"spot", "linear"} {
		mtype := market.Spot
		if cat == "linear" {
			mtype = market.Perp
		}
		url := fmt.Sprintf("%s/v5/market/tickers?category=%s", baseURL, cat)
		var resp tickerResp
		if err := c.get(ctx, url, &resp); err != nil {
			return fmt.Errorf("bybit tickers %s: %w", cat, err)
		}
		for _, item := range resp.Result.List {
			vol, _ := strconv.ParseFloat(item.Volume24h, 64)
			oi, _ := strconv.ParseFloat(item.OpenInterest, 64)
			fr, _ := strconv.ParseFloat(item.FundingRate, 64)
			key := market.InstrumentKey{Exchange: "bybit", Symbol: item.Symbol, MarketType: mtype}
			slow, _ := ca.GetSlow(key)
			slow.Volume24h = vol
			slow.OpenInt = oi
			slow.FundingRate = fr
			slow.UpdatedAt = now
			ca.SetSlow(key, slow)
		}
	}
	return nil
}

// ─── FetchFastData ───────────────────────────────────────────────────────────
// Bybit /v5/market/tickers returns bid1Price/ask1Price in the same call as
// FetchSlowData, so we reuse that endpoint and just write the fast-cache layer.

func (c *Client) FetchFastData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	// Build symbol sets per market type.
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

	for _, pair := range []struct {
		cat   string
		mtype market.MarketType
		set   map[string]bool
	}{
		{"spot", market.Spot, spotSet},
		{"linear", market.Perp, perpSet},
	} {
		if len(pair.set) == 0 {
			continue
		}
		url := fmt.Sprintf("%s/v5/market/tickers?category=%s", baseURL, pair.cat)
		var resp tickerResp
		if err := c.get(ctx, url, &resp); err != nil {
			return fmt.Errorf("bybit fast tickers %s: %w", pair.cat, err)
		}
		for _, item := range resp.Result.List {
			if !pair.set[item.Symbol] {
				continue
			}
			bid, e1 := strconv.ParseFloat(item.Bid1Price, 64)
			ask, e2 := strconv.ParseFloat(item.Ask1Price, 64)
			bidQty, e3 := strconv.ParseFloat(item.Bid1Size, 64)
			askQty, e4 := strconv.ParseFloat(item.Ask1Size, 64)
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || bid <= 0 || ask <= 0 {
				continue
			}
			key := market.InstrumentKey{Exchange: "bybit", Symbol: item.Symbol, MarketType: pair.mtype}
			ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, BidQty: bidQty, AskQty: askQty, UpdatedAt: now})
		}
	}

	return nil
}

// ─── StreamFast ──────────────────────────────────────────────────────────────

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "bybit"}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
