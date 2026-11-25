package panel

import "github.com/ushineko/hayami/internal/config"

/*
ApplySettings hands a source what it reads from the settings while it runs:
the bandwidth section its interfaces and VPNs, the usage section its
Development settings. A source that reads nothing from them is left alone.
It is what a shell does when the settings file changes, so a running panel or
service follows the change without being started again (spec 113 R8).
*/
func ApplySettings(s Source, c config.Config) {
	switch src := s.(type) {
	case *Bandwidth:
		settings := config.Bandwidth.Get(c)
		src.SetInterfaces(settings.Interfaces)
		src.SetVPNs(settings)
	case *Usage:
		src.SetDevelopment(config.Usage.Get(c))
	}
}
