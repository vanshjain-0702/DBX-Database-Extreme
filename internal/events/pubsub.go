// Package events provides pub/sub, durable streams, and consumer groups.
package events

import (
	"sync"
)

// Subscriber is a channel-based subscription.
type Subscriber struct {
	ID      uint64
	Channel string
	Ch      chan []byte
}

// PubSub is an in-memory publish/subscribe broker.
type PubSub struct {
	mu          sync.RWMutex
	channels    map[string][]*Subscriber
	subsByID    map[uint64]*Subscriber
	nextSubID   uint64
	maxChannels int
	maxSubs     int
}

// NewPubSub creates a pub/sub broker.
func NewPubSub(maxChannels, maxSubs int) *PubSub {
	return &PubSub{
		channels:    make(map[string][]*Subscriber),
		subsByID:    make(map[uint64]*Subscriber),
		maxChannels: maxChannels,
		maxSubs:     maxSubs,
	}
}

// Subscribe adds a subscriber to channel. Returns subscriber ID.
func (p *PubSub) Subscribe(clientID uint64, channels []string) []*Subscriber {
	p.mu.Lock()
	defer p.mu.Unlock()
	var subs []*Subscriber
	for _, ch := range channels {
		p.nextSubID++
		sub := &Subscriber{
			ID:      p.nextSubID,
			Channel: ch,
			Ch:      make(chan []byte, 256),
		}
		p.channels[ch] = append(p.channels[ch], sub)
		p.subsByID[sub.ID] = sub
		subs = append(subs, sub)
	}
	return subs
}

// Unsubscribe removes a subscriber by ID.
func (p *PubSub) Unsubscribe(subID uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sub, ok := p.subsByID[subID]
	if !ok {
		return
	}
	delete(p.subsByID, subID)
	subs := p.channels[sub.Channel]
	var filtered []*Subscriber
	for _, s := range subs {
		if s.ID != subID {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		delete(p.channels, sub.Channel)
	} else {
		p.channels[sub.Channel] = filtered
	}
	close(sub.Ch)
}

// Publish sends message to all subscribers of channel. Returns count.
func (p *PubSub) Publish(channel string, message []byte) int {
	p.mu.RLock()
	subs := p.channels[channel]
	p.mu.RUnlock()
	count := 0
	for _, sub := range subs {
		select {
		case sub.Ch <- message:
			count++
		default:
			// Drop message if subscriber is slow
		}
	}
	return count
}

// ChannelCount returns the number of active channels.
func (p *PubSub) ChannelCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.channels)
}

// SubscriberCount returns the number of subscribers on channel.
func (p *PubSub) SubscriberCount(channel string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.channels[channel])
}

// Channels returns all active channel names.
func (p *PubSub) Channels() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	names := make([]string, 0, len(p.channels))
	for ch := range p.channels {
		names = append(names, ch)
	}
	return names
}
