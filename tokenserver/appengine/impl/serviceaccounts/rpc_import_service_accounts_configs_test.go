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
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"
	admin "github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/policy"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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
			allowed_scope: "https://scope"
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
			Revision: "16d15198a351a6a9beb7afe6f3485c9a47b18f7d",
		})

		// Have config now.
		r, err = rules.Rules(ctx)
		So(err, ShouldBeNil)
		So(r.ConfigRevision(), ShouldEqual, "16d15198a351a6a9beb7afe6f3485c9a47b18f7d")

		// Noop import.
		resp, err = rpc.ImportServiceAccountsConfigs(ctx, nil)
		So(err, ShouldBeNil)
		So(resp.Revision, ShouldEqual, "16d15198a351a6a9beb7afe6f3485c9a47b18f7d")

		// Try to import completely broken config.
		ctx = prepareCfg(ctx, `I'm broken`)
		_, err = rpc.ImportServiceAccountsConfigs(ctx, nil)
		So(err, ShouldErrLike, `line 1.0: unknown field name`)

		// Old config is not replaced.
		r, _ = rules.Rules(ctx)
		So(r.ConfigRevision(), ShouldEqual, "16d15198a351a6a9beb7afe6f3485c9a47b18f7d")

		// Roll time to expire local rules cache.
		clk.Add(10 * time.Minute)

		// Have new config now!
		ctx = prepareCfg(ctx, `rules {
			name: "rule 2"
			owner: "developer@example.com"
			service_account: "abc@robots.com"
			allowed_scope: "https://scope"
			end_user: "user:abc@example.com"
			end_user: "group:group-name"
			proxy: "user:proxy@example.com"
			max_grant_validity_duration: 3600
		}`)

		// Import it.
		resp, err = rpc.ImportServiceAccountsConfigs(ctx, nil)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.ImportedConfigs{
			Revision: "663ea6e319c41b6fcee65d8d1d5c758813aa0fb1",
		})

		// It is now active.
		r, err = rules.Rules(ctx)
		So(err, ShouldBeNil)
		So(r.ConfigRevision(), ShouldEqual, "663ea6e319c41b6fcee65d8d1d5c758813aa0fb1")
	})
}

func prepareCfg(c context.Context, configFile string) context.Context {
	return testconfig.WithCommonClient(c, memory.New(map[string]memory.ConfigSet{
		"services/" + info.AppID(c): {
			"service_accounts.cfg": configFile,
		},
	}))
}
