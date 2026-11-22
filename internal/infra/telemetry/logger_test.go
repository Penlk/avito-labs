package telemetry_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Penlk/avito-labs/internal/infra/telemetry"
)

func TestStartupErrorJSON(t *testing.T) {
	var output bytes.Buffer
	telemetry.LogError(t.Context(), &output, "", errors.New("invalid config\nsecond line"))
	if strings.Count(output.String(), "\n") != 1 {
		t.Fatalf("split record: %q", output.String())
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["level"] != "ERROR" || record["service"] != "trip-service" ||
		record["error"] != "invalid config\nsecond line" || record["time"] == "" ||
		record["trace_id"] != "" || record["span_id"] != "" {
		t.Fatalf("invalid error log: %v", record)
	}
}
