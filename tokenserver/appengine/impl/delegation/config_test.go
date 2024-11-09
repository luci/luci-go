// Copyright 2016 The LUCI Authors.
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

package delegation

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	admin "go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/identityset"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
)

func TestIsAuthorizedRequestor(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(context.Background(), &authtest.FakeState{
		Identity: "user:some-user@example.com",
	})

	ftt.Run("IsAuthorizedRequestor works", t, func(t *ftt.Test) {
		cfg, err := loadConfig(ctx, `
			rules {
				name: "rule 1"
				requestor: "user:some-user@example.com"

				target_service: "service:some-service"
				allowed_to_impersonate: "group:some-group"
				allowed_audience: "REQUESTOR"
				max_validity_duration: 86400
			}

			rules {
				name: "rule 2"
				requestor: "user:some-another-user@example.com"
				requestor: "group:some-group"

				target_service: "service:some-service"
				allowed_to_impersonate: "group:some-group"
				allowed_audience: "REQUESTOR"
				max_validity_duration: 86400
			}
		`)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfg, should.NotBeNil)

		res, err := cfg.IsAuthorizedRequestor(ctx, identity.Identity("user:some-user@example.com"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.BeTrue)

		ctx = auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:some-another-user@example.com",
		})
		res, err = cfg.IsAuthorizedRequestor(ctx, identity.Identity("user:some-another-user@example.com"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.BeTrue)

		ctx = auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:unknown-user@example.com",
		})
		res, err = cfg.IsAuthorizedRequestor(ctx, identity.Identity("user:unknown-user@example.com"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.BeFalse)

		ctx = auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:via-group@example.com",
			IdentityGroups: []string{"some-group"},
		})
		res, err = cfg.IsAuthorizedRequestor(ctx, identity.Identity("user:via-group@example.com"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.BeTrue)
	})
}

func TestFindMatchingRule(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(context.Background(), &authtest.FakeState{
		Identity: "user:requestor@example.com",
		FakeDB: authtest.NewFakeDB(
			authtest.MockMembership("user:requestor-group-member@example.com", "requestor-group"),
			authtest.MockMembership("user:delegators-group-member@example.com", "delegators-group"),
			authtest.MockMembership("user:audience-group-member@example.com", "audience-group"),
			authtest.MockMembership("user:luci-service@example.com", "auth-luci-services"),
		),
	})

	ftt.Run("with example config", t, func(t *ftt.Test) {
		cfg, err := loadConfig(ctx, `
			rules {
				name: "rule 1"
				requestor: "user:requestor@example.com"
				target_service: "service:some-service"
				allowed_to_impersonate: "user:allowed-to-impersonate@example.com"
				allowed_audience: "user:allowed-audience@example.com"
				max_validity_duration: 86400
			}

			rules {
				name: "rule 2"
				requestor: "group:requestor-group"
				target_service: "service:some-service"
				allowed_to_impersonate: "group:delegators-group"
				allowed_audience: "group:audience-group"
				max_validity_duration: 86400
			}

			rules {
				name: "rule 3"
				requestor: "group:requestor-group"
				target_service: "service:some-service"
				allowed_to_impersonate: "REQUESTOR"
				allowed_audience: "REQUESTOR"
				max_validity_duration: 86400
			}

			rules {
				name: "rule 4"
				requestor: "user:some-requestor@example.com"
				requestor: "user:conflicts-with-rule-5@example.com"
				target_service: "*"
				allowed_to_impersonate: "REQUESTOR"
				allowed_audience: "*"
				max_validity_duration: 86400
			}

			rules {
				name: "rule 5"
				requestor: "user:conflicts-with-rule-5@example.com"
				target_service: "*"
				allowed_to_impersonate: "REQUESTOR"
				allowed_audience: "*"
				max_validity_duration: 86400
			}
		`)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfg, should.NotBeNil)

		t.Run("Direct matches and misses", func(t *ftt.Test) {
			// Match.
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor@example.com",
				Delegator: "user:allowed-to-impersonate@example.com",
				Audience:  makeSet("user:allowed-audience@example.com"),
				Services:  makeSet("service:some-service"),
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.Name, should.Equal("rule 1"))

			// Unknown requestor.
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:unknown-requestor@example.com",
				Delegator: "user:allowed-to-impersonate@example.com",
				Audience:  makeSet("user:allowed-audience@example.com"),
				Services:  makeSet("service:some-service"),
			})
			assert.Loosely(t, err, should.ErrLike("no matching delegation rules in the config"))
			assert.Loosely(t, res, should.BeNil)

			// Unknown delegator.
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor@example.com",
				Delegator: "user:unknown-allowed-to-impersonate@example.com",
				Audience:  makeSet("user:allowed-audience@example.com"),
				Services:  makeSet("service:some-service"),
			})
			assert.Loosely(t, err, should.ErrLike("no matching delegation rules in the config"))
			assert.Loosely(t, res, should.BeNil)

			// Unknown audience.
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor@example.com",
				Delegator: "user:allowed-to-impersonate@example.com",
				Audience:  makeSet("user:unknown-allowed-audience@example.com"),
				Services:  makeSet("service:some-service"),
			})
			assert.Loosely(t, err, should.ErrLike("no matching delegation rules in the config"))
			assert.Loosely(t, res, should.BeNil)

			// Unknown target service.
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor@example.com",
				Delegator: "user:allowed-to-impersonate@example.com",
				Audience:  makeSet("user:allowed-audience@example.com"),
				Services:  makeSet("service:unknown-some-service"),
			})
			assert.Loosely(t, err, should.ErrLike("no matching delegation rules in the config"))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("Matches via groups", func(t *ftt.Test) {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor-group-member@example.com",
				Delegator: "user:delegators-group-member@example.com",
				Audience:  makeSet("group:audience-group"),
				Services:  makeSet("service:some-service"),
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.Name, should.Equal("rule 2"))

			// Doesn't do group lookup when checking audience!
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor-group-member@example.com",
				Delegator: "user:delegators-group-member@example.com",
				Audience:  makeSet("user:audience-group-member@example.com"),
				Services:  makeSet("service:some-service"),
			})
			assert.Loosely(t, err, should.ErrLike("no matching delegation rules in the config"))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("REQUESTOR rules work", func(t *ftt.Test) {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor-group-member@example.com",
				Delegator: "user:requestor-group-member@example.com",
				Audience:  makeSet("user:requestor-group-member@example.com"),
				Services:  makeSet("service:some-service"),
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.Name, should.Equal("rule 3"))
		})

		t.Run("'*' rules work", func(t *ftt.Test) {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:some-requestor@example.com",
				Delegator: "user:some-requestor@example.com",
				Audience:  makeSet("group:abc", "user:def@example.com"),
				Services:  makeSet("service:unknown"),
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.Name, should.Equal("rule 4"))
		})

		t.Run("a conflict is handled", func(t *ftt.Test) {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:conflicts-with-rule-5@example.com",
				Delegator: "user:conflicts-with-rule-5@example.com",
				Audience:  makeSet("group:abc", "user:def@example.com"),
				Services:  makeSet("service:unknown"),
			})
			assert.Loosely(t, err, should.ErrLike(`ambiguous request, multiple delegation rules match ("rule 4", "rule 5")`))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("implicit project:* rule works", func(t *ftt.Test) {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:luci-service@example.com",
				Delegator: "project:some-project",
				Audience:  makeSet("user:luci-service@example.com"),
				Services:  makeSet("service:some-target-service"),
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)
			assert.Loosely(t, res.Name, should.Equal("allow-project-identities"))
		})
	})
}

func loadConfig(ctx context.Context, text string) (*Rules, error) {
	cfg := &admin.DelegationPermissions{}
	err := prototext.Unmarshal([]byte(text), cfg)
	if err != nil {
		return nil, err
	}
	rules, err := prepareRules(ctx, policy.ConfigBundle{delegationCfg: cfg}, "fake-revision")
	if err != nil {
		return nil, err
	}
	return rules.(*Rules), nil
}

func makeSet(ident ...string) *identityset.Set {
	s, err := identityset.FromStrings(ident, nil)
	if err != nil {
		panic(err)
	}
	return s
}
