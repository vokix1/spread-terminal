package binance


import (
	"context"

	"github.com/vokix1/spread-terminal/backend/internal/market"
)



type Client struct{}



func New() *Client {

	return &Client{}

}



func (c *Client) Name() string {

	return "binance"

}



func (c *Client) FetchInstruments(
	ctx context.Context,
)([]market.Instrument,error){

	return []market.Instrument{},nil

}



func (c *Client) FetchTickers(
	ctx context.Context,
)([]market.Ticker,error){

	return []market.Ticker{},nil

}