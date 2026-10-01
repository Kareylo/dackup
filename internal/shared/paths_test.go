package shared

import "testing"

func TestValidateConfiguredPath_AcceptsPathsInsideRoot(t *testing.T) {
	for _, path := range []string{"/app", "app", "app/data", "/app/../data", "app/./data", "/opt/app/"} {
		if err := ValidateConfiguredPath(path); err != nil {
			t.Errorf("ValidateConfiguredPath(%q) returned error %v, want nil", path, err)
		}
	}
}

// Root-only paths clean to "" and are already skipped (with a WARN) by
// TransferService, so they're not rejected here.
func TestValidateConfiguredPath_AcceptsPathsTransferServiceSkips(t *testing.T) {
	for _, path := range []string{"/", "/../etc/.."} {
		if err := ValidateConfiguredPath(path); err != nil {
			t.Errorf("ValidateConfiguredPath(%q) returned error %v, want nil", path, err)
		}
	}
}

func TestValidateConfiguredPath_RejectsPathsEscapingRoot(t *testing.T) {
	for _, path := range []string{"..", "../etc", "../../etc", "app/../../etc", "./../etc"} {
		if err := ValidateConfiguredPath(path); err == nil {
			t.Errorf("ValidateConfiguredPath(%q) returned nil, want an error", path)
		}
	}
}

func TestValidateConfiguredPath_RejectsWholeRoot(t *testing.T) {
	for _, path := range []string{"", ".", "./", "app/.."} {
		if err := ValidateConfiguredPath(path); err == nil {
			t.Errorf("ValidateConfiguredPath(%q) returned nil, want an error", path)
		}
	}
}
