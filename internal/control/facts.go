package control

import (
	"maps"
	"slices"
	"sync"
)

// facts holds the latest value of each published fact and pushes changes to
// subscribers. Publishing never blocks: each subscriber has its own goroutine
// and a pending map, so a slow reader gets the latest value of each fact and
// skips the ones in between.
type facts struct {
	mu     sync.Mutex
	values map[string]any
	subs   map[ConnID]*subscriber
}

type subscriber struct {
	conn    *Conn
	mu      sync.Mutex
	pending map[string]any
	wake    chan struct{}
	done    chan struct{}
}

func newFacts() *facts {
	return &facts{values: map[string]any{}, subs: map[ConnID]*subscriber{}}
}

// publish records a fact; an unchanged value is not pushed again. Values are
// scalars only.
func (f *facts) publish(name string, value any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if old, ok := f.values[name]; ok && old == value {
		return
	}
	f.values[name] = value
	for _, s := range f.subs {
		s.offer(map[string]any{name: value})
	}
}

func (f *facts) snapshot() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return maps.Clone(f.values)
}

// subscribe starts pushing to c, beginning with every current value.
func (f *facts) subscribe(id ConnID, c *Conn) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.subs[id]; ok {
		return
	}
	s := &subscriber{conn: c, pending: map[string]any{}, wake: make(chan struct{}, 1),
		done: make(chan struct{})}
	f.subs[id] = s
	s.offer(maps.Clone(f.values))
	go s.run()
}

func (f *facts) unsubscribe(id ConnID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.subs[id]; ok {
		delete(f.subs, id)
		close(s.done)
	}
}

func (s *subscriber) offer(values map[string]any) {
	s.mu.Lock()
	maps.Copy(s.pending, values)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *subscriber) run() {
	for {
		select {
		case <-s.done:
			return
		case <-s.wake:
		}
		s.mu.Lock()
		batch := s.pending
		s.pending = map[string]any{}
		s.mu.Unlock()
		for _, name := range slices.Sorted(maps.Keys(batch)) {
			if s.conn.Send(Fact{Fact: name, Value: batch[name]}) != nil {
				return
			}
		}
	}
}
