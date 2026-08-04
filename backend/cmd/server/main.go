package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/vokix1/spread-terminal/backend/internal/api"
	"github.com/vokix1/spread-terminal/backend/internal/cache"
	"github.com/vokix1/spread-terminal/backend/internal/engine"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/binance"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/bitget"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/bingx"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/bybit"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/gate"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/kraken"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/kucoin"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/mexc"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/okx"
	"github.com/vokix1/spread-terminal/backend/internal/exchanges/phemex"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// ── Infrastructure ─────────────────────────────────────────────────────
	registry := market.NewRegistry()
	dataCache := cache.New()
	bus := market.NewBus()

	// ── Exchange connectors ─────────────────────────────────────────────────
	// Tier 1: full WS (Binance) + REST fallback (all others)
	binanceClient := binance.New()
	okxClient     := okx.New()
	bybitClient   := bybit.New()
	bitgetClient  := bitget.New()
	gateClient    := gate.New()
	kucoinClient  := kucoin.New()
	mexcClient    := mexc.New()
	bingxClient   := bingx.New()
	krakenClient  := kraken.New()
	phemexClient  := phemex.New()

	exchangeNames := []string{
		binanceClient.Name(), okxClient.Name(), bybitClient.Name(), bitgetClient.Name(),
		gateClient.Name(), kucoinClient.Name(), mexcClient.Name(), bingxClient.Name(),
		krakenClient.Name(), phemexClient.Name(),
	}
	log.Printf("loaded %d exchange connectors: %v", len(exchangeNames), exchangeNames)

	// ── Phase 1: Discovery (REST — instruments + slow data) ─────────────────
	discovery := engine.NewDiscoveryService(
		registry,
		dataCache,
		60*time.Second,
		binanceClient,
		okxClient,
		bybitClient,
		bitgetClient,
		gateClient,
		kucoinClient,
		mexcClient,
		bingxClient,
		krakenClient,
		phemexClient,
	)

	go func() {
		if err := discovery.BootstrapAndRun(ctx); err != nil && err != context.Canceled {
			log.Printf("discovery error: %v", err)
		}
	}()

	// Wait for instrument registry to be populated (max 60s)
	log.Println("waiting for initial instrument discovery...")
	{
		deadline := time.After(60 * time.Second)
		waiting := true
		for waiting {
			select {
			case <-ctx.Done():
				return
			case <-deadline:
				log.Println("discovery timeout — continuing with partial data")
				waiting = false
			case <-time.After(500 * time.Millisecond):
				if registry.Size() > 0 {
					waiting = false
				}
			}
		}
	}
	log.Printf("registry ready: %d instruments", registry.Size())

	// ── Phase 2: REST Poller ─────────────────────────────────────────────────
	scanner := engine.NewScanner(registry, dataCache)

	streamMgr := engine.NewAdaptiveStreamManager(
		dataCache,
		scanner,
		200,           // top 200 instruments per exchange for WS
		5*time.Minute,
		binanceClient,
		okxClient,
		bybitClient,
		bitgetClient,
		gateClient,
		kucoinClient,
		mexcClient,
		bingxClient,
		krakenClient,
		phemexClient,
	)

	restPoller := engine.NewRestPoller(
		dataCache,
		12*time.Second,
		binanceClient,
		okxClient,
		bybitClient,
		bitgetClient,
		gateClient,
		kucoinClient,
		mexcClient,
		bingxClient,
		krakenClient,
		phemexClient,
	)

	go restPoller.Run(ctx, registry, streamMgr.IsWSActive)

	// ── Phase 3: Adaptive WS ─────────────────────────────────────────────────
	go streamMgr.Run(ctx)

	// ── Scanner + Scan Loop ───────────────────────────────────────────────────
	scanLoop := engine.NewScanLoop(scanner, bus, 2*time.Second)
	go scanLoop.Run(ctx)

	// ── HTTP / WebSocket API ──────────────────────────────────────────────────
	router := gin.Default()
	api.MarketWS(router, bus)

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":      "ok",
			"instruments": registry.Size(),
			"exchanges":   exchangeNames,
		})
	})

	router.GET("/api/instruments", func(c *gin.Context) {
		c.JSON(http.StatusOK, registry.All())
	})

	router.GET("/api/scan", func(c *gin.Context) {
		c.JSON(http.StatusOK, scanner.Scan())
	})

	addr := ":8080"
	srv := &http.Server{Addr: addr, Handler: router}

	go func() {
		<-ctx.Done()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("spread-terminal backend listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
