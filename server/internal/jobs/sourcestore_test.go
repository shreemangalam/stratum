package jobs

import (
	"sync"
	"testing"
)

func TestSourceStore_PutAndGet(t *testing.T) {
	s := NewSourceStore()
	s.Put("abc123", []byte("hello world"))

	got, ok := s.Get("abc123")
	if !ok {
		t.Fatal("expected source to exist")
	}
	if string(got) != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", string(got))
	}
}

func TestSourceStore_GetMissing(t *testing.T) {
	s := NewSourceStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("expected missing source to return false")
	}
}

func TestSourceStore_Delete(t *testing.T) {
	s := NewSourceStore()
	s.Put("abc", []byte("data"))
	s.Delete("abc")

	_, ok := s.Get("abc")
	if ok {
		t.Error("expected source to be deleted")
	}
	if s.Size() != 0 {
		t.Errorf("expected size 0, got %d", s.Size())
	}
}

func TestSourceStore_DeleteNonexistent(t *testing.T) {
	s := NewSourceStore()
	s.Delete("nonexistent")
}

func TestSourceStore_Size(t *testing.T) {
	s := NewSourceStore()
	if s.Size() != 0 {
		t.Errorf("expected 0, got %d", s.Size())
	}

	s.Put("a", []byte("1"))
	s.Put("b", []byte("2"))
	if s.Size() != 2 {
		t.Errorf("expected 2, got %d", s.Size())
	}
}

func TestSourceStore_Overwrite(t *testing.T) {
	s := NewSourceStore()
	s.Put("key", []byte("first"))
	s.Put("key", []byte("second"))

	got, ok := s.Get("key")
	if !ok {
		t.Fatal("expected source to exist")
	}
	if string(got) != "second" {
		t.Errorf("expected %q, got %q", "second", string(got))
	}
	if s.Size() != 1 {
		t.Errorf("expected size 1, got %d", s.Size())
	}
}

func TestSourceStore_ConcurrentAccess(t *testing.T) {
	s := NewSourceStore()
	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + (i % 26)))
			s.Put(key, []byte("data"))
			s.Get(key)
			s.Delete(key)
		}(i)
	}

	wg.Wait()
}
