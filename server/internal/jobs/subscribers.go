package jobs

import (
	"sync"
)

// Event is an SSE event to send to subscribers.
type Event struct {
	Type string
	Data string
}

// Subscribers manages SSE event channels per job ID.
type Subscribers struct {
	mu   sync.RWMutex
	subs map[string][]chan Event
}

// NewSubscribers creates a new subscriber registry.
func NewSubscribers() *Subscribers {
	return &Subscribers{
		subs: make(map[string][]chan Event),
	}
}

// Subscribe returns a channel that receives events for the given job ID.
func (s *Subscribers) Subscribe(jobID string) chan Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan Event, 16)
	s.subs[jobID] = append(s.subs[jobID], ch)
	return ch
}

// Unsubscribe removes a channel from the given job ID's subscribers.
func (s *Subscribers) Unsubscribe(jobID string, ch chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	channels := s.subs[jobID]
	for i, c := range channels {
		if c == ch {
			channels[i] = channels[len(channels)-1]
			s.subs[jobID] = channels[:len(channels)-1]
			close(ch)
			return
		}
	}
}

// Count returns the number of active job subscriptions.
func (s *Subscribers) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, chs := range s.subs {
		count += len(chs)
	}
	return count
}

// Cleanup removes the subscriber list for a job, closing any remaining channels.
func (s *Subscribers) Cleanup(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs[jobID] {
		close(ch)
	}
	delete(s.subs, jobID)
}

// Send broadcasts an event to all subscribers of the given job ID.
func (s *Subscribers) Send(jobID string, event Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.subs[jobID] {
		select {
		case ch <- event:
		default:
		}
	}
}
