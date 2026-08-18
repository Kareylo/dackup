package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileLogger_Log_WritesTimestampedLineToLogFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "dackup.log")
	logger := FileLogger{LogFile: logFile}

	logger.Log("INFO", "backup started")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	logged := string(data)
	if !strings.Contains(logged, "[INFO] backup started") {
		t.Fatalf("expected log file to contain the message, got %q", logged)
	}
}

func TestFileLogger_Log_AppendsAcrossMultipleCalls(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "dackup.log")
	logger := FileLogger{LogFile: logFile}

	logger.Log("INFO", "first")
	logger.Log("WARN", "second")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	logged := string(data)
	if !strings.Contains(logged, "first") || !strings.Contains(logged, "second") {
		t.Fatalf("expected log file to contain both messages, got %q", logged)
	}
}

func TestFileLogger_Log_InvalidLogFilePathDoesNotPanic(t *testing.T) {
	logger := FileLogger{LogFile: filepath.Join(t.TempDir(), "does-not-exist", "sub", "dackup.log")}

	logger.Log("ERROR", "should not panic even though the directory does not exist")
}

func TestFileLogger_Log_WriteFailureDoesNotPanic(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "dackup.log")
	logger := FileLogger{LogFile: logFile, FS: fakeSecretFileSystem{closeOpenedFile: true}}

	logger.Log("ERROR", "should not panic when the log file can't be written to")
}
