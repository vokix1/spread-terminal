package market

import "time"


type MarketEvent struct {

	Exchange string `json:"exchange"`

	Symbol string `json:"symbol"`

	Bid float64 `json:"bid"`

	Ask float64 `json:"ask"`

	BidQty float64 `json:"bidQty"`

	AskQty float64 `json:"askQty"`

	Time time.Time `json:"time"`

}