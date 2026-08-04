// Package phemex implements the Exchange interface for Phemex.
package phemex

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

const baseURL = "https://api.phemex.com"

type Client struct{ http *http.Client }

func New() *Client { return &Client{http: httpclient.New(20 * time.Second)} }
func (c *Client) Name() string { return "phemex" }

// ─── FetchInstruments ────────────────────────────────────────────────────────

type phProducts struct {
	Code int `json:"code"`
	Data struct {
		Spot []struct {
			Symbol     string `json:"symbol"`     // "sBTCUSDT"
			BaseCurrency  string `json:"baseCurrency"`
			QuoteCurrency string `json:"quoteCurrency"`
			Status     string `json:"status"` // "Listed"
		} `json:"spot"`
		Perpetuals []struct {
			Symbol         string `json:"symbol"`       // "BTCUSDT"
			UnderlyingSymbol string `json:"underlyingSymbol"` // ".BTC"
			QuoteCurrency  string `json:"quoteCurrency"`
			ContractType   string `json:"contractType"`  // "Linear"
			Status         string `json:"status"`
		} `json:"perpProductList"`
	} `json:"data"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var resp phProducts
	if err := c.get(ctx, baseURL+"/public/products", &resp); err != nil {
		return nil, fmt.Errorf("phemex products: %w", err)
	}
	result := make([]market.Instrument, 0)

	for _, s := range resp.Data.Spot {
		if s.Status != "Listed" || s.QuoteCurrency != "USDT" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange:   "phemex",
			Symbol:     s.Symbol,
			Base:       s.BaseCurrency,
			Quote:      s.QuoteCurrency,
			MarketType: market.Spot,
		})
	}

	for _, p := range resp.Data.Perpetuals {
		if p.Status != "Listed" || p.QuoteCurrency != "USDT" || p.ContractType != "Linear" {
			continue
		}
		// Extract base from underlyingSymbol ".BTC" → "BTC"
		base := strings.TrimPrefix(p.UnderlyingSymbol, ".")
		if base == "" {
			continue
		}
		result = append(result, market.Instrument{
			Exchange:   "phemex",
			Symbol:     p.Symbol,
			Base:       base,
			Quote:      p.QuoteCurrency,
			MarketType: market.Perp,
		})
	}
	return result, nil
}

// ─── FetchSlowData / FetchFastData ───────────────────────────────────────────
// Phemex /md/v3/ticker/24hr returns all tickers including bid/ask.

type phTicker struct {
	Symbol      string `json:"symbol"`
	BidPrice    string `json:"bidPrice"`
	AskPrice    string `json:"askPrice"`
	Volume      string `json:"volumeEv"` // base volume — use turnover for USDT vol
	Turnover    string `json:"turnoverEv"`
	OpenInterest string `json:"openInterest"`
	FundingRate string `json:"fundingRateEr"` // scaled
}

type phTickerResp struct {
	Code int `json:"code"`
	Data struct {
		Tickers []phTicker `json:"tickers"`
	} `json:"data"`
}

func (c *Client) fetchAllTickers(ctx context.Context) ([]phTicker, error) {
	var resp phTickerResp
	if err := c.get(ctx, baseURL+"/md/v3/ticker/24hr?symbol=", &resp); err != nil {
		return nil, err
	}
	return resp.Data.Tickers, nil
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	tickers, err := c.fetchAllTickers(ctx)
	if err != nil {
		return fmt.Errorf("phemex tickers: %w", err)
	}
	now := time.Now()
	mtype := func(sym string) market.MarketType {
		// Phemex spot symbols start with 's': "sBTCUSDT"
		if strings.HasPrefix(sym, "s") {
			return market.Spot
		}
		return market.Perp
	}
	for _, t := range tickers {
		vol, _ := strconv.ParseFloat(t.Turnover, 64)
		oi, _ := strconv.ParseFloat(t.OpenInterest, 64)
		fr, _ := strconv.ParseFloat(t.FundingRate, 64)
		fr = fr / 1e8 // Phemex funding rate is scaled by 1e8
		mt := mtype(t.Symbol)
		key := market.InstrumentKey{Exchange: "phemex", Symbol: t.Symbol, MarketType: mt}
		slow, _ := ca.GetSlow(key)
		slow.Volume24h = vol
		slow.OpenInt = oi
		slow.FundingRate = fr
		slow.UpdatedAt = now
		ca.SetSlow(key, slow)
	}
	return nil
}

func (c *Client) FetchFastData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	symSet := make(map[string]market.MarketType, len(instruments))
	for _, inst := range instruments {
		symSet[inst.Symbol] = inst.MarketType
	}
	tickers, err := c.fetchAllTickers(ctx)
	if err != nil {
		return fmt.Errorf("phemex fast tickers: %w", err)
	}
	now := time.Now()
	for _, t := range tickers {
		mt, ok := symSet[t.Symbol]
		if !ok {
			continue
		}
		bid, e1 := strconv.ParseFloat(t.BidPrice, 64)
		ask, e2 := strconv.ParseFloat(t.AskPrice, 64)
		if e1 != nil || e2 != nil || bid <= 0 || ask <= 0 {
			continue
		}
		key := market.InstrumentKey{Exchange: "phemex", Symbol: t.Symbol, MarketType: mt}
		ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, UpdatedAt: now})
	}
	return nil
}

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "phemex"}
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
