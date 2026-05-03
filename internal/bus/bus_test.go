package bus

import (
	"testing"
	"time"
)

func TestBusFanOut(t *testing.T) {
	b := New()
	a := b.Subscribe()
	c := b.Subscribe()
	b.Publish("hello")
	for _, ch := range []<-chan any{a, c} {
		select {
		case got := <-ch:
			if got != "hello" {
				t.Errorf("got %v, want hello", got)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive event")
		}
	}
}

func TestBusDropsWhenSubscriberFull(t *testing.T) {
	b := &Bus{bufSize: 1}
	ch := b.Subscribe()
	b.Publish("a")
	b.Publish("b") // dropped — buffer full, must not block
	b.Publish("c") // also dropped
	if got := <-ch; got != "a" {
		t.Errorf("first event = %v, want a", got)
	}
	select {
	case extra := <-ch:
		t.Errorf("expected no further events, got %v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBusDispatchNoProgramIsNoop(t *testing.T) {
	b := New()
	b.Dispatch("anything") // must not panic when no program is set
}
