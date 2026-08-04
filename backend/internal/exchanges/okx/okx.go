// Package okx implements the Exchange interface for OKX.
package okx

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

const baseURL = "https://www.okx.com"

type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Name() string { return "okx" }

// ─── FetchInstruments ────────────────────────────────────────────────────────

type instResponse struct {
	Data []struct {
		InstID   string `json:"instId"`  // e.g. "BTC-USDT"
		BaseCcy  string `json:"baseCcy"`
		QuoteCcy string `json:"quoteCcy"`
		State    string `json:"state"`
		InstType string `json:"instType"` // SPOT / SWAP
	} `json:"data"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var result []market.Instrument

	for _, instType := range []string{"SPOT", "SWAP"} {
		url := fmt.Sprintf("%s/api/v5/public/instruments?instType=%s", baseURL, instType)
		var resp instResponse
		if err := c.get(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("okx instruments %s: %w", instType, err)
		}
		for _, d := range resp.Data {
			if d.State != "live" {
				continue
			}
			if d.QuoteCcy != "USDT" {
				continue
			}
			mtype := market.Spot
			if instType == "SWAP" {
				mtype = market.Perp
			}
			result = append(result, market.Instrument{
				Exchange:   "okx",
				Symbol:     d.InstID,
				Base:       d.BaseCcy,
				Quote:      d.QuoteCcy,
				MarketType: mtype,
			})
		}
	}
	return result, nil
}

// ─── FetchSlowData ───────────────────────────────────────────────────────────

type tickerResp struct {
	Data []struct {
		InstID  string `json:"instId"`
		Vol24h  string `json:"volCcy24h"` // quote volume
		OI      string `json:"oi"`
	} `json:"data"`
}

type fundingResp struct {
	Data []struct {
		InstID      string `json:"instId"`
		FundingRate string `json:"fundingRate"`
	} `json:"data"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	now := time.Now()

	// Spot/Swap tickers (volume + OI)
	for _, instType := range []string{"SPOT", "SWAP"} {
		url := fmt.Sprintf("%s/api/v5/market/tickers?instType=%s", baseURL, instType)
		var resp tickerResp
		if err := c.get(ctx, url, &resp); err != nil {
			return fmt.Errorf("okx tickers %s: %w", instType, err)
		}
		for _, d := range resp.Data {
			vol, _ := strconv.ParseFloat(d.Vol24h, 64)
			oi, _ := strconv.ParseFloat(d.OI, 64)
			mtype := market.Spot
			if instType == "SWAP" {
				mtype = market.Perp
			}
			key := market.InstrumentKey{Exchange: "okx", Symbol: d.InstID, MarketType: mtype}
			slow, _ := ca.GetSlow(key)
			slow.Volume24h = vol
			slow.OpenInt = oi
			slow.UpdatedAt = now
			ca.SetSlow(key, slow)
		}
	}

	// Funding rates (SWAP only) — per instrument
	for _, inst := range instruments {
		if inst.MarketType != market.Perp {
			continue
		}
		url := fmt.Sprintf("%s/api/v5/public/funding-rate?instId=%s", baseURL, inst.Symbol)
		var resp fundingResp
		if err := c.get(ctx, url, &resp); err != nil {
			continue
		}
		for _, d := range resp.Data {
			f, _ := strconv.ParseFloat(d.FundingRate, 64)
			key := market.InstrumentKey{Exchange: "okx", Symbol: d.InstID, MarketType: market.Perp}
			slow, _ := ca.GetSlow(key)
			slow.FundingRate = f
			slow.UpdatedAt = now
			ca.SetSlow(key, slow)
		}
	}

	return nil
}

// ─── FetchFastData ───────────────────────────────────────────────────────────
// OKX /api/v5/market/tickers returns bid1Px / ask1Px fields for all instruments
// in one call per instType. This is our REST fast-data source.

type fastTickerResp struct {
	Data []struct {
		InstID  string `json:"instId"`
		BidPx   string `json:"bidPx"`
		BidSz   string `json:"bidSz"`
		AskPx   string `json:"askPx"`
		AskSz   string `json:"askSz"`
	} `json:"data"`
}

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
		instType string
		mtype    market.MarketType
		set      map[string]bool
	}{
		{"SPOT", market.Spot, spotSet},
		{"SWAP", market.Perp, perpSet},
	} {
		if len(pair.set) == 0 {
			continue
		}
		url := fmt.Sprintf("%s/api/v5/market/tickers?instType=%s", baseURL, pair.instType)
		var resp fastTickerResp
		if err := c.get(ctx, url, &resp); err != nil {
			return fmt.Errorf("okx fast tickers %s: %w", pair.instType, err)
		}
		for _, d := range resp.Data {
			if !pair.set[d.InstID] {
				continue
			}
			bid, e1 := strconv.ParseFloat(d.BidPx, 64)
			ask, e2 := strconv.ParseFloat(d.AskPx, 64)
			bidQty, e3 := strconv.ParseFloat(d.BidSz, 64)
			askQty, e4 := strconv.ParseFloat(d.AskSz, 64)
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || bid <= 0 || ask <= 0 {
				continue
			}
			key := market.InstrumentKey{Exchange: "okx", Symbol: d.InstID, MarketType: pair.mtype}
			ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, BidQty: bidQty, AskQty: askQty, UpdatedAt: now})
		}
	}

	return nil
}

// ─── StreamFast ──────────────────────────────────────────────────────────────

// StreamFast — WS implementation is milestone 2.
func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "okx"}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
