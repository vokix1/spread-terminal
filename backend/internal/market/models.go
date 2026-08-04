package market

import "time"

// MarketType describes the trading venue type.
type MarketType string

const (
	Spot    MarketType = "spot"
	Perp    MarketType = "perp"
	Futures MarketType = "futures"
	Option  MarketType = "option"
)

// Instrument is a single tradable market on a single exchange.
type Instrument struct {
	Exchange   string     `json:"exchange"`
	Symbol     string     `json:"symbol"`   // native exchange symbol, e.g. "BTCUSDT"
	Base       string     `json:"base"`     // normalized, e.g. "BTC"
	Quote      string     `json:"quote"`    // e.g. "USDT"
	MarketType MarketType `json:"marketType"`
}

// InstrumentKey is the unique identifier across all exchanges.
type InstrumentKey struct {
	Exchange   string
	Symbol     string
	MarketType MarketType
}

// Ticker holds the best bid/ask for one instrument at a point in time.
type Ticker struct {
	Exchange   string     `json:"exchange"`
	Symbol     string     `json:"symbol"`
	MarketType MarketType `json:"marketType"`
	Bid        float64    `json:"bid"`
	Ask        float64    `json:"ask"`
	BidQty     float64    `json:"bidQty"`
	AskQty     float64    `json:"askQty"`
	Volume24h  float64    `json:"volume24h"`
	OpenInt    float64    `json:"openInterest"`
	FundingRate float64   `json:"fundingRate"`
	Timestamp  time.Time  `json:"timestamp"`
}

// Opportunity is a cross-exchange/cross-market spread opportunity.
type Opportunity struct {
	Base        string     `json:"base"`
	BuyExchange string     `json:"buyExchange"`
	BuySymbol   string     `json:"buySymbol"`
	BuyType     MarketType `json:"buyType"`
	BuyAsk      float64    `json:"buyAsk"`
	SellExchange string    `json:"sellExchange"`
	SellSymbol  string     `json:"sellSymbol"`
	SellType    MarketType `json:"sellType"`
	SellBid     float64    `json:"sellBid"`
	SpreadPct   float64    `json:"spreadPct"`
	Volume24h   float64    `json:"volume24h"`
	OpenInt     float64    `json:"openInterest"`
	FundingRate float64    `json:"fundingRate"`
	Score       float64    `json:"score"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
