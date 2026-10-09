package cli

import (
	"context"

	"github.com/ushineko/hayami/internal/panel"
)

// DiagnoseSources is Diagnose over a test's sources, every one configured.
func DiagnoseSources(ctx context.Context, sources []panel.Source) []Finding {
	on := make(map[string]bool, len(sources))
	for _, s := range sources {
		on[s.Key()] = true
	}
	return diagnose(ctx, on, sources)
}

// WatchSettings is the terminal panel's look at its settings file.
var WatchSettings = watchSettings
