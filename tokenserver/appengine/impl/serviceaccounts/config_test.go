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

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRules(t *testing.T) {
	t.Parallel()

	Convey("Loads", t, func() {
		cfg, err := loadConfig(`
			rules {
				name: "rule 1"
				owner: "developer@example.com"
				service_account: "abc@robots.com"
				service_account: "def@robots.com"
				allowed_scope: "https://www.googleapis.com/scope1"
				allowed_scope: "https://www.googleapis.com/scope2"
				end_user: "user:abc@example.com"
				end_user: "group:enduser-group"
				proxy: "user:proxy@example.com"
				proxy: "group:proxy-group"
			}
			rules {
				name: "rule 2"
				service_account: "xyz@robots.com"
			}
		`)
		So(err, ShouldBeNil)
		So(cfg, ShouldNotBeNil)

		rule := cfg.Rule("abc@robots.com")
		So(rule, ShouldNotBeNil)
		So(rule.Rule.Name, ShouldEqual, "rule 1")

		scopes := rule.AllowedScopes.ToSlice()
		sort.Strings(scopes)
		So(scopes, ShouldResemble, []string{
			"https://www.googleapis.com/scope1",
			"https://www.googleapis.com/scope2",
		})

		So(rule.EndUsers.ToStrings(), ShouldResemble, []string{
			"group:enduser-group",
			"user:abc@example.com",
		})
		So(rule.Proxies.ToStrings(), ShouldResemble, []string{
			"group:proxy-group",
			"user:proxy@example.com",
		})
		So(rule.Rule.MaxGrantValidityDuration, ShouldEqual, 24*3600)

		So(cfg.Rule("def@robots.com").Rule.Name, ShouldEqual, "rule 1")
		So(cfg.Rule("xyz@robots.com").Rule.Name, ShouldEqual, "rule 2")
		So(cfg.Rule("unknown@robots.com"), ShouldBeNil)
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
