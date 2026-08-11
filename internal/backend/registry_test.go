package backend

import "testing"

func TestAvailableBackends_EmptyUntilBackendsAreImplemented(t *testing.T) {
	if got := AvailableBackends(); len(got) != 0 {
		t.Fatalf("expected no registered backends yet, got %#v", got)
	}
}
