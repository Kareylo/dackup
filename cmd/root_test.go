package cmd

import "testing"

func TestRootCommandHasExpectedSubcommands(t *testing.T) {
	expectedCommands := []string{
		"backup",
		"restore",
		"config",
	}

	for _, expectedCommand := range expectedCommands {
		t.Run(expectedCommand, func(t *testing.T) {
			cmd, _, err := rootCmd.Find([]string{expectedCommand})
			if err != nil {
				t.Fatalf("rootCmd.Find returned error: %v", err)
			}

			if cmd == nil {
				t.Fatalf("expected command %q to exist", expectedCommand)
			}

			if cmd.Name() != expectedCommand {
				t.Fatalf("expected command %q, got %q", expectedCommand, cmd.Name())
			}
		})
	}
}

func TestConfigCommandHasExpectedSubcommands(t *testing.T) {
	expectedCommands := []string{
		"init",
		"add",
		"update",
		"use-file",
	}

	for _, expectedCommand := range expectedCommands {
		t.Run(expectedCommand, func(t *testing.T) {
			cmd, _, err := configCmd.Find([]string{expectedCommand})
			if err != nil {
				t.Fatalf("configCmd.Find returned error: %v", err)
			}

			if cmd == nil {
				t.Fatalf("expected config command %q to exist", expectedCommand)
			}

			if cmd.Name() != expectedCommand {
				t.Fatalf("expected config command %q, got %q", expectedCommand, cmd.Name())
			}
		})
	}
}

func TestBackupCommandAcceptsArbitraryArgs(t *testing.T) {
	if err := backupCmd.Args(backupCmd, []string{"paperless", "adguard"}); err != nil {
		t.Fatalf("backup command should accept arbitrary args, got error: %v", err)
	}
}

func TestRestoreCommandAcceptsArbitraryArgs(t *testing.T) {
	if err := restoreCmd.Args(restoreCmd, []string{"paperless", "adguard"}); err != nil {
		t.Fatalf("restore command should accept arbitrary args, got error: %v", err)
	}
}

func TestConfigUseFileRequiresOneArg(t *testing.T) {
	if err := configUseFileCmd.Args(configUseFileCmd, nil); err == nil {
		t.Fatal("expected error with no args")
	}

	if err := configUseFileCmd.Args(configUseFileCmd, []string{"/tmp/config.json"}); err != nil {
		t.Fatalf("expected one arg to be valid, got error: %v", err)
	}

	if err := configUseFileCmd.Args(configUseFileCmd, []string{"/tmp/a.json", "/tmp/b.json"}); err == nil {
		t.Fatal("expected error with two args")
	}
}
