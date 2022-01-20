// Copyright 2022 The LUCI Authors.
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

package acls

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckRunCLs(t *testing.T) {
	t.Parallel()

	const (
		lProject   = "chromium"
		gerritHost = "chromium-review.googlesource.com"
	)

	Convey("CheckRunCreate", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		authState := &authtest.FakeState{}
		ctx = auth.WithState(ctx, authState)
		cg := prjcfg.ConfigGroup{
			Content: &cfgpb.ConfigGroup{
				Verifiers: &cfgpb.Verifiers{
					GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
						CommitterList: []string{"grp1"},
					},
				},
			},
		}
		rid := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))

		// test helpers
		var rCLs []*run.RunCL
		addRunCL := func(trigger, owner string) {
			id := len(rCLs) + 1
			rCLs = append(rCLs, &run.RunCL{
				ID:         common.CLID(id),
				Run:        datastore.MakeKey(ctx, run.RunKind, string(rid)),
				ExternalID: changelist.MustGobID(gerritHost, int64(id)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gerritHost,
							Info: &gerritpb.ChangeInfo{
								Owner: &gerritpb.AccountInfo{
									Email: owner,
								},
							},
						},
					},
				},
				Trigger: &run.Trigger{Email: trigger},
			})
		}
		setCommitterMembership := func(email string) {
			id, err := identity.MakeIdentity("user:" + email)
			So(err, ShouldBeNil)
			authState.FakeDB = authtest.NewFakeDB(authtest.MockMembership(id, "grp1"))
		}
		mustOK := func() {
			res, err := CheckRunCreate(ctx, &cg, rCLs)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeTrue)
		}
		mustFail := func(format string, args ...interface{}) {
			res, err := CheckRunCreate(ctx, &cg, rCLs)
			So(err, ShouldBeNil)
			So(res.OK(), ShouldBeFalse)

			if format != "" {
				msg := fmt.Sprintf(format, args...)
				So(res.FailuresSummary, ShouldContainSubstring, msg)
			}
		}

		Convey("trigger != owner", func() {
			addRunCL("tr1@example.org", "owner@example.org")

			Convey("trigger is a committer", func() {
				setCommitterMembership("tr1@example.org")
				mustOK()
			})
			Convey("trigger is not a committer", func() {
				mustFail("* only the full committers or CL owner can trigger runs")
			})
		})

		Convey("trigger == owner", func() {
			addRunCL("tr1@example.org", "tr1@example.org")

			Convey("trigger is a committer", func() {
				setCommitterMembership("tr1@example.org")
				mustOK()
			})
			Convey("trigger is not a committer", func() {
				mustOK()
			})
		})

		Convey("multiple owners", func() {
			addRunCL("tr1@example.org", "tr1@example.org")
			addRunCL("tr1@example.org", "ow1@example.org")

			Convey("trigger is a committer", func() {
				setCommitterMembership("tr1@example.org")
				mustOK()
			})
			Convey("trigger is not a committer", func() {
				mustFail("* only the full committers or CL owner can trigger runs.\n" +
					rCLs[1].ExternalID.MustURL())
			})
		})
	})
}
