// Package kraken implements the Exchange interface for Kraken (spot only).
package kraken

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

const baseURL = "https://api.kraken.com"

type Client struct{ http *http.Client }

func New() *Client { return &Client{http: httpclient.New(20 * time.Second)} }
func (c *Client) Name() string { return "kraken" }

// ─── FetchInstruments ────────────────────────────────────────────────────────

type krakenAssetPairsResp struct {
	Error  []string        `json:"error"`
	Result map[string]struct {
		Altname   string `json:"altname"`
		Base      string `json:"base"`
		Quote     string `json:"quote"`
		Status    string `json:"status"` // "online"
		WSName    string `json:"wsname"` // "BTC/USD"
	} `json:"result"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var resp krakenAssetPairsResp
	if err := c.get(ctx, baseURL+"/0/public/AssetPairs", &resp); err != nil {
		return nil, fmt.Errorf("kraken assetpairs: %w", err)
	}
	result := make([]market.Instrument, 0)
	for pair, info := range resp.Result {
		if info.Status != "online" {
			continue
		}
		// Skip dark pools (.d suffix)
		if strings.HasSuffix(pair, ".d") {
			continue
		}
		// WSName format: "BTC/USDT" — filter USDT pairs
		if !strings.HasSuffix(info.WSName, "/USDT") {
			continue
		}
		base := strings.TrimSuffix(info.WSName, "/USDT")
		result = append(result, market.Instrument{
			Exchange:   "kraken",
			Symbol:     pair,      // Kraken internal key e.g. "XBTUSDT"
			Base:       base,
			Quote:      "USDT",
			MarketType: market.Spot,
		})
	}
	return result, nil
}

// ─── FetchSlowData ───────────────────────────────────────────────────────────

type krakenTickerResp struct {
	Error  []string            `json:"error"`
	Result map[string]struct {
		V [2]string `json:"v"` // volume[0]=today, [1]=24h
		B [3]string `json:"b"` // bid[price, lot, qty]
		A [3]string `json:"a"` // ask[price, lot, qty]
	} `json:"result"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	// Kraken requires comma-separated pairs — batch all
	symbols := make([]string, 0, len(instruments))
	symIdx := make(map[string]bool)
	for _, inst := range instruments {
		if inst.MarketType == market.Spot {
			symbols = append(symbols, inst.Symbol)
			symIdx[inst.Symbol] = true
		}
	}
	if len(symbols) == 0 {
		return nil
	}
	// Kraken rate-limits — fetch in batches of 100
	now := time.Now()
	for i := 0; i < len(symbols); i += 100 {
		end := i + 100
		if end > len(symbols) {
			end = len(symbols)
		}
		batch := strings.Join(symbols[i:end], ",")
		var resp krakenTickerResp
		url := baseURL + "/0/public/Ticker?pair=" + batch
		if err := c.get(ctx, url, &resp); err != nil {
			continue // non-fatal per batch
		}
		for pair, t := range resp.Result {
			// map back to our symbol
			if !symIdx[pair] {
				// Kraken sometimes returns different key, try prefix match
				found := false
				for _, sym := range symbols {
					if strings.HasPrefix(pair, sym) || strings.HasPrefix(sym, pair) {
						pair = sym
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			vol, _ := strconv.ParseFloat(t.V[1], 64)
			key := market.InstrumentKey{Exchange: "kraken", Symbol: pair, MarketType: market.Spot}
			slow, _ := ca.GetSlow(key)
			slow.Volume24h = vol
			slow.UpdatedAt = now
			ca.SetSlow(key, slow)
		}
	}
	return nil
}

// ─── FetchFastData ───────────────────────────────────────────────────────────

func (c *Client) FetchFastData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	symbols := make([]string, 0, len(instruments))
	for _, inst := range instruments {
		if inst.MarketType == market.Spot {
			symbols = append(symbols, inst.Symbol)
		}
	}
	if len(symbols) == 0 {
		return nil
	}
	now := time.Now()
	// Batch fetch
	for i := 0; i < len(symbols); i += 100 {
		end := i + 100
		if end > len(symbols) {
			end = len(symbols)
		}
		batch := strings.Join(symbols[i:end], ",")
		var resp krakenTickerResp
		url := baseURL + "/0/public/Ticker?pair=" + batch
		if err := c.get(ctx, url, &resp); err != nil {
			continue
		}
		for pair, t := range resp.Result {
			bid, e1 := strconv.ParseFloat(t.B[0], 64)
			ask, e2 := strconv.ParseFloat(t.A[0], 64)
			if e1 != nil || e2 != nil || bid <= 0 || ask <= 0 {
				continue
			}
			// find the matching instrument symbol
			sym := pair
			for _, s := range symbols[i:end] {
				if s == pair || strings.HasPrefix(pair, s) {
					sym = s
					break
				}
			}
			key := market.InstrumentKey{Exchange: "kraken", Symbol: sym, MarketType: market.Spot}
			ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, UpdatedAt: now})
		}
	}
	return nil
}

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "kraken"}
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
