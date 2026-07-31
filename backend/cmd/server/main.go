package main

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/vokix1/spread-terminal/backend/internal/exchanges/binance"
	"github.com/vokix1/spread-terminal/backend/internal/market"
	"github.com/vokix1/spread-terminal/backend/internal/services"
	"github.com/vokix1/spread-terminal/backend/internal/api"
	
)

func main() {

	ctx := context.Background()

	registry := market.NewRegistry()

	bus := market.NewBus()

	binanceClient := binance.New()

	go func(){

	err :=
	binanceClient.StreamTicker(bus)

	if err!=nil{
		panic(err)
	}

}()

	marketService := services.NewMarketService(
		registry,
		binanceClient,
	)

	// пока discovery пустой,
	// но архитектура уже готова
	err := marketService.Discover(ctx)

if err != nil {
	panic(err)
}


	router := gin.Default()

	api.MarketWS(
	router,
	bus,
)


	router.GET("/health", func(c *gin.Context) {

		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"service": "spread-terminal-backend",
		})

	})


	router.GET("/api/instruments", func(c *gin.Context) {

		c.JSON(
			http.StatusOK,
			registry.All(),
		)

	})


	router.Run(":8080")
}