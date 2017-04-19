// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

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

func TestImportDelegationConfigs(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := gaetesting.TestingContext()
		ctx, clk := testclock.UseTime(ctx, testclock.TestTimeUTC)

		ctx = prepareCfg(ctx, `rules {
			name: "rule 1"
			requestor: "user:some-user@example.com"
			target_service: "service:some-service"
			allowed_to_impersonate: "group:some-group"
			allowed_audience: "REQUESTOR"
			max_validity_duration: 86400
		}`)

		rules := NewRulesCache()
		rpc := ImportDelegationConfigsRPC{RulesCache: rules}

		// No config.
		r, err := rules.Rules(ctx)
		So(err, ShouldEqual, policy.ErrNoPolicy)

		resp, err := rpc.ImportDelegationConfigs(ctx, nil)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.ImportedConfigs{
			Revision: "780529c8f5e6b219e27482978d792d580f7d4f9d",
		})

		// Have config now.
		r, err = rules.Rules(ctx)
		So(err, ShouldBeNil)
		So(r.rules[0].rule.Name, ShouldEqual, "rule 1")
		So(r.revision, ShouldEqual, "780529c8f5e6b219e27482978d792d580f7d4f9d")

		// Noop import.
		resp, err = rpc.ImportDelegationConfigs(ctx, nil)
		So(err, ShouldBeNil)
		So(resp.Revision, ShouldEqual, "780529c8f5e6b219e27482978d792d580f7d4f9d")

		// Try to import completely broken config.
		ctx = prepareCfg(ctx, `I'm broken`)
		_, err = rpc.ImportDelegationConfigs(ctx, nil)
		So(err, ShouldErrLike, `line 1.0: unknown field name`)

		// Old config is not replaced.
		r, _ = rules.Rules(ctx)
		So(r.revision, ShouldEqual, "780529c8f5e6b219e27482978d792d580f7d4f9d")

		// Try to import a config that doesn't pass validation.
		ctx = prepareCfg(ctx, `rules {
			name: "rule 1"
		}`)
		_, err = rpc.ImportDelegationConfigs(ctx, nil)
		So(err, ShouldErrLike, `"requestor" is required (and 4 other errors)`)

		// Old config is not replaced.
		r, _ = rules.Rules(ctx)
		So(r.revision, ShouldEqual, "780529c8f5e6b219e27482978d792d580f7d4f9d")

		// Roll time to expire local rules cache.
		clk.Add(10 * time.Minute)

		// Have new config now!
		ctx = prepareCfg(ctx, `rules {
			name: "rule 2"
			requestor: "user:some-user@example.com"
			target_service: "service:some-service"
			allowed_to_impersonate: "group:some-group"
			allowed_audience: "REQUESTOR"
			max_validity_duration: 86400
		}`)

		// Import it.
		resp, err = rpc.ImportDelegationConfigs(ctx, nil)
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &admin.ImportedConfigs{
			Revision: "79770d0350b86402f434bf5479a800edeba64255",
		})

		// It is now active.
		r, err = rules.Rules(ctx)
		So(err, ShouldBeNil)
		So(r.rules[0].rule.Name, ShouldEqual, "rule 2")
		So(r.revision, ShouldEqual, "79770d0350b86402f434bf5479a800edeba64255")
	})
}

func prepareCfg(c context.Context, configFile string) context.Context {
	return testconfig.WithCommonClient(c, memory.New(map[string]memory.ConfigSet{
		"services/" + info.AppID(c): {
			"delegation.cfg": configFile,
		},
	}))
}
