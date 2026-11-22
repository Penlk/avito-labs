package telemetry

import (
	"context"
	"runtime/debug"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

func newResource(ctx context.Context, service string) (*resource.Resource, error) {
	info, _ := debug.ReadBuildInfo()
	return resource.New(ctx,
		resource.WithAttributes(attribute.String("service.version", buildVersion(info))),
		resource.WithFromEnv(),
		resource.WithAttributes(attribute.String("service.name", service)))
}

func buildVersion(info *debug.BuildInfo) string {
	if info == nil {
		return "devel"
	}
	if version := info.Main.Version; version != "" && version != "(devel)" {
		return version
	}
	var revision string
	var dirty bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if revision == "" {
		return "devel"
	}
	if dirty {
		revision += "-dirty"
	}
	return revision
}
