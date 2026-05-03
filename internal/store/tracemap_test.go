package store

import (
	"testing"
	"time"

	"github.com/rifat977/taplytix/internal/model"
)

func TestTraceMapAssemblesTree(t *testing.T) {
	tm := NewTraceMap()
	now := time.Now()
	root := model.SpanEvent{
		TraceID: "trace-1", SpanID: "root", ParentID: "",
		Name: "GET /api", StartTime: now, Duration: 100 * time.Millisecond,
	}
	child := model.SpanEvent{
		TraceID: "trace-1", SpanID: "child", ParentID: "root",
		Name: "db.query", StartTime: now.Add(10 * time.Millisecond), Duration: 40 * time.Millisecond,
	}
	tm.Add(root)
	tm.Add(child)

	tr, ok := tm.Get("trace-1")
	if !ok {
		t.Fatal("trace-1 not found")
	}
	if tr.Root == nil || tr.Root.SpanID != "root" {
		t.Fatalf("root not set: %+v", tr.Root)
	}
	if got := tr.Children["root"]; len(got) != 1 || got[0].SpanID != "child" {
		t.Fatalf("children[root] = %+v", got)
	}
}

func TestTraceMapRecentOrdersByStart(t *testing.T) {
	tm := NewTraceMap()
	base := time.Now()
	for i, id := range []string{"a", "b", "c"} {
		tm.Add(model.SpanEvent{
			TraceID: id, SpanID: "r", StartTime: base.Add(time.Duration(i) * time.Second),
			Duration: time.Millisecond,
		})
	}
	rec := tm.Recent(2)
	if len(rec) != 2 || rec[0].TraceID != "c" || rec[1].TraceID != "b" {
		t.Errorf("recent(2) = %+v", rec)
	}
}

func TestTraceMapEvict(t *testing.T) {
	tm := NewTraceMap()
	old := time.Now().Add(-time.Hour)
	tm.Add(model.SpanEvent{TraceID: "old", SpanID: "r", StartTime: old, Duration: time.Second})
	tm.Add(model.SpanEvent{TraceID: "new", SpanID: "r", StartTime: time.Now(), Duration: time.Second})
	if removed := tm.Evict(time.Minute); removed != 1 {
		t.Errorf("evict removed %d, want 1", removed)
	}
	if _, ok := tm.Get("old"); ok {
		t.Errorf("old trace not evicted")
	}
	if _, ok := tm.Get("new"); !ok {
		t.Errorf("new trace was evicted")
	}
}
