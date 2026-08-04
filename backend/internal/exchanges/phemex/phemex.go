// Package phemex implements the Exchange interface for Phemex.
package phemex

import (
	"context"
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

func New() *Client { return &Client{http: httpclient.New(30 * time.Second)} }
func (c *Client) Name() string { return "phemex" }

type phProduct struct {
	Symbol          string `json:"symbol"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	BaseCurrency    string `json:"baseCurrency"`
	QuoteCurrency   string `json:"quoteCurrency"`
	UnderlyingSymbol string `json:"underlyingSymbol"`
	ContractType    string `json:"contractType"`
	SettleCurrency  string `json:"settleCurrency"`
}

type phProducts struct {
	Code int `json:"code"`
	Data struct {
		Products       []phProduct `json:"products"`
		Perpetuals     []phProduct `json:"perpProductsV2"`
		LegacyPerpetuals []phProduct `json:"perpProductList"`
	} `json:"data"`
}

func (c *Client) FetchInstruments(ctx context.Context) ([]market.Instrument, error) {
	var resp phProducts
	if err := c.get(ctx, baseURL+"/public/products", &resp); err != nil {
		return nil, fmt.Errorf("phemex products: %w", err)
	}
	result := make([]market.Instrument, 0, len(resp.Data.Products)+len(resp.Data.Perpetuals))
	seen := make(map[market.InstrumentKey]struct{})

	for _, p := range resp.Data.Products {
		if !strings.EqualFold(p.Type, "Spot") || !isActive(p.Status) || p.QuoteCurrency != "USDT" {
			continue
		}
		base := p.BaseCurrency
		if base == "" {
			base = strings.TrimSuffix(strings.TrimPrefix(p.Symbol, "s"), p.QuoteCurrency)
		}
		appendInstrument(&result, seen, market.Instrument{Exchange: "phemex", Symbol: p.Symbol, Base: base, Quote: p.QuoteCurrency, MarketType: market.Spot})
	}

	perps := append(resp.Data.Perpetuals, resp.Data.LegacyPerpetuals...)
	for _, p := range perps {
		quote := p.QuoteCurrency
		if quote == "" {
			quote = p.SettleCurrency
		}
		if !isActive(p.Status) || quote != "USDT" {
			continue
		}
		base := p.BaseCurrency
		if base == "" {
			base = strings.TrimPrefix(p.UnderlyingSymbol, ".")
		}
		if base == "" {
			base = strings.TrimSuffix(p.Symbol, quote)
		}
		if base == "" {
			continue
		}
		appendInstrument(&result, seen, market.Instrument{Exchange: "phemex", Symbol: p.Symbol, Base: base, Quote: quote, MarketType: market.Perp})
	}
	return result, nil
}

func isActive(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status != "" && status != "delisted" && status != "closed" && status != "suspended"
}

func appendInstrument(dst *[]market.Instrument, seen map[market.InstrumentKey]struct{}, inst market.Instrument) {
	key := market.InstrumentKey{Exchange: inst.Exchange, Symbol: inst.Symbol, MarketType: inst.MarketType}
	if _, exists := seen[key]; exists {
		return
	}
	seen[key] = struct{}{}
	*dst = append(*dst, inst)
}

type phTicker struct {
	Symbol       string `json:"symbol"`
	BidPrice     string `json:"bidPriceRp"`
	AskPrice     string `json:"askPriceRp"`
	LegacyBid    string `json:"bidPrice"`
	LegacyAsk    string `json:"askPrice"`
	Turnover     string `json:"turnoverRv"`
	LegacyTurnover string `json:"turnoverEv"`
	OpenInterest string `json:"openInterestRv"`
	LegacyOI     string `json:"openInterest"`
	FundingRate  string `json:"fundingRateRr"`
	LegacyFunding string `json:"fundingRateEr"`
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
	types := instrumentTypes(instruments)
	now := time.Now()
	for _, t := range tickers {
		mt, ok := types[t.Symbol]
		if !ok {
			continue
		}
		vol := parseFirst(t.Turnover, t.LegacyTurnover)
		oi := parseFirst(t.OpenInterest, t.LegacyOI)
		fr := parseFirst(t.FundingRate, t.LegacyFunding)
		if t.FundingRate == "" && t.LegacyFunding != "" {
			fr /= 1e8
		}
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
	types := instrumentTypes(instruments)
	tickers, err := c.fetchAllTickers(ctx)
	if err != nil {
		return fmt.Errorf("phemex fast tickers: %w", err)
	}
	now := time.Now()
	for _, t := range tickers {
		mt, ok := types[t.Symbol]
		if !ok {
			continue
		}
		bid := parseFirst(t.BidPrice, t.LegacyBid)
		ask := parseFirst(t.AskPrice, t.LegacyAsk)
		if bid <= 0 || ask <= 0 {
			continue
		}
		key := market.InstrumentKey{Exchange: "phemex", Symbol: t.Symbol, MarketType: mt}
		ca.SetFast(key, cache.FastData{Bid: bid, Ask: ask, UpdatedAt: now})
	}
	return nil
}

func instrumentTypes(instruments []market.Instrument) map[string]market.MarketType {
	result := make(map[string]market.MarketType, len(instruments))
	for _, inst := range instruments {
		result[inst.Symbol] = inst.MarketType
	}
	return result
}

func parseFirst(values ...string) float64 {
	for _, value := range values {
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed
		}
	}
	return 0
}

func (c *Client) StreamFast(_ context.Context, _ []market.Instrument, _ *cache.Cache) error {
	return exchanges.ErrWSNotImplemented{Exchange: "phemex"}
}

func (c *Client) get(ctx context.Context, url string, dst interface{}) error {
	return httpclient.GetJSON(ctx, c.http, url, dst)
}
