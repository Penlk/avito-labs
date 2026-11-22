package request_test

import (
	"strings"
	"testing"

	"github.com/Penlk/avito-labs/internal/http/request"
)

func TestDecodeJSON(t *testing.T) {
	const expectedOneObject = "expected one JSON object"

	tests := []struct {
		name  string
		input string
		err   string
	}{
		{"object", `{"name":"test"}`, ""},
		{"trailing whitespace", "{\"name\":\"test\"} \n\t", ""},
		{"unknown field", `{"name":"test","extra":1}`, `unknown field "extra"`},
		{"second object", `{"name":"test"} {}`, expectedOneObject},
		{"second value", `{"name":"test"} null`, expectedOneObject},
		{"trailing garbage", `{"name":"test"} garbage`, expectedOneObject},
		{"empty body", "", "EOF"},
		{"incomplete object", `{"name":`, "unexpected EOF"},
		{"wrong field type", `{"name":1}`, "cannot unmarshal number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body struct {
				Name string `json:"name"`
			}
			err := request.DecodeJSON(strings.NewReader(tt.input), &body)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("error=%v, want containing %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if body.Name != "test" {
				t.Fatalf("name=%q, want test", body.Name)
			}
		})
	}
}
