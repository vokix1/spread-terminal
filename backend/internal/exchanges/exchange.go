package exchanges

import (
	"context"

	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

// Exchange is the interface every connector must implement.
type Exchange interface {
	// Name returns the exchange identifier, e.g. "binance".
	Name() string

	// FetchInstruments returns all tradable instruments from this exchange
	// (spot + perp + futures where available).
	FetchInstruments(ctx context.Context) ([]market.Instrument, error)

	// FetchSlowData fetches REST-sourced data (volume, funding, OI) and
	// populates the slow cache layer for the given instruments.
	FetchSlowData(ctx context.Context, instruments []market.Instrument, c *cache.Cache) error

	// FetchFastData fetches bid/ask via REST for the given instruments and
	// writes them into the fast cache layer. Used by RestPoller as a fallback
	// when no WebSocket implementation is available, and also for the initial
	// scan before WS connections are established.
	FetchFastData(ctx context.Context, instruments []market.Instrument, c *cache.Cache) error

	// StreamFast opens a WebSocket stream and writes bid/ask into the fast
	// cache layer. Blocks until the stream ends; the caller retries.
	// Only instruments in the provided set are processed.
	// Returns ErrWSNotImplemented if WS is not available for this exchange.
	StreamFast(ctx context.Context, instruments []market.Instrument, c *cache.Cache) error
}

// ErrWSNotImplemented should be returned by StreamFast when the exchange
// does not yet have a WS implementation. RestPoller uses this to decide
// whether to keep polling REST instead.
type ErrWSNotImplemented struct{ Exchange string }

func (e ErrWSNotImplemented) Error() string {
	return e.Exchange + ": WebSocket not yet implemented, use REST polling"
}
