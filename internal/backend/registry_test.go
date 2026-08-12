package backend

import "testing"

func TestAvailableBackends_ListsBorgAndKopia(t *testing.T) {
	got := AvailableBackends()

	if len(got) != 2 || got[0] != "borg" || got[1] != "kopia" {
		t.Fatalf("expected [\"borg\", \"kopia\"], got %#v", got)
	}
}
