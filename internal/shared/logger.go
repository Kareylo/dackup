package shared

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

// LogFileMode is the permission set for every log file dackup writes. Log
// files receive the full stdout/stderr of rsync, docker and the backup
// backends, so they're readable by their owner only.
const LogFileMode os.FileMode = 0o600

// openLogFile opens path for appending, creating it with LogFileMode. An
// existing file that's still readable or writable by group/other (e.g.
// one created by an older dackup with 0644) is tightened to LogFileMode;
// if that fails (e.g. the file belongs to another user), a warning is
// printed to stderr and logging continues.
func openLogFile(fs FileSystem, path string) (*os.File, error) {
	file, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, LogFileMode)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err == nil && info.Mode().Perm()&^LogFileMode != 0 {
		if err := file.Chmod(LogFileMode); err != nil {
			fmt.Fprintf(os.Stderr, "warning: log file %s is accessible by other users (mode %#o) and could not be restricted to %#o: %v\n", path, info.Mode().Perm(), LogFileMode, err)
		}
	}

	return file, nil
}

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

	logFile, err := openLogFile(fs, logger.LogFile)
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
