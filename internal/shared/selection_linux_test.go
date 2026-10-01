package shared

import (
	"os"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// openPseudoTerminal opens a pty pair and returns its slave end, which
// behaves like a real terminal for term.IsTerminal/MakeRaw.
func openPseudoTerminal(t *testing.T) *os.File {
	t.Helper()

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("pseudo-terminals unavailable: %v", err)
	}
	t.Cleanup(func() { master.Close() })

	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatalf("failed to unlock pty: %v", err)
	}

	number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatalf("failed to get pty number: %v", err)
	}

	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(number), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("failed to open pty slave: %v", err)
	}
	t.Cleanup(func() { slave.Close() })

	return slave
}

func TestStdinTerminal_MakeRawAndRestoreOnRealTerminal(t *testing.T) {
	pty := openPseudoTerminal(t)
	withStdio(t, pty, pty)

	terminal := StdinTerminal()
	if terminal == nil {
		t.Fatal("expected a terminal when stdin and stdout are a pty, got nil")
	}

	before, err := term.GetState(int(pty.Fd()))
	if err != nil {
		t.Fatalf("failed to read pty state: %v", err)
	}

	restore, err := terminal.MakeRaw()
	if err != nil {
		t.Fatalf("MakeRaw returned error: %v", err)
	}

	if err := restore(); err != nil {
		t.Fatalf("restore returned error: %v", err)
	}

	after, err := term.GetState(int(pty.Fd()))
	if err != nil {
		t.Fatalf("failed to read pty state: %v", err)
	}

	if *before != *after {
		t.Fatal("expected restore to bring back the original terminal state")
	}
}
