package buildstore

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/milo/api/proto"
)

var emulationOptionsProviderKey = "emulation options provider"

// WithEmulationOptions overrides current emulation options for a builder.
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

// WithEmulationOptionsProvider overrides current emulation options.
func WithEmulationOptionsProvider(c context.Context, provider EmulationOptionsProvider) context.Context {
	prev := c.Value(&emulationOptionsProviderKey)
	if prev != nil {
		this := provider
		provider = func(master, builder string) (*milo.EmulationOptions, bool) {
			if opt, ok := this(master, builder); ok {
				return opt, true
			}
			return prev.(EmulationOptionsProvider)(master, builder)
		}
	}
	return context.WithValue(c, &emulationOptionsProviderKey, provider)
}

// GetEmulationOptions returns Buildbot emulation options for a builder.
func GetEmulationOptions(c context.Context, master, builder string) *milo.EmulationOptions {
	if p := c.Value(&emulationOptionsProviderKey); p != nil {
		if opt, ok := p.(EmulationOptionsProvider)(master, builder); ok {
			return opt
		}
	}
	return nil
}
