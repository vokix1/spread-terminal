package main

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/vokix1/spread-terminal/backend/internal/exchanges/binance"
	"github.com/vokix1/spread-terminal/backend/internal/market"
	"github.com/vokix1/spread-terminal/backend/internal/services"
)

func main() {

	ctx := context.Background()

	registry := market.NewRegistry()

	binanceClient := binance.New()

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