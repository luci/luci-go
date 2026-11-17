// Copyright 2025 The LUCI Authors.
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

package prefixcfg

import (
	"cmp"
	"context"
	"regexp"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/cfgcache"
	"go.chromium.org/luci/server/cfgcache/dsstore"

	configpb "go.chromium.org/luci/cipd/api/config/v1"
	"go.chromium.org/luci/cipd/appengine/impl/prefix"
	"go.chromium.org/luci/cipd/common"
)

var prefixesCfgFile = &cfgcache.SingleFile[*configpb.PrefixesConfigFile]{
	Path: "prefixes.cfg",
}

var defaultServiceTree, _ = transform(nil)

var serviceConfig = &cfgcache.ServicePipeline[*configpb.PrefixesConfigFile, *prefix.Tree[*ServiceEntry]]{
	ID:                 "cipd-prefixes",
	Store:              &dsstore.Store[*configpb.PrefixesConfigFile]{},
	Files:              []cfgcache.File{prefixesCfgFile},
	TransformVersion:   1,
	ToMemoryMaxVersion: 1,
	Transform: func(c *validation.Context, snap *cfgcache.Snapshot) (*configpb.PrefixesConfigFile, error) {
		msg := prefixesCfgFile.Get(snap)
		validate(c, msg)
		return msg, nil
	},
	ToMemory: func(ctx context.Context, cs config.Set, msg *configpb.PrefixesConfigFile) (*prefix.Tree[*ServiceEntry], error) {
		return transform(msg)
	},
	MemoryDefault: func() *prefix.Tree[*ServiceEntry] {
		return defaultServiceTree
	},
}

func RegisterConfig(reg *cfgcache.Registry) *Config {
	return &Config{
		serviceConfig.Register(reg),
	}
}

// Config holds parsed and transformed configs in memory, refreshed in the
// background.
type Config struct {
	svcHandle *cfgcache.RegisteredService[*prefix.Tree[*ServiceEntry]]
}

// ServiceEntry holds the resolved service-wide settings from configpb.Prefix:
//   - Path is omitted (stored in the prefix.Tree structure).
//   - AllowWritersFromRegexp entries are compiled into *regexp.Regexp.
//   - EnableLegacyAclFallback is resolved to a bool (defaulting to true at root).
//
// Returned *ServiceEntry values (and nested Billing protos) are shared across
// lookups and must not be mutated by callers.
type ServiceEntry struct {
	OwningLuciProject                 string
	Billing                           *configpb.Prefix_Billing
	AllowWritersFromRegexp            []*regexp.Regexp
	ExemptFromVerifySoftwareArtifacts bool
	EnableLegacyAclFallback           bool

	// hasEnableLegacyAclFallback tracks whether a prefixes.cfg entry set
	// EnableLegacyAclFallback explicitly (e.g. to false), without making
	// EnableLegacyAclFallback a `*bool`. Only used by the merge function in
	// `transform`.
	hasEnableLegacyAclFallback bool
}

func protoToServiceEntry(prefix *configpb.Prefix, out *ServiceEntry) error {
	var allowedWriters []*regexp.Regexp
	if len(prefix.AllowWritersFromRegexp) > 0 {
		allowedWriters = make([]*regexp.Regexp, len(prefix.AllowWritersFromRegexp))
		for i, re := range prefix.AllowWritersFromRegexp {
			var err error
			allowedWriters[i], err = regexp.Compile(re)
			if err != nil {
				return errors.Fmt("failed to compile validated regexp: %w", err)
			}
		}
	}

	*out = ServiceEntry{
		OwningLuciProject:                 prefix.OwningLuciProject,
		Billing:                           prefix.Billing,
		ExemptFromVerifySoftwareArtifacts: prefix.ExemptFromVerifySoftwareArtifacts,
		AllowWritersFromRegexp:            allowedWriters,
		EnableLegacyAclFallback:           prefix.GetEnableLegacyAclFallback(),
		hasEnableLegacyAclFallback:        prefix.EnableLegacyAclFallback != nil,
	}
	return nil
}

// Entry holds the effective configuration pertaining to a given CIPD package
// path.
type Entry struct {
	// ServiceEntry is all the global settings which pertain to the CIPD package
	// path.
	Service *ServiceEntry

	// TODO: Add project-specific Realm field.
}

// Lookup returns the effective service-wide configuration for `path`.
func (c *Config) Lookup(ctx context.Context, path string) (*Entry, error) {
	svcCfg, err := c.svcHandle.GetData(ctx)
	if err != nil {
		return nil, err
	}
	return &Entry{Service: svcCfg.Get(path)}, nil
}

// validate validates correctness of the config.
//
// Errors are reported through the validation.Context.
func validate(c *validation.Context, cfg *configpb.PrefixesConfigFile) {
	seen := stringset.New(len(cfg.Prefix))
	for idx, entry := range cfg.Prefix {
		c.Enter("prefix #%d %q", idx, entry.Path)
		pfx, err := common.ValidatePackagePrefix(entry.Path)
		if err != nil {
			c.Errorf("%s", err)
		} else if !seen.Add(pfx) {
			c.Errorf("this prefix was already configured")
		}
		if billing := entry.Billing; billing != nil {
			if billing.PercentOfCallsToBill < 0 || billing.PercentOfCallsToBill > 100 {
				c.Errorf("billing.percent_of_calls_to_bill should be within range [0, 100], got %d", billing.PercentOfCallsToBill)
			}
		}
		for _, re := range entry.AllowWritersFromRegexp {
			if _, err := regexp.Compile(re); err != nil {
				c.Errorf("invalid regexp for allowed writer: %s: %s", re, err)
			}
		}
		c.Exit()
	}
}

func transform(cfg *configpb.PrefixesConfigFile) (*prefix.Tree[*ServiceEntry], error) {
	prefixes := cfg.GetPrefix()
	// Allocate all entries in a single allocation, for funsies.
	allEntries := make([]ServiceEntry, len(prefixes))
	for i, pfx := range prefixes {
		if err := protoToServiceEntry(pfx, &allEntries[i]); err != nil {
			return nil, err
		}
	}

	defaultRoot := &ServiceEntry{EnableLegacyAclFallback: true}
	tree := prefix.New(defaultRoot, len(prefixes), func(yield func(string, *ServiceEntry) bool) {
		for i, pfx := range prefixes {
			if !yield(pfx.GetPath(), &allEntries[i]) {
				return
			}
		}
	}, func(parent, child *ServiceEntry) *ServiceEntry {
		child.OwningLuciProject = cmp.Or(child.OwningLuciProject, parent.OwningLuciProject)
		child.Billing = cmp.Or(child.Billing, parent.Billing)
		child.AllowWritersFromRegexp = append(child.AllowWritersFromRegexp, parent.AllowWritersFromRegexp...)
		child.ExemptFromVerifySoftwareArtifacts = cmp.Or(child.ExemptFromVerifySoftwareArtifacts, parent.ExemptFromVerifySoftwareArtifacts)
		if !child.hasEnableLegacyAclFallback {
			child.EnableLegacyAclFallback = parent.EnableLegacyAclFallback
		}
		return child
	})

	return tree, nil
}
