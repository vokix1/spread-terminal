package binance

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/httpclient"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

const (
	restBaseURL  = "https://api.binance.com"
	frestBaseURL = "https://fapi.binance.com"
)

type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: httpclient.New(20 * time.Second)}
}

func (c *Client) Name() string { return "binance" }

type spotExchangeInfo struct {
	Symbols []struct {
		Symbol     string `json:"symbol"`
		BaseAsset  string `json:"baseAsset"`
		QuoteAsset string `json:"quoteAsset"`
		Status     string `json:"status"`
	} `json:"symbols"`
}

type perpExchangeInfo struct {
	Symbols []struct {
		Symbol       string `json:"symbol"`
		BaseAsset    string `json:"baseAsset"`
		QuoteAsset   string `json:"quoteAsset"`
		Status       string `json:"status"`
		ContractType string `json:"contractType"`
	} `json:"symbols"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	spot, err := c.fetchSpotInstruments(ctx)
	if err != nil {
		return nil, fmt.Errorf("binance spot instruments: %w", err)
	}
	perp, err := c.fetchPerpInstruments(ctx)
	if err != nil {
		return nil, fmt.Errorf("binance perp instruments: %w", err)
	}
	return append(spot, perp...), nil
}

func (c *Client) fetchSpotInstruments(ctx context.Context) ([]market.Instrument, error) {
	var data spotExchangeInfo
	if err := c.get(ctx, restBaseURL+"/api/v3/exchangeInfo", &data); err != nil {
		return nil, err
	}
	result := make([]market.Instrument, 0, len(data.Symbols))
	for _, s := range data.Symbols {
		if s.Status != "TRADING" || s.QuoteAsset != "USDT" {
			continue
		}
		result = append(result, market.Instrument{Exchange: "binance", Symbol: s.Symbol, Base: s.BaseAsset, Quote: s.QuoteAsset, MarketType: market.Spot})
	}
	return result, nil
}

func (c *Client) fetchPerpInstruments(ctx context.Context) ([]market.Instrument, error) {
	var data perpExchangeInfo
	if err := c.get(ctx, frestBaseURL+"/fapi/v1/exchangeInfo", &data); err != nil {
		return nil, err
	}
	result := make([]market.Instrument, 0, len(data.Symbols))
	for _, s := range data.Symbols {
		if s.Status != "TRADING" || s.ContractType != "PERPETUAL" || s.QuoteAsset != "USDT" {
			continue
		}
		result = append(result, market.Instrument{Exchange: "binance", Symbol: s.Symbol, Base: s.BaseAsset, Quote: s.QuoteAsset, MarketType: market.Perp})
	}
	return result, nil
}

type ticker24h struct {
	Symbol string `json:"symbol"`
	Volume string `json:"quoteVolume"`
}

type fundingRate struct {
	Symbol      string `json:"symbol"`
	FundingRate string `json:"lastFundingRate"`
}

type perpTicker struct {
	Symbol string `json:"symbol"`
	Volume string `json:"quoteVolume"`
}

func (c *Client) FetchSlowData(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	if err := c.fetchSpotVolumes(ctx, instruments, ca); err != nil {
		return err
	}
	return c.fetchPerpFunding(ctx, instruments, ca)
}

func (c *Client) fetchSpotVolumes(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	var tickers []ticker24h
	if err := c.get(ctx, restBaseURL+"/api/v3/ticker/24hr", &tickers); err != nil {
		return fmt.Errorf("binance spot 24hr: %w", err)
	}
	idx := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		idx[t.Symbol], _ = strconv.ParseFloat(t.Volume, 64)
	}
	now := time.Now()
	for _, inst := range instruments {
		if inst.MarketType != market.Spot {
			continue
		}
		key := market.InstrumentKey{Exchange: inst.Exchange, Symbol: inst.Symbol, MarketType: inst.MarketType}
		slow, _ := ca.GetSlow(key)
		slow.Volume24h = idx[inst.Symbol]
		slow.UpdatedAt = now
		ca.SetSlow(key, slow)
	}
	return nil
}

func (c *Client) fetchPerpFunding(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	var rates []fundingRate
	if err := c.get(ctx, frestBaseURL+"/fapi/v1/premiumIndex", &rates); err != nil {
		return fmt.Errorf("binance premium index: %w", err)
	}
	fundIdx := make(map[string]float64, len(rates))
	for _, r := range rates {
		fundIdx[r.Symbol], _ = strconv.ParseFloat(r.FundingRate, 64)
	}

	var tickers []perpTicker
	if err := c.get(ctx, frestBaseURL+"/fapi/v1/ticker/24hr", &tickers); err != nil {
		return fmt.Errorf("binance perp 24hr: %w", err)
	}
	volIdx := make(map[string]float64, len(tickers))
	for _, t := range tickers {
		volIdx[t.Symbol], _ = strconv.ParseFloat(t.Volume, 64)
	}

	now := time.Now()
	for _, inst := range instruments {
		if inst.MarketType != market.Perp {
			continue
		}
		key := market.InstrumentKey{Exchange: inst.Exchange, Symbol: inst.Symbol, MarketType: inst.MarketType}
		slow, _ := ca.GetSlow(key)
		slow.FundingRate = fundIdx[inst.Symbol]
		slow.Volume24h = volIdx[inst.Symbol]
		slow.UpdatedAt = now
		ca.SetSlow(key, slow)
	}
	return nil
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
