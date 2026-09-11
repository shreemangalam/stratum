package jobs

import (
	"sync"
	"testing"
	"time"
)

func TestSubscribers_SubscribeAndSend(t *testing.T) {
	subs := NewSubscribers()
	ch := subs.Subscribe("job-1")

	subs.Send("job-1", Event{Type: "status", Data: "running"})

	select {
	case e := <-ch:
		if e.Type != "status" || e.Data != "running" {
			t.Errorf("unexpected event: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscribers_MultipleSubscribers(t *testing.T) {
	subs := NewSubscribers()
	ch1 := subs.Subscribe("job-1")
	ch2 := subs.Subscribe("job-1")

	subs.Send("job-1", Event{Type: "progress", Data: "parsing"})

	for _, ch := range []chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Type != "progress" {
				t.Errorf("unexpected event type: %s", e.Type)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
	}
}

func TestSubscribers_Unsubscribe(t *testing.T) {
	subs := NewSubscribers()
	ch := subs.Subscribe("job-1")

	subs.Unsubscribe("job-1", ch)

	if subs.Count() != 0 {
		t.Errorf("expected 0 subscribers, got %d", subs.Count())
	}
}

func TestSubscribers_Cleanup(t *testing.T) {
	subs := NewSubscribers()
	ch1 := subs.Subscribe("job-1")
	_ = subs.Subscribe("job-1")
	_ = subs.Subscribe("job-2")

	subs.Cleanup("job-1")

	if subs.Count() != 1 {
		t.Errorf("expected 1 subscriber after cleanup, got %d", subs.Count())
	}

	_, ok := <-ch1
	if ok {
		t.Error("expected ch1 to be closed after cleanup")
	}
}

func TestSubscribers_Count(t *testing.T) {
	subs := NewSubscribers()
	if subs.Count() != 0 {
		t.Errorf("expected 0, got %d", subs.Count())
	}

	subs.Subscribe("a")
	subs.Subscribe("a")
	subs.Subscribe("b")

	if subs.Count() != 3 {
		t.Errorf("expected 3, got %d", subs.Count())
	}
}

func TestSubscribers_SendToNoSubscribers(t *testing.T) {
	subs := NewSubscribers()
	subs.Send("nonexistent", Event{Type: "status", Data: "ok"})
}

func TestSubscribers_SendDropsWhenFull(t *testing.T) {
	subs := NewSubscribers()
	ch := subs.Subscribe("job-1")

	for i := range 20 {
		subs.Send("job-1", Event{Type: "progress", Data: string(rune('a' + i))})
	}

	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count > 16 {
		t.Errorf("channel buffer is 16, got %d events", count)
	}
}

func TestSubscribers_ConcurrentAccess(t *testing.T) {
	subs := NewSubscribers()
	var wg sync.WaitGroup

	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ch := subs.Subscribe("job-1")
			subs.Send("job-1", Event{Type: "status", Data: "running"})
			time.Sleep(time.Millisecond)
			subs.Unsubscribe("job-1", ch)
		}(i)
	}

	wg.Wait()
}
