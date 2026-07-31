package services


import (

	"context"

	"github.com/vokix1/spread-terminal/backend/internal/exchanges"

	"github.com/vokix1/spread-terminal/backend/internal/market"

)



type MarketService struct {

	exchanges []exchanges.Exchange

	Registry *market.Registry

}



func NewMarketService(
	registry *market.Registry,
	exchanges ...exchanges.Exchange,
)*MarketService{

	return &MarketService{

		Registry:registry,

		exchanges:exchanges,

	}

}



func (s *MarketService) Discover(
	ctx context.Context,
)error{


	for _,exchange:=range s.exchanges{


		items,err:=exchange.FetchInstruments(ctx)

		if err!=nil{

			return err

		}


		for _,item:=range items{

			s.Registry.Add(item)

		}

	}


	return nil
}