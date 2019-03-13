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

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	admin "go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/identityset"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestIsAuthorizedRequestor(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(context.Background(), &authtest.FakeState{
		Identity: "user:some-user@example.com",
	})

	Convey("IsAuthorizedRequestor works", t, func() {
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
		So(err, ShouldBeNil)
		So(cfg, ShouldNotBeNil)

		res, err := cfg.IsAuthorizedRequestor(ctx, identity.Identity("user:some-user@example.com"))
		So(err, ShouldBeNil)
		So(res, ShouldBeTrue)

		ctx = auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:some-another-user@example.com",
		})
		res, err = cfg.IsAuthorizedRequestor(ctx, identity.Identity("user:some-another-user@example.com"))
		So(err, ShouldBeNil)
		So(res, ShouldBeTrue)

		ctx = auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:unknown-user@example.com",
		})
		res, err = cfg.IsAuthorizedRequestor(ctx, identity.Identity("user:unknown-user@example.com"))
		So(err, ShouldBeNil)
		So(res, ShouldBeFalse)

		ctx = auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:via-group@example.com",
			IdentityGroups: []string{"some-group"},
		})
		res, err = cfg.IsAuthorizedRequestor(ctx, identity.Identity("user:via-group@example.com"))
		So(err, ShouldBeNil)
		So(res, ShouldBeTrue)
	})
}

func TestFindMatchingRule(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(context.Background(), &authtest.FakeState{
		Identity: "user:requestor@example.com",
		FakeDB: authtest.FakeDB{
			"user:requestor-group-member@example.com":  []string{"requestor-group"},
			"user:delegators-group-member@example.com": []string{"delegators-group"},
			"user:audience-group-member@example.com":   []string{"audience-group"},
			"user:luci-service@example.com":            []string{"auth-luci-services"},
		},
	})

	Convey("with example config", t, func() {
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
		So(err, ShouldBeNil)
		So(cfg, ShouldNotBeNil)

		Convey("Direct matches and misses", func() {
			// Match.
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor@example.com",
				Delegator: "user:allowed-to-impersonate@example.com",
				Audience:  makeSet("user:allowed-audience@example.com"),
				Services:  makeSet("service:some-service"),
			})
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)
			So(res.Name, ShouldEqual, "rule 1")

			// Unknown requestor.
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:unknown-requestor@example.com",
				Delegator: "user:allowed-to-impersonate@example.com",
				Audience:  makeSet("user:allowed-audience@example.com"),
				Services:  makeSet("service:some-service"),
			})
			So(err, ShouldErrLike, "no matching delegation rules in the config")
			So(res, ShouldBeNil)

			// Unknown delegator.
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor@example.com",
				Delegator: "user:unknown-allowed-to-impersonate@example.com",
				Audience:  makeSet("user:allowed-audience@example.com"),
				Services:  makeSet("service:some-service"),
			})
			So(err, ShouldErrLike, "no matching delegation rules in the config")
			So(res, ShouldBeNil)

			// Unknown audience.
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor@example.com",
				Delegator: "user:allowed-to-impersonate@example.com",
				Audience:  makeSet("user:unknown-allowed-audience@example.com"),
				Services:  makeSet("service:some-service"),
			})
			So(err, ShouldErrLike, "no matching delegation rules in the config")
			So(res, ShouldBeNil)

			// Unknown target service.
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor@example.com",
				Delegator: "user:allowed-to-impersonate@example.com",
				Audience:  makeSet("user:allowed-audience@example.com"),
				Services:  makeSet("service:unknown-some-service"),
			})
			So(err, ShouldErrLike, "no matching delegation rules in the config")
			So(res, ShouldBeNil)
		})

		Convey("Matches via groups", func() {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor-group-member@example.com",
				Delegator: "user:delegators-group-member@example.com",
				Audience:  makeSet("group:audience-group"),
				Services:  makeSet("service:some-service"),
			})
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)
			So(res.Name, ShouldEqual, "rule 2")

			// Doesn't do group lookup when checking audience!
			res, err = cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor-group-member@example.com",
				Delegator: "user:delegators-group-member@example.com",
				Audience:  makeSet("user:audience-group-member@example.com"),
				Services:  makeSet("service:some-service"),
			})
			So(err, ShouldErrLike, "no matching delegation rules in the config")
			So(res, ShouldBeNil)
		})

		Convey("REQUESTOR rules work", func() {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:requestor-group-member@example.com",
				Delegator: "user:requestor-group-member@example.com",
				Audience:  makeSet("user:requestor-group-member@example.com"),
				Services:  makeSet("service:some-service"),
			})
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)
			So(res.Name, ShouldEqual, "rule 3")
		})

		Convey("'*' rules work", func() {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:some-requestor@example.com",
				Delegator: "user:some-requestor@example.com",
				Audience:  makeSet("group:abc", "user:def@example.com"),
				Services:  makeSet("service:unknown"),
			})
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)
			So(res.Name, ShouldEqual, "rule 4")
		})

		Convey("a conflict is handled", func() {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:conflicts-with-rule-5@example.com",
				Delegator: "user:conflicts-with-rule-5@example.com",
				Audience:  makeSet("group:abc", "user:def@example.com"),
				Services:  makeSet("service:unknown"),
			})
			So(err, ShouldErrLike, `ambiguous request, multiple delegation rules match ("rule 4", "rule 5")`)
			So(res, ShouldBeNil)
		})

		Convey("implicit project:* rule works", func() {
			res, err := cfg.FindMatchingRule(ctx, &RulesQuery{
				Requestor: "user:luci-service@example.com",
				Delegator: "project:some-project",
				Audience:  makeSet("user:luci-service@example.com"),
				Services:  makeSet("service:some-target-service"),
			})
			So(err, ShouldBeNil)
			So(res, ShouldNotBeNil)
			So(res.Name, ShouldEqual, "allow-project-identities")
		})
	})
}

func loadConfig(ctx context.Context, text string) (*Rules, error) {
	cfg := &admin.DelegationPermissions{}
	err := proto.UnmarshalText(text, cfg)
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
