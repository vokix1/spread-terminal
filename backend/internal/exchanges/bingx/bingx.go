// Package bingx implements the Exchange interface for BingX.
package bingx

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

const baseURL = "https://open-api.bingx.com"

type Client struct{ http *http.Client }

func New() *Client { return &Client{http: httpclient.New(20 * time.Second)} }
func (c *Client) Name() string { return "bingx" }

type flexibleStatus string

func (s *flexibleStatus) UnmarshalJSON(data []byte) error {
	var text string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*s = flexibleStatus(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*s = flexibleStatus(number.String())
	return nil
}

func (s flexibleStatus) trading() bool {
	value := strings.ToUpper(strings.TrimSpace(string(s)))
	return value == "TRADING" || value == "ONLINE" || value == "1"
}

type bxSymbolResp struct {
	Code int `json:"code"`
	Data struct {
		Symbols []struct {
			Symbol string         `json:"symbol"`
			Status flexibleStatus `json:"status"`
		} `json:"symbols"`
	} `json:"data"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var resp bxSymbolResp
	if err := c.get(ctx, baseURL+"/openApi/spot/v1/common/symbols", &resp); err != nil {
		return nil, fmt.Errorf("bingx symbols: %w", err)
	}
	result := make([]market.Instrument, 0, len(resp.Data.Symbols))
	for _, s := range resp.Data.Symbols {
		if !s.Status.trading() {
			continue
		}
		base, quote := splitSymbol(s.Symbol)
		if quote != "USDT" || base == "" {
			continue
		}
		result = append(result, market.Instrument{Exchange: "bingx", Symbol: s.Symbol, Base: base, Quote: quote, MarketType: market.Spot})
	}
	return result, nil
}

type bxTickerResp struct {
	Code int `json:"code"`
	Data []struct {
		Symbol      string `json:"symbol"`
		QuoteVolume string `json:"quoteVolume"`
	} `json:"data"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	var resp bxTickerResp
	if err := c.get(ctx, baseURL+"/openApi/spot/v1/ticker/24hr", &resp); err != nil {
		return fmt.Errorf("bingx 24hr: %w", err)
	}
	now := time.Now()
	volIdx := make(map[string]float64, len(resp.Data))
	for _, t := range resp.Data {
		v, _ := strconv.ParseFloat(t.QuoteVolume, 64)
		volIdx[t.Symbol] = v
	}
	for _, inst := range instruments {
		key := market.InstrumentKey{Exchange: "bingx", Symbol: inst.Symbol, MarketType: inst.MarketType}
		slow, _ := ca.GetSlow(key)
		slow.Volume24h = volIdx[inst.Symbol]
		slow.UpdatedAt = now
		ca.SetSlow(key, slow)
	}
	return nil
}

type bxAllTickerResp struct {
	Code int `json:"code"`
	Data []struct {
		Symbol   string `json:"symbol"`
		BidPrice string `json:"bidPrice"`
		BidQty   string `json:"bidQty"`
		AskPrice string `json:"askPrice"`
		AskQty   string `json:"askQty"`
	} `json:"data"`
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

	var resp bxAllTickerResp
	if err := c.get(ctx, baseURL+"/openApi/spot/v1/ticker/bookTicker", &resp); err != nil {
		return fmt.Errorf("bingx bookTicker: %w", err)
	}
	now := time.Now()
	for _, t := range resp.Data {
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
		key := market.InstrumentKey{Exchange: "bingx", Symbol: t.Symbol, MarketType: market.Spot}
		ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, BidQty: bidQty, AskQty: askQty, UpdatedAt: now})
	}
	return nil
}

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "bingx"}
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}

func splitSymbol(s string) (base, quote string) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return s, ""
	}
	return parts[0], parts[1]
}
