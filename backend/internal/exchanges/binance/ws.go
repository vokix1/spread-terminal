package binance

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

const bookTickerURL = "wss://stream.binance.com:9443/ws/!bookTicker"

type bookTicker struct {
	Symbol string `json:"s"`
	Bid    string `json:"b"`
	BidQty string `json:"B"`
	Ask    string `json:"a"`
	AskQty string `json:"A"`
}

func (c *Client) StreamTicker(bus *market.Bus) error {
	backoff := time.Second
	for {
		if err := c.streamTickerSession(bus); err != nil {
			log.Printf("binance websocket disconnected: %v; reconnecting in %s", err, backoff)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func (c *Client) streamTickerSession(bus *market.Bus) error {
	conn, response, err := websocket.DefaultDialer.Dial(bookTickerURL, nil)
	if err != nil {
		if response != nil {
			return fmt.Errorf("dial Binance websocket: %w (status %s)", err, response.Status)
		}
		return fmt.Errorf("dial Binance websocket: %w", err)
	}
	defer conn.Close()

	log.Println("Binance book ticker connected")
	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var ticker bookTicker
		if err := json.Unmarshal(message, &ticker); err != nil {
			continue
		}

		bid, errBid := strconv.ParseFloat(ticker.Bid, 64)
		ask, errAsk := strconv.ParseFloat(ticker.Ask, 64)
		bidQty, errBidQty := strconv.ParseFloat(ticker.BidQty, 64)
		askQty, errAskQty := strconv.ParseFloat(ticker.AskQty, 64)
		if errBid != nil || errAsk != nil || errBidQty != nil || errAskQty != nil || bid <= 0 || ask <= 0 {
			continue
		}

		bus.Publish(market.MarketEvent{
			Exchange: "binance",
			Symbol:   ticker.Symbol,
			Bid:      bid,
			Ask:      ask,
			BidQty:   bidQty,
			AskQty:   askQty,
			Time:     time.Now().UTC(),
		})
	}
}
