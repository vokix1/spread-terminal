package market

import "time"


type Instrument struct {
	Symbol string `json:"symbol"`
	Base string `json:"base"`
	Quote string `json:"quote"`

	Type string `json:"type"`
	
	Exchange string `json:"exchange"`
}


type Ticker struct {

	Exchange string `json:"exchange"`

	Symbol string `json:"symbol"`

	Bid float64 `json:"bid"`

	Ask float64 `json:"ask"`

	Last float64 `json:"last"`

	Volume24h float64 `json:"volume24h"`

	Timestamp time.Time `json:"timestamp"`
}