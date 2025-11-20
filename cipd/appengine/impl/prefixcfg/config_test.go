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
	"context"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	configpb "go.chromium.org/luci/cipd/api/config/v1"
)

func TestMain(m *testing.M) {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(regexp.Regexp{}))
}

func TestTransform(t *testing.T) {
	t.Parallel()

	type query struct {
		path string
		want *Entry
	}

	pfx := func(path, proj string, billPercent int, allowedWriters []string) *configpb.Prefix {
		var billing *configpb.Prefix_Billing
		if billPercent != 0 {
			billing = &configpb.Prefix_Billing{
				PercentOfCallsToBill: int32(billPercent),
			}
		}

		return &configpb.Prefix{
			Path:                   path,
			OwningLuciProject:      proj,
			Billing:                billing,
			AllowWritersFromRegexp: allowedWriters,
		}
	}

	ent := func(path, proj string, billPercent int, allowedWriters []string) *Entry {
		p := pfx(path, proj, billPercent, allowedWriters)
		ret := &Entry{
			PrefixConfig: p,
		}
		for _, raw := range p.AllowWritersFromRegexp {
			ret.AllowWritersFromRegexp = append(ret.AllowWritersFromRegexp, regexp.MustCompile(raw))
		}

		return ret
	}

	cases := []struct {
		name    string
		cfg     []*configpb.Prefix
		queries []query
	}{
		{
			name: "empty",
			queries: []query{
				{"", &Entry{PrefixConfig: &configpb.Prefix{}}},
				{"a", &Entry{PrefixConfig: &configpb.Prefix{}}},
				{"a/b", &Entry{PrefixConfig: &configpb.Prefix{}}},
				{"a/b/", &Entry{PrefixConfig: &configpb.Prefix{}}},
			},
		},

		{
			name: "explicit_root",
			cfg: []*configpb.Prefix{
				pfx("a/b", "ab", 0, nil),
				pfx("", "root", 0, nil),
			},
			queries: []query{
				{"", ent("", "root", 0, nil)},
				{"a", ent("", "root", 0, nil)},
				{"a/b", ent("a/b/", "ab", 0, nil)},
				{"a/b/c", ent("a/b/", "ab", 0, nil)},
				{"a/d", ent("", "root", 0, nil)},
				{"a/bc", ent("", "root", 0, nil)},
			},
		},

		{
			name: "inheritance",
			cfg: []*configpb.Prefix{
				pfx("a/b/replace/deep/deeper", "y", 0, []string{"re4"}),
				pfx("a/b/replace", "x", 2, []string{"re3"}),
				pfx("a/b/inherit", "", 0, nil),
				pfx("a", "a", 1, []string{"re1", "re2"}),
			},
			queries: []query{
				{"", ent("", "", 0, nil)},
				{"a", ent("a/", "a", 1, []string{"re1", "re2"})},
				{"a/b", ent("a/", "a", 1, []string{"re1", "re2"})},
				{"a/b/c", ent("a/", "a", 1, []string{"re1", "re2"})},
				{"a/b/inherit", ent("a/b/inherit/", "a", 1, []string{"re1", "re2"})},
				{"a/b/inherit/deeper", ent("a/b/inherit/", "a", 1, []string{"re1", "re2"})},
				{"a/b/replace", ent("a/b/replace/", "x", 2, []string{"re3", "re1", "re2"})},
				{"a/b/replace/deep", ent("a/b/replace/", "x", 2, []string{"re3", "re1", "re2"})},
				{"a/b/replace/deep/deeper", ent("a/b/replace/deep/deeper/", "y", 2, []string{"re4", "re3", "re1", "re2"})},
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			qr, err := transform(&configpb.PrefixesConfigFile{Prefix: cs.cfg}, "")
			assert.NoErr(t, err)
			for _, q := range cs.queries {
				got := qr.lookup(q.path)
				assert.That(t, got, should.Match(q.want), truth.Explain("path %q", q.path))
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *configpb.PrefixesConfigFile
		errs []string
	}{
		{
			name: "ok",
			cfg: &configpb.PrefixesConfigFile{
				Prefix: []*configpb.Prefix{
					{
						Path:              "/",
						OwningLuciProject: "root",
					},
					{
						Path:              "a/b/c",
						OwningLuciProject: "proj",
					},
					{
						Path: "a/b/c/d",
						Billing: &configpb.Prefix_Billing{
							PercentOfCallsToBill: 100,
						},
					},
				},
			},
		},
		{
			name: "empty",
			cfg:  &configpb.PrefixesConfigFile{},
		},
		{
			name: "bad_pfx",
			cfg: &configpb.PrefixesConfigFile{
				Prefix: []*configpb.Prefix{
					{Path: "ABC"},
				},
			},
			errs: []string{`(prefix #0 "ABC"): invalid package prefix "ABC"`},
		},
		{
			name: "dup_pfx",
			cfg: &configpb.PrefixesConfigFile{
				Prefix: []*configpb.Prefix{
					{Path: "a/b"},
					{Path: "a/b/"},
				},
			},
			errs: []string{`(prefix #1 "a/b/"): this prefix was already configured`},
		},
		{
			name: "bad_percent",
			cfg: &configpb.PrefixesConfigFile{
				Prefix: []*configpb.Prefix{
					{Path: "a/b", Billing: &configpb.Prefix_Billing{PercentOfCallsToBill: 101}},
				},
			},
			errs: []string{`(prefix #0 "a/b"): billing.percent_of_calls_to_bill should be within range [0, 100], got 101`},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			var ctx validation.Context
			validate(&ctx, cs.cfg)
			err := ctx.Finalize()
			if len(cs.errs) == 0 {
				assert.NoErr(t, err)
			} else {
				errs := err.(*validation.Error).Errors
				assert.That(t, len(errs), should.Equal(len(cs.errs)))
				for i, err := range errs {
					assert.That(t, err, should.ErrLike(cs.errs[i]))
				}
			}
		})
	}
}

func TestConfig(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(t.Context())

	// The config is missing initially, but it is OK.
	cfg, err := NewConfig(ctx)
	assert.NoErr(t, err)
	assert.That(t, cfg.queryable().tree.Len(), should.Equal(1)) // only the implicit root

	// Refresh doesn't freak out.
	assert.NoErr(t, cfg.refresh(ctx))

	// Load the initial config into the datastore.
	err = ImportConfig(withConfig(ctx, &configpb.PrefixesConfigFile{
		Prefix: []*configpb.Prefix{
			{Path: "a/b/c"},
		},
	}))
	assert.NoErr(t, err)

	// Refresh picks it up.
	assert.NoErr(t, cfg.refresh(ctx))
	assert.That(t, cfg.queryable().tree.Len(), should.Equal(2))
	prev := cfg.queryable()

	// Noop refresh.
	assert.NoErr(t, cfg.refresh(ctx))
	assert.That(t, cfg.queryable(), should.Equal(prev))

	// Config changes.
	err = ImportConfig(withConfig(ctx, &configpb.PrefixesConfigFile{
		Prefix: []*configpb.Prefix{
			{Path: "a/b/c"},
			{Path: "a/b/c/d"},
		},
	}))
	assert.NoErr(t, err)

	// Refresh picks it up.
	assert.NoErr(t, cfg.refresh(ctx))
	assert.That(t, cfg.queryable().tree.Len(), should.Equal(3))
}

func withConfig(ctx context.Context, cfg *configpb.PrefixesConfigFile) context.Context {
	body, err := prototext.Marshal(cfg)
	if err != nil {
		panic(err)
	}
	return cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
		"services/${appid}": {"prefixes.cfg": string(body)},
	}))
}
