package buildstore

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/milo/api/proto"
)

var emulationOptionsKey = "emulation options"

// WithEmulationOptions overrides default emulation options for a builder.
func WithEmulationOptions(c context.Context, master, builder string, opt milo.EmulationOptions) context.Context {
	return WithEmulationOptionsProvider(c, func(m, b string) (*milo.EmulationOptions, bool) {
		if m == master && b == builder {
			return &opt, true
		}
		return nil, false
	})
}

// EmulationOptionsProvider returns emulation options for a Buildbot builder.
type EmulationOptionsProvider func(master, builder string) (opt *milo.EmulationOptions, ok bool)

// WithEmulationOptionsProvider overrides default emulation options.
func WithEmulationOptionsProvider(c context.Context, provider EmulationOptionsProvider) context.Context {
	prev := c.Value(&emulationOptionsKey)
	if prev != nil {
		this := provider
		provider = func(master, builder string) (*milo.EmulationOptions, bool) {
			if opt, ok := this(master, builder); ok {
				return opt, true
			}
			return prev.(EmulationOptionsProvider)(master, builder)
		}
	}
	return context.WithValue(c, &emulationOptionsKey, provider)
}

// GetEmulationOptions returns Buildbot emulation options for a builder.
func GetEmulationOptions(c context.Context, master, builder string) *milo.EmulationOptions {
	provider := c.Value(&emulationOptionsKey)
	if provider != nil {
		if opt, ok := provider.(EmulationOptionsProvider)(master, builder); ok {
			return opt
		}
	}
	return nil
}
