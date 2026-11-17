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
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/cfgcache"

	configpb "go.chromium.org/luci/cipd/api/config/v1"
)

func init() {
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(regexp.Regexp{}))
	// We ignore the unexported fields (specifically, hasEnableLegacyAclFallback)
	// because they are not part of the public, observable, API of ServiceEntry.
	registry.RegisterCmpOption(cmpopts.IgnoreUnexported(ServiceEntry{}))
}

func TestTransform(t *testing.T) {
	t.Parallel()

	type query struct {
		path string
		want *ServiceEntry
	}

	pfx := func(path, proj string, billPercent int, allowedWriters []string, exemptVSA bool, enableFallback *bool) *configpb.Prefix {
		var billing *configpb.Prefix_Billing
		if billPercent != 0 {
			billing = &configpb.Prefix_Billing{
				PercentOfCallsToBill: int32(billPercent),
			}
		}

		return &configpb.Prefix{
			Path:                              path,
			OwningLuciProject:                 proj,
			Billing:                           billing,
			AllowWritersFromRegexp:            allowedWriters,
			ExemptFromVerifySoftwareArtifacts: exemptVSA,
			EnableLegacyAclFallback:           enableFallback,
		}
	}

	ent := func(proj string, billPercent int, allowedWriters []string, exemptVSA bool, enableFallback bool) *ServiceEntry {
		ret := &ServiceEntry{
			OwningLuciProject:                 proj,
			ExemptFromVerifySoftwareArtifacts: exemptVSA,
			EnableLegacyAclFallback:           enableFallback,
		}
		if billPercent != 0 {
			ret.Billing = &configpb.Prefix_Billing{
				PercentOfCallsToBill: int32(billPercent),
			}
		}
		for _, raw := range allowedWriters {
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
				{"", ent("", 0, nil, false, true)},
				{"a", ent("", 0, nil, false, true)},
				{"a/b", ent("", 0, nil, false, true)},
				{"a/b/", ent("", 0, nil, false, true)},
			},
		},

		{
			name: "explicit_root",
			cfg: []*configpb.Prefix{
				pfx("a/b", "ab", 0, nil, false, proto.Bool(false)),
				pfx("", "root", 0, nil, false, nil),
			},
			queries: []query{
				{"", ent("root", 0, nil, false, true)},
				{"/", ent("root", 0, nil, false, true)},
				{"a", ent("root", 0, nil, false, true)},
				{"a/", ent("root", 0, nil, false, true)},
				{"a/b", ent("ab", 0, nil, false, false)},
				{"a/b/", ent("ab", 0, nil, false, false)},
				{"a/b/c", ent("ab", 0, nil, false, false)},
				{"a/b/c/", ent("ab", 0, nil, false, false)},
				{"a/d", ent("root", 0, nil, false, true)},
				{"a/bc", ent("root", 0, nil, false, true)},
			},
		},

		{
			name: "inheritance",
			cfg: []*configpb.Prefix{
				pfx("a/b/replace/deep/deeper", "y", 0, []string{"re4"}, false, proto.Bool(false)),
				pfx("a/b/replace", "x", 2, []string{"re3"}, true, proto.Bool(true)),
				pfx("a/b/inherit", "", 0, nil, false, nil),
				pfx("a", "a", 1, []string{"re1", "re2"}, false, proto.Bool(false)),
			},
			queries: []query{
				{"", ent("", 0, nil, false, true)},
				{"a", ent("a", 1, []string{"re1", "re2"}, false, false)},
				{"a/b", ent("a", 1, []string{"re1", "re2"}, false, false)},
				{"a/b/c", ent("a", 1, []string{"re1", "re2"}, false, false)},
				{"a/b/inherit", ent("a", 1, []string{"re1", "re2"}, false, false)},
				{"a/b/inherit/deeper", ent("a", 1, []string{"re1", "re2"}, false, false)},
				{"a/b/replace", ent("x", 2, []string{"re3", "re1", "re2"}, true, true)},
				{"a/b/replace/deep", ent("x", 2, []string{"re3", "re1", "re2"}, true, true)},
				{"a/b/replace/deep/deeper", ent("y", 2, []string{"re4", "re3", "re1", "re2"}, true, false)},
			},
		},
	}

	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			qr, err := transform(&configpb.PrefixesConfigFile{Prefix: cs.cfg})
			assert.NoErr(t, err)
			for _, q := range cs.queries {
				assert.That(t, qr.Get(q.path), should.Match(q.want), truth.Explain("path %q", q.path))
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

	ctx, cfgstate := cfgcache.TestingContext(memory.Use(t.Context()))
	datastore.GetTestable(ctx).Consistent(true)

	cfg := RegisterConfig(cfgstate.Registry)
	assert.NoErr(t, cfgstate.Initialize(ctx))
	assert.NoErr(t, cfgstate.RefreshInMemory(ctx))

	// The config is missing initially, but it is OK.
	ent, err := cfg.Lookup(ctx, "a/b/c")
	assert.NoErr(t, err)
	assert.That(t, ent, should.Match(&Entry{
		Service: &ServiceEntry{
			EnableLegacyAclFallback: true,
		},
	}))

	// Load the initial config into the datastore.
	cfgstate.SetConfig("services/my-app", "prefixes.cfg", &configpb.PrefixesConfigFile{
		Prefix: []*configpb.Prefix{
			{Path: "a/b/c", OwningLuciProject: "proj-abc"},
		},
	})
	assert.NoErr(t, cfgstate.IngestToStorage(ctx))

	// Refresh picks it up.
	assert.NoErr(t, cfgstate.RefreshInMemory(ctx))
	ent, err = cfg.Lookup(ctx, "a/b/c/d")
	assert.NoErr(t, err)
	assert.That(t, ent, should.Match(&Entry{
		Service: &ServiceEntry{
			OwningLuciProject:       "proj-abc",
			EnableLegacyAclFallback: true,
		},
	}))

	// Config changes.
	cfgstate.SetConfig("services/my-app", "prefixes.cfg", &configpb.PrefixesConfigFile{
		Prefix: []*configpb.Prefix{
			{Path: "a/b/c", OwningLuciProject: "proj-abc"},
			{Path: "a/b/c/d", OwningLuciProject: "proj-abcd"},
		},
	})
	assert.NoErr(t, cfgstate.IngestToStorage(ctx))

	// Refresh picks it up.
	assert.NoErr(t, cfgstate.RefreshInMemory(ctx))
	ent, err = cfg.Lookup(ctx, "a/b/c/d")
	assert.NoErr(t, err)
	assert.That(t, ent, should.Match(&Entry{
		Service: &ServiceEntry{
			OwningLuciProject:       "proj-abcd",
			EnableLegacyAclFallback: true,
		},
	}))
}
