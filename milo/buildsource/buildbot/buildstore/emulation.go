// Copyright 2017 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package buildstore

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/milo/api/proto"
)

var emulationOptionsProviderKey = "emulation options provider"

// WithEmulationOptions overrides the current emulation options for a builder.
func WithEmulationOptions(c context.Context, master, builder string, opt milo.EmulationOptions) context.Context {
	return WithEmulationOptionsProvider(c, func(m, b string) (*milo.EmulationOptions, bool) {
		if m == master && b == builder {
			return &opt, true
		}
		return nil, false
	})
}

// EmulationOptionsProvider returns the emulation options for a Buildbot
// builder.
type EmulationOptionsProvider func(master, builder string) (opt *milo.EmulationOptions, ok bool)

// WithEmulationOptionsProvider overrides the current emulation options.
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

// GetEmulationOptions returns the Buildbot emulation options for a Buildbot
// builder.
func GetEmulationOptions(c context.Context, master, builder string) *milo.EmulationOptions {
	if p := c.Value(&emulationOptionsProviderKey); p != nil {
		if opt, ok := p.(EmulationOptionsProvider)(master, builder); ok {
			return opt
		}
	}
	return nil
}
