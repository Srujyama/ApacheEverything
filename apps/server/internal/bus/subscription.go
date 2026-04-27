package bus

import (
	"sync/atomic"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// Subscription is a handle returned by Bus.Subscribe.
//
// Read records from C(). When done, call Close() — failing to do so leaks the
// subscription and the bus will keep trying to deliver to it.
type Subscription struct {
	id      int
	ch      chan sdk.Record
	Filter  func(sdk.Record) bool
	dropped atomic.Uint64
	bus     *Bus
}

// C returns the receive-only channel of records.
func (s *Subscription) C() <-chan sdk.Record { return s.ch }

// Dropped returns the number of records dropped because the subscriber
// couldn't keep up. Useful for observability.
func (s *Subscription) Dropped() uint64 { return s.dropped.Load() }

// Close removes the subscription from the bus and closes the channel.
// Safe to call multiple times.
func (s *Subscription) Close() {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	if _, ok := s.bus.subs[s.id]; !ok {
		return
	}
	delete(s.bus.subs, s.id)
	close(s.ch)
}
