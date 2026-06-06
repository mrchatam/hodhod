package debuglog

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const logPath = "/home/ali/Desktop/mirzapro/.cursor/debug-8a4c6b.log"

// Write appends one NDJSON debug line (session 8a4c6b).
func Write(hypothesisID, location, message string, data map[string]any) {
	// #region agent log
	entry := map[string]any{
		"sessionId":    "8a4c6b",
		"hypothesisId": hypothesisID,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG_NDJSON %s\n", string(b))
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
	// #endregion
}
