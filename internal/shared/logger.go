package shared

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

// Logger records a leveled message.
type Logger interface {
	Log(level string, message string)
}

// FileLogger is a Logger that prints each message to stdout and appends a
// timestamped line to LogFile.
type FileLogger struct {
	LogFile string
	FS      FileSystem
}

func (logger FileLogger) Log(level string, message string) {
	fs := logger.FS
	if fs == nil {
		fs = OSFileSystem{}
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] [%s] %s", timestamp, level, message)

	fmt.Println(line)

	logFile, err := fs.OpenFile(logger.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write log file: %v\n", err)
		return
	}
	defer logFile.Close()

	writer := bufio.NewWriter(logFile)
	if _, err := writer.WriteString(line + "\n"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write log file: %v\n", err)
		return
	}

	if err := writer.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write log file: %v\n", err)
	}
}
