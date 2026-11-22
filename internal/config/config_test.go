package config_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Penlk/avito-labs/internal/config"
)

func TestWorkerConfig(t *testing.T) {
	example, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(example), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			t.Setenv(key, value)
		}
	}
	t.Setenv("POSITION_WORKERS", "")
	t.Setenv("PUSH_REQUEST_TIMEOUT", "")
	cfg, err := config.NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PositionWorkers != 4 || cfg.PushRequestTimeout != time.Second {
		t.Fatal("unexpected worker defaults")
	}
	for _, test := range []struct{ key, value string }{
		{"POSITION_WORKERS", "0"},
		{"PUSH_REQUEST_TIMEOUT", "0s"},
		{"PUSH_HTTP_URL", "/api/v1/push"},
		{"PUSH_HTTP_URL", ""},
	} {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := config.NewConfig(); err == nil {
				t.Fatal("invalid worker configuration was accepted")
			}
		})
	}
}
