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
	"slices"
	"strings"

	"github.com/armon/go-radix"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/cfgcache"
	"go.chromium.org/luci/server/cfgcache/dsstore"

	configpb "go.chromium.org/luci/cipd/api/config/v1"
	"go.chromium.org/luci/cipd/common"
)

var prefixesCfgFile = &cfgcache.SingleFile[*configpb.PrefixesConfigFile]{
	Path: "prefixes.cfg",
}

var serviceConfig = (&cfgcache.ServicePipeline[*configpb.PrefixesConfigFile, *queryable]{
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
	ToMemory: func(ctx context.Context, cs config.Set, msg *configpb.PrefixesConfigFile) (*queryable, error) {
		return transform(msg)
	},
})

func RegisterConfig(reg *cfgcache.Registry) *Config {
	return &Config{
		serviceConfig.Register(reg),
	}
}

// Config holds parsed and transformed configs in memory, refreshed in the
// background.
type Config struct {
	svcHandle *cfgcache.RegisteredService[*queryable]
}

func (c *Config) Lookup(ctx context.Context, path string) (*Entry, error) {
	q, err := c.svcHandle.GetData(ctx)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return &Entry{PrefixConfig: &configpb.Prefix{}}, nil
	}
	return q.lookup(path), nil
}

// queryable is a queryable representation of the prefixes.cfg config.
type queryable struct {
	tree *radix.Tree // "<prefix>/" -> resolved *configpb.Prefix
}

type Entry struct {
	PrefixConfig           *configpb.Prefix
	AllowWritersFromRegexp []*regexp.Regexp
}

func (qr *queryable) lookup(path string) *Entry {
	// Per construction in `transform` we always have a root (as ""). The lookup
	// will always return something.
	_, cfg, _ := qr.tree.LongestPrefix(normPath(path))
	return cfg.(*Entry)
}

// validate validates correctness of the config.
//
// Errors are reported through the validation.Context.
func validate(c *validation.Context, cfg *configpb.PrefixesConfigFile) {
	seen := stringset.New(len(cfg.Prefix))
	for idx, entry := range cfg.Prefix {
		c.Enter("prefix #%d %q", idx, entry.Path)
		if _, err := common.ValidatePackagePrefix(entry.Path); err != nil {
			c.Errorf("%s", err)
		} else if !seen.Add(normPath(entry.Path)) {
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

// transform takes a validated config and transforms it into a queryable form.
func transform(cfg *configpb.PrefixesConfigFile) (*queryable, error) {
	// We are going to mutate entries.
	entries := proto.CloneOf(cfg).GetPrefix()

	// Normalize paths to be compatible with longest prefix match algo for paths.
	// I.e. as a path "a/b/c" is a prefix of "a/b/c/d", but not a prefix of
	// "a/b/cd".
	seenRoot := false
	for _, entry := range entries {
		entry.Path = normPath(entry.Path)
		seenRoot = seenRoot || entry.Path == ""
	}

	// Insert an explicit root entry if one is not already defined. This entry
	// also will become the default config in case prefixes.cfg is completely
	// absent (in that case `entries` above is an empty slice).
	if !seenRoot {
		entries = append(entries, &configpb.Prefix{})
	}

	// Start putting nodes into the tree, root to leafs, resolving inheritance.
	slices.SortFunc(entries, func(a, b *configpb.Prefix) int {
		return cmp.Compare(len(a.Path), len(b.Path))
	})
	tree := radix.New()
	for _, prefix := range entries {
		entry := &Entry{
			PrefixConfig: prefix,
		}
		for _, re := range prefix.AllowWritersFromRegexp {
			compiled, err := regexp.Compile(re)
			if err != nil {
				return nil, errors.Fmt("failed to compile validated regexp: %w", err)
			}
			entry.AllowWritersFromRegexp = append(entry.AllowWritersFromRegexp, compiled)
		}

		if _, ancestor, ok := tree.LongestPrefix(prefix.Path); ok {
			inheritFromAncestor(entry, ancestor.(*Entry))
		}

		tree.Insert(prefix.Path, entry)
	}
	return &queryable{tree: tree}, nil
}

// normPath normalizes the prefix path for radix tree lookups.
func normPath(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

// inheritFromAncestor mutates `entry` by picking values from its parent config.
func inheritFromAncestor(entry, ancestor *Entry) {
	prefix := entry.PrefixConfig
	if prefix.OwningLuciProject == "" {
		prefix.OwningLuciProject = ancestor.PrefixConfig.OwningLuciProject
	}
	if prefix.Billing == nil {
		prefix.Billing = ancestor.PrefixConfig.Billing
	}
	// Allow explicitly overriding the fallback in a deeper prefix, but it's
	// otherwise sticky.
	if entry.PrefixConfig.EnableLegacyAclFallback == nil && ancestor.PrefixConfig.EnableLegacyAclFallback != nil {
		entry.PrefixConfig.EnableLegacyAclFallback = proto.Bool(ancestor.PrefixConfig.GetEnableLegacyAclFallback())
	}
	prefix.ExemptFromVerifySoftwareArtifacts = ancestor.PrefixConfig.ExemptFromVerifySoftwareArtifacts
	prefix.AllowWritersFromRegexp = append(prefix.AllowWritersFromRegexp, ancestor.PrefixConfig.AllowWritersFromRegexp...)
	entry.AllowWritersFromRegexp = append(entry.AllowWritersFromRegexp, ancestor.AllowWritersFromRegexp...)
}
