package engine

import (
	"context"
	"log"
	"time"

	"github.com/vokix1/spread-terminal/backend/internal/market"
)

// ScanLoop runs the Scanner at a fixed interval and publishes results to the Bus.
type ScanLoop struct {
	scanner  *Scanner
	bus      *market.Bus
	interval time.Duration
}

func NewScanLoop(scanner *Scanner, bus *market.Bus, interval time.Duration) *ScanLoop {
	return &ScanLoop{
		scanner:  scanner,
		bus:      bus,
		interval: interval,
	}
}

// Run blocks until ctx is cancelled. It scans on startup and then at interval.
func (sl *ScanLoop) Run(ctx context.Context) {
	sl.tick(ctx)
	ticker := time.NewTicker(sl.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sl.tick(ctx)
		}
	}
}

func (sl *ScanLoop) tick(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	opps := sl.scanner.Scan()
	if len(opps) == 0 {
		return
	}
	log.Printf("scan_loop: found %d opportunities (top score: %.1f)", len(opps), opps[0].Score)
	sl.bus.PublishAll(opps)
}
