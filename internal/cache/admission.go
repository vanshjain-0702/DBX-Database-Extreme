package cache

import "sync"

// AdmissionGate implements a TinyLFU-inspired admission filter.
// It decides whether a new entry should be admitted to cache,
// by comparing its estimated frequency to the victim's frequency.
type AdmissionGate struct {
	mu      sync.Mutex
	counter map[string]int
	window  int
	total   int
}

// NewAdmissionGate creates a new admission gate.
func NewAdmissionGate(window int) *AdmissionGate {
	return &AdmissionGate{
		counter: make(map[string]int),
		window:  window,
	}
}

// Admit returns true if the incoming key should be admitted over victim.
func (a *AdmissionGate) Admit(incoming, victim string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	inFreq := a.counter[incoming]
	victimFreq := a.counter[victim]
	return inFreq >= victimFreq
}

// Increment records an access for key.
func (a *AdmissionGate) Increment(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.counter[key]++
	a.total++
	// Decay counts when window is exceeded (aging)
	if a.total >= a.window {
		for k, v := range a.counter {
			a.counter[k] = v / 2
			if a.counter[k] == 0 {
				delete(a.counter, k)
			}
		}
		a.total = 0
	}
}

// HotColdTracker tracks hot/cold status of keys.
type HotColdTracker struct {
	mu      sync.RWMutex
	hotKeys map[string]int // key -> access count
	hotN    int            // top N keys are "hot"
}

// NewHotColdTracker creates a hot/cold tracker.
func NewHotColdTracker(hotN int) *HotColdTracker {
	return &HotColdTracker{
		hotKeys: make(map[string]int),
		hotN:    hotN,
	}
}

// Access records an access for key.
func (h *HotColdTracker) Access(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hotKeys[key]++
}

// IsHot returns true if key is in the top-N accessed keys.
func (h *HotColdTracker) IsHot(key string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	threshold := h.hotThreshold()
	return h.hotKeys[key] >= threshold
}

func (h *HotColdTracker) hotThreshold() int {
	if len(h.hotKeys) == 0 {
		return 1
	}
	counts := make([]int, 0, len(h.hotKeys))
	for _, v := range h.hotKeys {
		counts = append(counts, v)
	}
	// Find the hotN-th largest count
	if h.hotN >= len(counts) {
		return 1
	}
	// Simple partial sort for threshold
	topN := counts
	if len(topN) > h.hotN {
		// find threshold via nth element (approx)
		sum := 0
		for _, v := range topN {
			sum += v
		}
		return sum / len(topN)
	}
	return 1
}
