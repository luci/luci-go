// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/auth/identity"
	admin "github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/utils/identityset"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDelegationConfigLoader(t *testing.T) {
	Convey("DelegationConfigLoader works", t, func() {
		ctx := gaetesting.TestingContext()
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)

		loader := DelegationConfigLoader()

		// Put the initial copy into the datastore.
		cfg, err := loadConfig(`
			rules {
				name: "rule 1"
				requestor: "user:some-user@example.com"
				target_service: "service:some-service"
				allowed_to_impersonate: "group:some-group"
				allowed_audience: "REQUESTOR"
				max_validity_duration: 86400
			}`)
		So(err, ShouldBeNil)
		cfg.Revision = "1"
		So(datastore.Put(ctx, cfg), ShouldBeNil)

		// Loader fetches it.
		fetched1, err := loader(ctx)
		So(err, ShouldBeNil)
		So(fetched1.ParsedConfig.Rules[0].Name, ShouldEqual, "rule 1")

		// Config is updated.
		cfg, err = loadConfig(`
			rules {
				name: "rule 2"
				requestor: "user:some-user@example.com"
				target_service: "service:some-service"
				allowed_to_impersonate: "group:some-group"
				allowed_audience: "REQUESTOR"
				max_validity_duration: 86400
			}`)
		So(err, ShouldBeNil)
		cfg.Revision = "2"
		So(datastore.Put(ctx, cfg), ShouldBeNil)

		// Loader still returns old cached copy.
		fetched2, err := loader(ctx)
		So(err, ShouldBeNil)
		So(fetched2, ShouldEqual, fetched1)

		// Advance time to expire the cache. The new copy is fetched.
		tc.Add(procCacheExpiration + time.Second)
		fetched3, err := loader(ctx)
		So(err, ShouldBeNil)
		So(fetched3.ParsedConfig.Rules[0].Name, ShouldEqual, "rule 2")

		// Advance time again, but do not change the config. Loader reuses existing
		// object.
		tc.Add(procCacheExpiration + time.Second)
		fetched4, err := loader(ctx)
		So(err, ShouldBeNil)
		So(fetched4, ShouldEqual, fetched3)
	})
}

func TestIsAuthorizedRequestor(t *testing.T) {
	Convey("IsAuthorizedRequestor works", t, func() {
		cfg, err := loadConfig(`
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

		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:some-user@example.com",
		})
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
	Convey("with example config", t, func() {
		cfg, err := loadConfig(`
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

		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:requestor@example.com",
			FakeDB: authtest.FakeDB{
				"user:requestor-group-member@example.com":  []string{"requestor-group"},
				"user:delegators-group-member@example.com": []string{"delegators-group"},
				"user:audience-group-member@example.com":   []string{"audience-group"},
			},
		})

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
	})
}

func loadConfig(text string) (*DelegationConfig, error) {
	cfg := &admin.DelegationPermissions{}
	err := proto.UnmarshalText(text, cfg)
	if err != nil {
		return nil, err
	}
	blob, err := proto.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	c := &DelegationConfig{Config: blob}
	if err := c.Initialize(); err != nil {
		return nil, err
	}
	return c, nil
}

func makeSet(ident ...string) *identityset.Set {
	s, err := identityset.FromStrings(ident, nil)
	if err != nil {
		panic(err)
	}
	return s
}
