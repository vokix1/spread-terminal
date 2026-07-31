package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/vokix1/spread-terminal/backend/internal/market"
)

const baseURL = "https://api.binance.com"


type Client struct {
	httpClient *http.Client
}


func New() *Client {
	return &Client{
		httpClient: &http.Client{},
	}
}


func (c *Client) Name() string {
	return "binance"
}


type exchangeInfoResponse struct {
	Symbols []struct {

		Symbol string `json:"symbol"`

		BaseAsset string `json:"baseAsset"`

		QuoteAsset string `json:"quoteAsset"`

		Status string `json:"status"`

	} `json:"symbols"`
}



func (c *Client) FetchInstruments(
	ctx context.Context,
)([]market.Instrument,error){

	req,err:=http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/api/v3/exchangeInfo",
		nil,
	)

	if err!=nil{
		return nil,err
	}


	resp,err:=c.httpClient.Do(req)

	if err!=nil{
		return nil,err
	}

	defer resp.Body.Close()


	if resp.StatusCode != http.StatusOK {

		return nil,
			fmt.Errorf(
				"binance exchange info status: %d",
				resp.StatusCode,
			)

	}


	var data exchangeInfoResponse


	if err:=json.NewDecoder(resp.Body).Decode(&data); err!=nil{
		return nil,err
	}


	result:=make([]market.Instrument,0)


	for _,item:=range data.Symbols{


		if item.Status!="TRADING"{
			continue
		}


		result=append(
			result,
			market.Instrument{

				Symbol:item.Symbol,

				Base:item.BaseAsset,

				Quote:item.QuoteAsset,

				Type:"spot",

				Exchange:"binance",

			},
		)

	}


	return result,nil
}



func (c *Client) FetchTickers(
	ctx context.Context,
)([]market.Ticker,error){

	return []market.Ticker{},nil

}