package telemetry

import (
	"runtime/debug"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

const testRevision = "abc123"

func TestBuildVersion(t *testing.T) {
	for _, test := range []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"missing metadata", nil, "devel"},
		{"no vcs", &debug.BuildInfo{}, "devel"},
		{"release", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, "v1.2.3"},
		{"revision", &debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: testRevision},
		}}, testRevision},
		{"dirty revision", &debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.modified", Value: "true"}, {Key: "vcs.revision", Value: testRevision},
		}}, "abc123-dirty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := buildVersion(test.info); got != test.want {
				t.Fatalf("version=%s want=%s", got, test.want)
			}
		})
	}
}

func TestResource(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.namespace=tripgo,deployment.environment=local")
	res, err := newResource(t.Context(), "trip-service")
	if err != nil {
		t.Fatal(err)
	}
	attrs := res.Set()
	version, ok := attrs.Value("service.version")
	if !ok || version.AsString() == "" {
		t.Fatal("build version is missing")
	}
	for key, want := range map[string]string{
		"service.name": "trip-service", "service.namespace": "tripgo", "deployment.environment": "local",
	} {
		value, ok := attrs.Value(attribute.Key(key))
		if !ok || value.AsString() != want {
			t.Fatalf("%s=%v want=%s", key, value, want)
		}
	}
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.version=release-42")
	res, err = newResource(t.Context(), "trip-service")
	if err != nil {
		t.Fatal(err)
	}
	version, _ = res.Set().Value("service.version")
	if version.AsString() != "release-42" {
		t.Fatalf("env version ignored: %v", version)
	}
}
