package binance

import (

	"encoding/json"

	"fmt"

	"github.com/gorilla/websocket"

	"github.com/vokix1/spread-terminal/backend/internal/market"

)



type bookTicker struct {

	Symbol string `json:"s"`

	Bid string `json:"b"`

	BidQty string `json:"B"`

	Ask string `json:"a"`

	AskQty string `json:"A"`

}



func (c *Client) StreamTicker(
	bus *market.Bus,
) error {


	url :=
	"wss://stream.binance.com:9443/ws/!bookTicker"



	conn,_,err :=
	websocket.DefaultDialer.Dial(
		url,nil,
	)


	if err!=nil{
		return err
	}


	defer conn.Close()



	for {


		_,msg,err :=
		conn.ReadMessage()


		if err!=nil{
			return err
		}


		var ticker bookTicker


		if err:=json.Unmarshal(msg,&ticker);err!=nil{

			continue

		}



		event:=market.MarketEvent{

			Exchange:"binance",

			Symbol:ticker.Symbol,

		}



		fmt.Sscanf(
			ticker.Bid,
			"%f",
			&event.Bid,
		)


		fmt.Sscanf(
			ticker.Ask,
			"%f",
			&event.Ask,
		)


		fmt.Sscanf(
			ticker.BidQty,
			"%f",
			&event.BidQty,
		)


		fmt.Sscanf(
			ticker.AskQty,
			"%f",
			&event.AskQty,
		)



		bus.Publish(event)

	}

}