package backend

import "testing"

func TestAvailableBackends_ListsBorgKopiaAndRestic(t *testing.T) {
	got := AvailableBackends()

	if len(got) != 3 || got[0] != "borg" || got[1] != "kopia" || got[2] != "restic" {
		t.Fatalf("expected [\"borg\", \"kopia\", \"restic\"], got %#v", got)
	}
}
