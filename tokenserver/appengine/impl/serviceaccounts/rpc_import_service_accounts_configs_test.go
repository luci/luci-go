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
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	admin "go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestImportServiceAccountsConfigs(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := gaetesting.TestingContext()
		ctx, clk := testclock.UseTime(ctx, testclock.TestTimeUTC)

		ctx = prepareCfg(ctx, `rules {
			name: "rule 1"
			owner: "developer@example.com"
			service_account: "abc@robots.com"
			allowed_scope: "https://www.googleapis.com/scope"
			end_user: "user:abc@example.com"
			end_user: "group:group-name"
			proxy: "user:proxy@example.com"
			max_grant_validity_duration: 3600
		}`)

		rules := NewRulesCache()
		rpc := ImportServiceAccountsConfigsRPC{RulesCache: rules}

		// No config.
		r, err := rules.Rules(ctx)
		So(err, ShouldEqual, policy.ErrNoPolicy)

		resp, err := rpc.ImportServiceAccountsConfigs(ctx, nil)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.ImportedConfigs{
			Revision: "bd386b01fa0aaaa9d39c13bbafc4ef187c96f95b",
		})

		// Have config now.
		r, err = rules.Rules(ctx)
		So(err, ShouldBeNil)
		So(r.ConfigRevision(), ShouldEqual, "bd386b01fa0aaaa9d39c13bbafc4ef187c96f95b")

		// Noop import.
		resp, err = rpc.ImportServiceAccountsConfigs(ctx, nil)
		So(err, ShouldBeNil)
		So(resp.Revision, ShouldEqual, "bd386b01fa0aaaa9d39c13bbafc4ef187c96f95b")

		// Try to import completely broken config.
		ctx = prepareCfg(ctx, `I'm broken`)
		_, err = rpc.ImportServiceAccountsConfigs(ctx, nil)
		So(err, ShouldErrLike, `line 1.0: unknown field name`)

		// Old config is not replaced.
		r, _ = rules.Rules(ctx)
		So(r.ConfigRevision(), ShouldEqual, "bd386b01fa0aaaa9d39c13bbafc4ef187c96f95b")

		// Roll time to expire local rules cache.
		clk.Add(10 * time.Minute)

		// Have new config now!
		ctx = prepareCfg(ctx, `rules {
			name: "rule 2"
			owner: "developer@example.com"
			service_account: "abc@robots.com"
			allowed_scope: "https://www.googleapis.com/scope"
			end_user: "user:abc@example.com"
			end_user: "group:group-name"
			proxy: "user:proxy@example.com"
			max_grant_validity_duration: 3600
		}`)

		// Import it.
		resp, err = rpc.ImportServiceAccountsConfigs(ctx, nil)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.ImportedConfigs{
			Revision: "6c7967b3fb492fda4e4305aa17e6728d4408d344",
		})

		// It is now active.
		r, err = rules.Rules(ctx)
		So(err, ShouldBeNil)
		So(r.ConfigRevision(), ShouldEqual, "6c7967b3fb492fda4e4305aa17e6728d4408d344")
	})
}

func prepareCfg(c context.Context, configFile string) context.Context {
	return testconfig.WithCommonClient(c, memory.New(map[config.Set]memory.Files{
		config.Set("services/" + info.AppID(c)): {
			"service_accounts.cfg": configFile,
		},
	}))
}
