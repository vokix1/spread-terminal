package exchanges


import (
	"context"

	"github.com/vokix1/spread-terminal/backend/internal/market"
)


type Exchange interface {

	Name() string


	FetchInstruments(
		ctx context.Context,
	) ([]market.Instrument,error)


	FetchTickers(
		ctx context.Context,
	) ([]market.Ticker,error)

}