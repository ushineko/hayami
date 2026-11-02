package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ushineko/hayami/internal/config"
)

// FuzzSettingsFile: the settings file, which a person edits by hand
// (spec 074).
func FuzzSettingsFile(f *testing.F) {
	f.Add([]byte("arrangement: stack\nsections: [peripherals, bandwidth]\n"))
	f.Add([]byte("sectionSettings:\n  cooler:\n    lhm: http://127.0.0.1:8085/data.json\n"))
	f.Add([]byte(""))
	f.Add([]byte("{"))
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "settings.yaml")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := config.Open(path)
		if err != nil {
			return
		}
		_ = s.Config()
		_ = s.Close()
	})
}
