package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

// StreamFast subscribes to Binance !ticker@arr stream and populates the fast
// cache layer. Blocks until ctx is cancelled or the connection drops.
// The Stream Manager retries the call.
func (c *Client) StreamFast(ctx context.Context, instruments []market.Instrument, ca *cache.Cache) error {
	// Build a set of symbols we care about for fast filtering.
	spotSet := make(map[string]bool)
	perpSet := make(map[string]bool)
	for _, inst := range instruments {
		switch inst.MarketType {
		case market.Spot:
			spotSet[inst.Symbol] = true
		case market.Perp:
			perpSet[inst.Symbol] = true
		}
	}

	errCh := make(chan error, 2)

	// Spot stream
	if len(spotSet) > 0 {
		go func() {
			errCh <- streamSession(
				ctx,
				"wss://stream.binance.com:9443/ws/!ticker@arr",
				"binance", market.Spot, spotSet, ca,
			)
		}()
	}

	// Perp stream
	if len(perpSet) > 0 {
		go func() {
			errCh <- streamSession(
				ctx,
				"wss://fstream.binance.com/ws/!ticker@arr",
				"binance", market.Perp, perpSet, ca,
			)
		}()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

type wsBookTicker struct {
	Symbol string `json:"s"`
	Bid    string `json:"b"`
	BidQty string `json:"B"`
	Ask    string `json:"a"`
	AskQty string `json:"A"`
}

func streamSession(
	ctx context.Context,
	url string,
	exchange string,
	mtype market.MarketType,
	symbolSet map[string]bool,
	ca *cache.Cache,
) error {
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial %s: %w (status %s)", url, err, resp.Status)
		}
		return fmt.Errorf("dial %s: %w", url, err)
	}
	defer conn.Close()

	log.Printf("binance: connected %s stream (%s)", mtype, url)

	conn.SetReadLimit(8 << 20)
	resetDeadline := func() { _ = conn.SetReadDeadline(time.Now().Add(90 * time.Second)) }
	resetDeadline()
	conn.SetPongHandler(func(string) error { resetDeadline(); return nil })

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var tickers []wsBookTicker
		if err := json.Unmarshal(msg, &tickers); err != nil {
			continue
		}

		now := time.Now()
		for _, t := range tickers {
			if !symbolSet[t.Symbol] {
				continue
			}
			bid, e1 := strconv.ParseFloat(t.Bid, 64)
			ask, e2 := strconv.ParseFloat(t.Ask, 64)
			bidQty, e3 := strconv.ParseFloat(t.BidQty, 64)
			askQty, e4 := strconv.ParseFloat(t.AskQty, 64)
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
				continue
			}
			if bid <= 0 || ask <= 0 {
				continue
			}
			key := market.InstrumentKey{
				Exchange:   exchange,
				Symbol:     t.Symbol,
				MarketType: mtype,
			}
			ca.SetFast(key, cache.FastData{
				Bid:       bid,
				Ask:       ask,
				BidQty:    bidQty,
				AskQty:    askQty,
				UpdatedAt: now,
			})
		}
	}
}
