package store

import (
	"reflect"
	"sync"
	"testing"
)

func TestRingEvictsOldestWhenFull(t *testing.T) {
	r := NewRing[int](3)
	for i := 1; i <= 4; i++ {
		r.Push(i)
	}
	if r.Len() != 3 {
		t.Fatalf("len = %d, want 3", r.Len())
	}
	got := r.Slice()
	want := []int{2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("slice = %v, want %v", got, want)
	}
}

func TestRingSliceOrderedOldestFirst(t *testing.T) {
	r := NewRing[string](4)
	for _, s := range []string{"a", "b", "c"} {
		r.Push(s)
	}
	got := r.Slice()
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("slice = %v, want %v", got, want)
	}
}

func TestRingConcurrentPush(t *testing.T) {
	r := NewRing[int](1024)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				r.Push(j)
			}
		}()
	}
	wg.Wait()
	if r.Len() != 1024 {
		t.Errorf("len = %d, want 1024", r.Len())
	}
}
