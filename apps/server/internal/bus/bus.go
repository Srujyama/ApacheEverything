// Package bus is the in-process record bus.
//
// Every connector instance Publishes records to the bus. Subscribers (the
// WebSocket stream endpoint, and starting in phase 2 the storage writer)
// receive them. The bus also keeps a small ring buffer of recent records so
// new subscribers see context immediately on connect.
package bus

import (
	"context"
	"sync"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// Bus is a fan-out pub/sub of sdk.Records. Safe for concurrent use.
type Bus struct {
	mu          sync.RWMutex
	subs        map[int]*Subscription
	nextID      int
	ring        []sdk.Record // most recent records, newest last
	ringSize    int
	subBuffer   int
}

// New creates a Bus that retains the last `ringSize` records and gives each
// subscriber a buffered channel of `subBuffer` records before dropping.
func New(ringSize, subBuffer int) *Bus {
	if ringSize < 0 {
		ringSize = 0
	}
	if subBuffer <= 0 {
		subBuffer = 64
	}
	return &Bus{
		subs:      make(map[int]*Subscription),
		ring:      make([]sdk.Record, 0, ringSize),
		ringSize:  ringSize,
		subBuffer: subBuffer,
	}
}

// Publish delivers r to every active subscription whose Filter accepts it,
// and appends it to the ring buffer. Slow subscribers drop records rather
// than block the publisher.
func (b *Bus) Publish(_ context.Context, r sdk.Record) {
	b.mu.Lock()
	if b.ringSize > 0 {
		if len(b.ring) >= b.ringSize {
			b.ring = b.ring[1:]
		}
		b.ring = append(b.ring, r)
	}
	subs := make([]*Subscription, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.Unlock()

	for _, s := range subs {
		if s.Filter != nil && !s.Filter(r) {
			continue
		}
		select {
		case s.ch <- r:
		default:
			// Subscriber's buffer is full — drop. WebSocket clients that
			// can't keep up will see gaps rather than backpressure the bus.
			s.dropped.Add(1)
		}
	}
}

// Subscribe registers a new subscription. Call sub.Close() when done.
//
// If replayRing is true, the subscriber's channel is pre-populated with the
// current ring buffer (filtered) before any new records are delivered.
func (b *Bus) Subscribe(filter func(sdk.Record) bool, replayRing bool) *Subscription {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	sub := &Subscription{
		id:     id,
		ch:     make(chan sdk.Record, b.subBuffer),
		Filter: filter,
		bus:    b,
	}
	b.subs[id] = sub

	if replayRing {
		for _, r := range b.ring {
			if filter != nil && !filter(r) {
				continue
			}
			select {
			case sub.ch <- r:
			default:
				// Ring has more matches than the buffer holds — skip the rest.
			}
		}
	}
	b.mu.Unlock()
	return sub
}

// Recent returns a snapshot of the ring buffer (newest last).
func (b *Bus) Recent() []sdk.Record {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]sdk.Record, len(b.ring))
	copy(out, b.ring)
	return out
}
