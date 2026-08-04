package market

import "sync"

// Registry stores all known instruments, indexed by base asset and by key.
// Thread-safe for concurrent reads/writes.
type Registry struct {
	mu          sync.RWMutex
	byKey       map[InstrumentKey]Instrument          // exact lookup
	byBase      map[string][]InstrumentKey            // base asset → all markets
}

func NewRegistry() *Registry {
	return &Registry{
		byKey:  make(map[InstrumentKey]Instrument),
		byBase: make(map[string][]InstrumentKey),
	}
}

// Add registers an instrument. Idempotent.
func (r *Registry) Add(inst Instrument) {
	key := InstrumentKey{
		Exchange:   inst.Exchange,
		Symbol:     inst.Symbol,
		MarketType: inst.MarketType,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byKey[key]; exists {
		return
	}
	r.byKey[key] = inst
	r.byBase[inst.Base] = append(r.byBase[inst.Base], key)
}

// Get returns a single instrument by key.
func (r *Registry) Get(key InstrumentKey) (Instrument, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.byKey[key]
	return inst, ok
}

// MarketsFor returns all instruments for a given base asset.
func (r *Registry) MarketsFor(base string) []Instrument {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := r.byBase[base]
	result := make([]Instrument, 0, len(keys))
	for _, k := range keys {
		result = append(result, r.byKey[k])
	}
	return result
}

// Bases returns all unique base assets.
func (r *Registry) Bases() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bases := make([]string, 0, len(r.byBase))
	for b := range r.byBase {
		bases = append(bases, b)
	}
	return bases
}

// All returns every instrument.
func (r *Registry) All() []Instrument {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Instrument, 0, len(r.byKey))
	for _, v := range r.byKey {
		result = append(result, v)
	}
	return result
}

// Size returns the total number of instruments.
func (r *Registry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byKey)
}
