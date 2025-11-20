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
	"math/rand"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/armon/go-radix"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgcache"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/cipd/api/config/v1"
	"go.chromium.org/luci/cipd/common"
)

var cachedCfg = cfgcache.Register(&cfgcache.Entry{
	Path: "prefixes.cfg",
	Type: (*configpb.PrefixesConfigFile)(nil),
	Validator: func(c *validation.Context, msg proto.Message) error {
		validate(c, msg.(*configpb.PrefixesConfigFile))
		return nil
	},
})

// ImportConfig is called from a cron to import prefixes.cfg into the datastore.
func ImportConfig(ctx context.Context) error {
	_, err := cachedCfg.Update(ctx, nil)
	if errors.Is(err, config.ErrNoConfig) {
		logging.Warningf(ctx, "No prefixes.cfg config file")
		return nil
	}
	return err
}

// Config holds parsed and transformed configs in memory, refetching them from
// the datastore occasionally.
type Config struct {
	cur atomic.Value // contains *queryable that is periodically replaced
}

// queryable is a queryable representation of the prefixes.cfg config.
type queryable struct {
	rev  string      // the revision this config was derived from
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

// NewConfig loads the initial copy of the config from the datastore.
func NewConfig(ctx context.Context) (*Config, error) {
	raw, rev, err := fetch(ctx)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	transformed, err := transform(raw, rev)
	if err != nil {
		return nil, errors.Fmt("transforming initial config: %w", err)
	}
	cfg.cur.Store(transformed)
	return cfg, nil
}

// Lookup returns a config entry to use for the given prefix or package.
//
// It is a fast local operation and it can't fail. If the prefix doesn't have
// a config, returns a default one.
func (c *Config) Lookup(path string) *Entry {
	return c.queryable().lookup(path)
}

// RefreshPeriodically runs a loop that periodically refetches the config from
// the datastore to update the local in-memory cache.
func (c *Config) RefreshPeriodically(ctx context.Context) {
	for {
		jitter := time.Duration(rand.Int63n(int64(60 * time.Second)))
		if r := <-clock.After(ctx, 60*time.Second+jitter); r.Err != nil {
			return // the context is canceled
		}
		if err := c.refresh(ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				logging.Warningf(ctx, "Failed to refresh the local copy of the config: %s", err)
			}
		}
	}
}

// queryable is the queryable config stored in the atomic.
func (c *Config) queryable() *queryable {
	return c.cur.Load().(*queryable)
}

// refresh updates the in-memory config based on what's in the datastore.
func (c *Config) refresh(ctx context.Context) error {
	fetched, rev, err := fetch(ctx)
	if err != nil {
		return errors.Fmt("fetching config from datastore: %w", err)
	}
	cur := c.queryable()
	if cur.rev == rev {
		logging.Infof(ctx, "Config is up-to-date at rev %q", rev)
		return nil
	}
	logging.Infof(ctx, "Picking up config update: %q => %q", cur.rev, rev)
	transformed, err := transform(fetched, rev)
	if err != nil {
		return errors.Fmt("transforming fetched config: %w", err)
	}
	c.cur.Store(transformed)
	return nil
}

// fetch fetches the config from the datastore, returning a default one if
// the datastore is empty.
func fetch(ctx context.Context) (cfg *configpb.PrefixesConfigFile, rev string, err error) {
	var meta config.Meta
	switch raw, err := cachedCfg.Fetch(ctx, &meta); {
	case err == nil:
		return raw.(*configpb.PrefixesConfigFile), meta.Revision, nil
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return &configpb.PrefixesConfigFile{}, "none", nil
	default:
		return nil, "", err
	}
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
func transform(cfg *configpb.PrefixesConfigFile, rev string) (*queryable, error) {
	// We are going to mutate entries.
	entries := proto.Clone(cfg).(*configpb.PrefixesConfigFile).Prefix

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
	return &queryable{rev: rev, tree: tree}, nil
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

	prefix.AllowWritersFromRegexp = append(prefix.AllowWritersFromRegexp, ancestor.PrefixConfig.AllowWritersFromRegexp...)
	entry.AllowWritersFromRegexp = append(entry.AllowWritersFromRegexp, ancestor.AllowWritersFromRegexp...)
}
