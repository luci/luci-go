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

package serviceaccounts

import (
	"sort"
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const fakeConfig = `
defaults {
	max_grant_validity_duration: 72000
	allowed_scope: "https://www.googleapis.com/default"
}
rules {
	name: "rule 1"
	owner: "developer@example.com"
	service_account: "abc@robots.com"
	service_account: "def@robots.com"
	service_account: "via-group1-and-rule1@robots.com"
	service_account_group: "account-group-1"
	allowed_scope: "https://www.googleapis.com/scope1"
	allowed_scope: "https://www.googleapis.com/scope2"
	end_user: "user:enduser@example.com"
	end_user: "group:enduser-group"
	proxy: "user:proxy@example.com"
	proxy: "group:proxy-group"
	trusted_proxy: "user:trusted-proxy@example.com"
}
rules {
	name: "rule 2"
	service_account: "xyz@robots.com"
	service_account: "via-group1-and-rule2@robots.com"
	service_account_group: "account-group-2"
}`

func TestRules(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(context.Background(), &authtest.FakeState{
		Identity: "user:unused@example.com",
		FakeDB: authtest.FakeDB{
			"user:via-group1@robots.com":           []string{"account-group-1"},
			"user:via-group2@robots.com":           []string{"account-group-2"},
			"user:via-both@robots.com":             []string{"account-group-1", "account-group-2"},
			"user:via-group1-and-rule1@robots.com": []string{"account-group-1"},
			"user:via-group1-and-rule2@robots.com": []string{"account-group-1"},
		},
	})

	Convey("Loads", t, func() {
		cfg, err := loadConfig(fakeConfig)
		So(err, ShouldBeNil)
		So(cfg, ShouldNotBeNil)

		rule, err := cfg.Rule(ctx, "abc@robots.com")
		So(err, ShouldBeNil)
		So(rule, ShouldNotBeNil)
		So(rule.Rule.Name, ShouldEqual, "rule 1")

		scopes := rule.AllowedScopes.ToSlice()
		sort.Strings(scopes)
		So(scopes, ShouldResemble, []string{
			"https://www.googleapis.com/default",
			"https://www.googleapis.com/scope1",
			"https://www.googleapis.com/scope2",
		})

		So(rule.CheckScopes([]string{"https://www.googleapis.com/scope1"}), ShouldBeNil)
		So(
			rule.CheckScopes([]string{"https://www.googleapis.com/scope1", "unknown_scope"}),
			ShouldErrLike,
			`following scopes are not allowed by the rule "rule 1" - ["unknown_scope"]`,
		)

		So(rule.EndUsers.ToStrings(), ShouldResemble, []string{
			"group:enduser-group",
			"user:enduser@example.com",
		})
		So(rule.Proxies.ToStrings(), ShouldResemble, []string{
			"group:proxy-group",
			"user:proxy@example.com",
		})
		So(rule.TrustedProxies.ToStrings(), ShouldResemble, []string{
			"user:trusted-proxy@example.com",
		})
		So(rule.Rule.MaxGrantValidityDuration, ShouldEqual, 72000)
	})

	Convey("Rule picker works", t, func() {
		cfg, err := loadConfig(fakeConfig)
		So(err, ShouldBeNil)
		So(cfg, ShouldNotBeNil)

		rule, err := cfg.Rule(ctx, "abc@robots.com")
		So(err, ShouldBeNil)
		So(rule.Rule.Name, ShouldEqual, "rule 1")

		rule, err = cfg.Rule(ctx, "def@robots.com")
		So(err, ShouldBeNil)
		So(rule.Rule.Name, ShouldEqual, "rule 1")

		rule, err = cfg.Rule(ctx, "xyz@robots.com")
		So(err, ShouldBeNil)
		So(rule.Rule.Name, ShouldEqual, "rule 2")

		rule, err = cfg.Rule(ctx, "via-group1@robots.com")
		So(err, ShouldBeNil)
		So(rule.Rule.Name, ShouldEqual, "rule 1")

		rule, err = cfg.Rule(ctx, "via-group2@robots.com")
		So(err, ShouldBeNil)
		So(rule.Rule.Name, ShouldEqual, "rule 2")

		rule, err = cfg.Rule(ctx, "via-both@robots.com")
		So(err, ShouldErrLike, `matches multiple rules: "rule 1", "rule 2"`)

		rule, err = cfg.Rule(ctx, "via-group1-and-rule1@robots.com")
		So(err, ShouldBeNil)
		So(rule.Rule.Name, ShouldEqual, "rule 1")

		rule, err = cfg.Rule(ctx, "via-group1-and-rule2@robots.com")
		So(err, ShouldErrLike, `matches multiple rules: "rule 1", "rule 2"`)

		rule, err = cfg.Rule(ctx, "unknown@robots.com")
		So(err, ShouldBeNil)
		So(rule, ShouldBeNil)
	})

	Convey("Check works", t, func() {
		cfg, err := loadConfig(fakeConfig)
		So(err, ShouldBeNil)
		So(cfg, ShouldNotBeNil)

		Convey("Happy path using 'proxy'", func() {
			r, err := cfg.Check(ctx, &RulesQuery{
				ServiceAccount: "abc@robots.com",
				Proxy:          "user:proxy@example.com",
				EndUser:        "user:enduser@example.com",
			})
			So(err, ShouldBeNil)
			So(r.Rule.Name, ShouldEqual, "rule 1")
		})

		Convey("Happy path using 'trusted_proxy'", func() {
			r, err := cfg.Check(ctx, &RulesQuery{
				ServiceAccount: "abc@robots.com",
				Proxy:          "user:trusted-proxy@example.com",
				EndUser:        "user:someone-random@example.com",
			})
			So(err, ShouldBeNil)
			So(r.Rule.Name, ShouldEqual, "rule 1")
		})

		Convey("Unknown service account", func() {
			_, err := cfg.Check(ctx, &RulesQuery{
				ServiceAccount: "unknown@robots.com",
				Proxy:          "user:proxy@example.com",
				EndUser:        "user:enduser@example.com",
			})
			So(err, ShouldBeRPCPermissionDenied, "unknown service account or not enough permissions to use it")
		})

		Convey("Unauthorized proxy", func() {
			_, err := cfg.Check(ctx, &RulesQuery{
				ServiceAccount: "abc@robots.com",
				Proxy:          "user:unknown@example.com",
				EndUser:        "user:enduser@example.com",
			})
			So(err, ShouldBeRPCPermissionDenied, "unknown service account or not enough permissions to use it")
		})

		Convey("Unauthorized end user", func() {
			_, err := cfg.Check(ctx, &RulesQuery{
				ServiceAccount: "abc@robots.com",
				Proxy:          "user:proxy@example.com",
				EndUser:        "user:unknown@example.com",
			})
			So(err, ShouldBeRPCPermissionDenied,
				`per rule "rule 1" the user "user:unknown@example.com" is not authorized to use the service account "abc@robots.com"`)
		})
	})
}

func loadConfig(text string) (*Rules, error) {
	cfg := &admin.ServiceAccountsPermissions{}
	err := proto.UnmarshalText(text, cfg)
	if err != nil {
		return nil, err
	}
	rules, err := prepareRules(policy.ConfigBundle{serviceAccountsCfg: cfg}, "fake-revision")
	if err != nil {
		return nil, err
	}
	return rules.(*Rules), nil
}
