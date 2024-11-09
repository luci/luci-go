// Copyright 2018 The LUCI Authors.
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

package gitacls

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	configpb "go.chromium.org/luci/milo/proto/config"
)

func TestACLsWork(t *testing.T) {
	t.Parallel()
	c := gaetesting.TestingContext()

	ftt.Run("ACLs work", t, func(t *ftt.Test) {
		t.Run("Validation works", func(t *ftt.Test) {
			validate := func(cfg ...*configpb.Settings_SourceAcls) error {
				ctx := validation.Context{Context: c}
				ctx.SetFile("settings.cfg")
				ValidateConfig(&ctx, cfg)
				return ctx.Finalize()
			}
			mustError := func(cfg ...*configpb.Settings_SourceAcls) multiError {
				err := validate(cfg...)
				assert.Loosely(t, err, should.NotBeNil)
				return multiError(err.(*validation.Error).Errors)
			}

			valid := configpb.Settings_SourceAcls{
				Hosts:    []string{"a.googlesource.com"},
				Projects: []string{"https://b.googlesource.com/c"},
				Readers:  []string{"group:g", "user@example.com"},
			}
			assert.Loosely(t, validate(&valid), should.BeNil)

			mustError(&configpb.Settings_SourceAcls{}).with(
				t,
				"at least 1 reader required",
				"at least 1 host or project required",
			)

			t.Run("readers", func(t *ftt.Test) {
				mod := valid
				mod.Readers = []string{"bad:kind", "group:", "user", "group:a", "group:a"}
				mustError(&mod).with(
					t,
					`invalid readers "bad:kind"`,
					`invalid readers "group:": needs a group name`,
					`invalid readers "user"`,
					`duplicate`,
				)
			})

			t.Run("hosts", func(t *ftt.Test) {
				second := configpb.Settings_SourceAcls{
					Hosts: []string{
						valid.Hosts[0],
						"example.com",
						"repo.googlesource.com/repo",
						"b.googlesource.com", // valid.Project was from here, and it's OK.
					},
					Readers: []string{"group:a"},
				}
				mustError(&valid, &second).with(
					t,
					`host "a.googlesource.com"): has already been defined in source_acl #0`,
					`isn't at *.googlesource.com`,
					"shouldn't have path or fragment components",
				)
			})

			t.Run("projects", func(t *ftt.Test) {
				second := configpb.Settings_SourceAcls{
					Hosts: []string{"r.googlesource.com"},
					Projects: []string{
						valid.Projects[0], // dups of prev blocks are OK.
						"r.googlesource.com/redundant",
						"not-repo.googlesource.com",
						"c.googlesource.com/a/repo.git#123",
						"c-review.googlesource.com/src",
						"https://\\meh",
						valid.Projects[0], // dups of projects in this block is not OK.
					},
					Readers: []string{"group:b"},
				}
				mustError(&valid, &second).with(
					t,
					`redundant because already covered by its host in the same source_acls block`,
					`project "not-repo.googlesource.com"): should not be just a host`,
					`should not contain '/a' path prefix`,
					`should not contain '.git'`,
					`shouldn't have fragment components`,
					`must not be a Gerrit host (try without '-review')`,
					`not a valid URL`,
					`duplicate, already defined in the same source_acls block`,
				)
			})
		})

		load := func(cfg ...*configpb.Settings_SourceAcls) *ACLs {
			a, err := FromConfig(c, cfg)
			if err != nil {
				panic(err) // for stacktrace.
			}
			return a
		}

		t.Run("Loading works", func(t *ftt.Test) {
			assert.Loosely(t, load(
				&configpb.Settings_SourceAcls{
					Hosts: []string{"first.googlesource.com"},
					Projects: []string{
						"second.googlesource.com/y1",
						"third.googlesource.com/z",
					},
					Readers: []string{"user@example.com"},
				}),
				should.Resemble(
					&ACLs{
						hosts: map[string]*hostACLs{
							"first.googlesource.com": {
								itemACLs: itemACLs{
									definedIn: 0,
									readers:   []string{"user:user@example.com"},
								},
							},
							"second.googlesource.com": {
								itemACLs: itemACLs{definedIn: -1},
								projects: map[string]*itemACLs{
									"y1": {
										definedIn: 0,
										readers:   []string{"user:user@example.com"},
									},
								},
							},
							"third.googlesource.com": {
								itemACLs: itemACLs{definedIn: -1},
								projects: map[string]*itemACLs{
									"z": {
										definedIn: 0,
										readers:   []string{"user:user@example.com"},
									},
								},
							},
						},
					}))

			assert.Loosely(t, load(
				&configpb.Settings_SourceAcls{
					Hosts: []string{"first.googlesource.com"},
					Projects: []string{
						"second.googlesource.com/y1",
						"third.googlesource.com/z",
					},
					Readers: []string{"user@example.com"},
				},
				&configpb.Settings_SourceAcls{
					Hosts: []string{"third.googlesource.com"},
					Projects: []string{
						"first.googlesource.com/x",
						"second.googlesource.com/y1",
						"second.googlesource.com/y2",
					},
					Readers: []string{"group:g"},
				}),
				should.Resemble(
					&ACLs{
						hosts: map[string]*hostACLs{
							"first.googlesource.com": {
								itemACLs: itemACLs{
									definedIn: 0,
									readers:   []string{"user:user@example.com"},
								},
								projects: map[string]*itemACLs{
									"x": {
										definedIn: 1,
										readers:   []string{"group:g"},
									},
								},
							},
							"second.googlesource.com": {
								itemACLs: itemACLs{definedIn: -1},
								projects: map[string]*itemACLs{
									"y1": {
										definedIn: 1,
										readers:   []string{"group:g", "user:user@example.com"},
									},
									"y2": {
										definedIn: 1,
										readers:   []string{"group:g"},
									},
								},
							},
							"third.googlesource.com": {
								itemACLs: itemACLs{
									definedIn: 1,
									readers:   []string{"group:g"},
								},
								projects: map[string]*itemACLs{
									"z": {
										definedIn: 0,
										readers:   []string{"user:user@example.com"},
									},
								},
							},
						},
					}))
		})

		t.Run("IsAllowed works", func(t *ftt.Test) {
			acls := load(
				&configpb.Settings_SourceAcls{
					Hosts:    []string{"public.googlesource.com"},
					Projects: []string{"limited.googlesource.com/public"},
					Readers:  []string{"group:all"},
				},
				&configpb.Settings_SourceAcls{
					Hosts:    []string{"limited.googlesource.com"},
					Projects: []string{"c.googlesource.com/private"},
					Readers:  []string{"group:some", "they@example.com"},
				},
			)
			granted := func(ctx context.Context, host, project string) bool {
				r, err := acls.IsAllowed(ctx, host, project)
				if err != nil {
					panic(err)
				}
				return r
			}

			cAnon := auth.WithState(c, &authtest.FakeState{
				Identity:       identity.AnonymousIdentity,
				IdentityGroups: []string{"all"},
			})
			assert.Loosely(t, granted(cAnon, "public.googlesource.com", "any"), should.BeTrue)
			assert.Loosely(t, granted(cAnon, "limited.googlesource.com", "public"), should.BeTrue)
			assert.Loosely(t, granted(cAnon, "limited.googlesource.com", "any"), should.BeFalse)

			cThey := auth.WithState(c, &authtest.FakeState{
				Identity:       "user:they@example.com",
				IdentityGroups: []string{"all"},
			})
			assert.Loosely(t, granted(cThey, "limited.googlesource.com", "public"), should.BeTrue)
			assert.Loosely(t, granted(cThey, "limited.googlesource.com", "any"), should.BeTrue)
			assert.Loosely(t, granted(cThey, "c.googlesource.com", "private"), should.BeTrue)
			assert.Loosely(t, granted(cThey, "c.googlesource.com", "nope"), should.BeFalse)

			cSome := auth.WithState(c, &authtest.FakeState{
				Identity:       "user:some@example.com",
				IdentityGroups: []string{"some", "all"},
			})
			assert.Loosely(t, granted(cSome, "limited.googlesource.com", "any"), should.BeTrue)
			assert.Loosely(t, granted(cSome, "c.googlesource.com", "private"), should.BeTrue)
			assert.Loosely(t, granted(cSome, "c.googlesource.com", "nope"), should.BeFalse)

			t.Run("for Gerrit, too", func(t *ftt.Test) {
				assert.Loosely(t, granted(cAnon, "public-review.googlesource.com", "any"), should.BeTrue)
				assert.Loosely(t, granted(cThey, "limited-review.googlesource.com", "any"), should.BeTrue)
				assert.Loosely(t, granted(cSome, "c-review.googlesource.com", "private"), should.BeTrue)
			})
		})
	})
}

type multiError []error

func (m multiError) with(t *ftt.Test, substrings ...string) {
	for i, err := range m {
		if i >= len(substrings) {
			assert.Loosely(t, fmt.Errorf("extra errors produced: %q", m[i:]), should.BeNil)
		} else {
			assert.Loosely(t, err.Error(), should.ContainSubstring(substrings[i]))
		}
	}
	if len(substrings) > len(m) {
		panic(fmt.Errorf("not produced errors: %q", substrings[len(m):]))
	}
}
