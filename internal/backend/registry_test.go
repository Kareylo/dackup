package backend

import "testing"

func TestAvailableBackends_ListsBorg(t *testing.T) {
	got := AvailableBackends()

	if len(got) != 1 || got[0] != "borg" {
		t.Fatalf("expected [\"borg\"], got %#v", got)
	}
}
