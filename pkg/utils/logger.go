package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type logEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Caller    string `json:"caller,omitempty"`
}

func writeLog(w *os.File, level, msg string) {
	caller := ""
	if _, file, line, ok := runtime.Caller(2); ok {
		parts := strings.Split(file, "/")
		short := file
		if len(parts) > 0 {
			short = parts[len(parts)-1]
		}
		caller = fmt.Sprintf("%s:%d", short, line)
	}

	entry := logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   msg,
		Caller:    caller,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(w, `{"level":"error","message":"log marshal failed: %v"}`+"\n", err)
		return
	}
	w.Write(append(data, '\n'))
}

func LogInfo(format string, args ...interface{}) {
	writeLog(os.Stdout, "info", fmt.Sprintf(format, args...))
}

func LogWarn(format string, args ...interface{}) {
	writeLog(os.Stdout, "warn", fmt.Sprintf(format, args...))
}

func LogError(format string, args ...interface{}) {
	writeLog(os.Stderr, "error", fmt.Sprintf(format, args...))
}
