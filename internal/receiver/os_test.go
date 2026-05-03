package receiver

import "testing"

func TestSplitProcStatPreservesParenComm(t *testing.T) {
	in := "1234 (my proc) S 1 1234 1234 0 -1 4194304 0 0 0 0 100 200 0 0 20 0 1 0 0 0 0 0"
	got := splitProcStat(in)
	if got[0] != "1234" {
		t.Errorf("pid field = %q, want 1234", got[0])
	}
	if got[1] != "my proc" {
		t.Errorf("comm field = %q, want %q", got[1], "my proc")
	}
	if got[13] != "100" {
		t.Errorf("utime (field 14) = %q, want 100", got[13])
	}
}
