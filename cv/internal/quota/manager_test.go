// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package quota

import (
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	. "go.chromium.org/luci/common/testing/assertions"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/server/quota/quotapb"
	_ "go.chromium.org/luci/server/quota/quotatestmonkeypatch"
	"go.chromium.org/luci/server/redisconn"
)

func TestManager(t *testing.T) {
	t.Parallel()

	Convey("Manager", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		s, err := miniredis.Run()
		So(err, ShouldBeNil)
		defer s.Close()
		s.SetTime(clock.Now(ctx))

		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

		makeIdentity := func(email string) identity.Identity {
			id, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
			So(err, ShouldBeNil)
			return id
		}

		const tEmail = "t@example.org"
		ct.AddMember(tEmail, "googlers")
		ct.AddMember(tEmail, "partners")

		const lProject = "chromium"
		cg := &cfgpb.ConfigGroup{Name: "infra"}
		cfg := &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{cg}}
		prjcfgtest.Create(ctx, lProject, cfg)

		genUserLimit := func(name string, limit int64, principals []string) *cfgpb.UserLimit {
			userLimit := &cfgpb.UserLimit{
				Name:       name,
				Principals: principals,
				Run: &cfgpb.UserLimit_Run{
					MaxActive: &cfgpb.UserLimit_Limit{},
				},
			}

			if limit == 0 {
				userLimit.Run.MaxActive.Limit = &cfgpb.UserLimit_Limit_Unlimited{
					Unlimited: true,
				}
			} else {
				userLimit.Run.MaxActive.Limit = &cfgpb.UserLimit_Limit_Value{
					Value: limit,
				}
			}

			return userLimit
		}

		qm := NewManager()

		Convey("WritePolicy() with config groups but no run limit", func() {
			pid, err := qm.WritePolicy(ctx, lProject)
			So(err, ShouldBeNil)
			So(pid, ShouldBeNil)
		})

		Convey("WritePolicy() with run limit values", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			So(err, ShouldBeNil)
			So(pid, ShouldResembleProto, &quotapb.PolicyConfigID{
				AppId:   "cv",
				Realm:   "chromium",
				Version: pid.Version,
			})
		})

		Convey("WritePolicy() with run limit defaults", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
			)
			cg.UserLimitDefault = genUserLimit("chromies-limit", 5, []string{"group:chromies", "group:chrome-infra"})
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			So(err, ShouldBeNil)
			So(pid, ShouldResembleProto, &quotapb.PolicyConfigID{
				AppId:   "cv",
				Realm:   "chromium",
				Version: pid.Version,
			})
		})

		Convey("WritePolicy() with run limit set to unlimited", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 0, []string{"group:googlers"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			So(err, ShouldBeNil)
			So(pid, ShouldResembleProto, &quotapb.PolicyConfigID{
				AppId:   "cv",
				Realm:   "chromium",
				Version: pid.Version,
			})
		})

		Convey("findRunPolicy() returns first valid run policy", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			rs := &state.RunState{
				Run: run.Run{
					ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
					ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
					Owner:         makeIdentity(tEmail),
				},
			}

			res, err := qm.findRunPolicy(ctx, rs)
			So(err, ShouldBeNil)
			So(res.Key, ShouldResembleProto, runPolicyKey("infra", "googlers-limit"))
		})

		Convey("findRunPolicy() works with user entry in principals", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("example-limit", 10, []string{"group:chromies", "user:t@example.org"}),
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			rs := &state.RunState{
				Run: run.Run{
					ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
					ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
					Owner:         makeIdentity(tEmail),
				},
			}

			res, err := qm.findRunPolicy(ctx, rs)
			So(err, ShouldBeNil)
			So(res.Key, ShouldResembleProto, runPolicyKey("infra", "example-limit"))
		})

		Convey("findRunPolicy() returns default policy if no valid policy is found", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:invalid"}),
				genUserLimit("partners-limit", 10, []string{"group:invalid"}),
			)
			cg.UserLimitDefault = genUserLimit("", 5, nil)
			prjcfgtest.Update(ctx, lProject, cfg)

			rs := &state.RunState{
				Run: run.Run{
					ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
					ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
					Owner:         makeIdentity(tEmail),
				},
			}

			res, err := qm.findRunPolicy(ctx, rs)
			So(err, ShouldBeNil)
			So(res.Key, ShouldResembleProto, runPolicyKey("infra", "default"))
		})

		Convey("findRunPolicy() returns nil when no valid policy is found", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:invalid"}),
				genUserLimit("partners-limit", 10, []string{"group:invalid"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			rs := &state.RunState{
				Run: run.Run{
					ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
					ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
					Owner:         makeIdentity(tEmail),
				},
			}

			res, err := qm.findRunPolicy(ctx, rs)
			So(err, ShouldBeNil)
			So(res, ShouldBeNil)
		})

		Convey("DebitRunQuota() debits quota for a given run state", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			rs := &state.RunState{
				Run: run.Run{
					ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
					ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
					Owner:         makeIdentity(tEmail),
				},
			}

			res, err := qm.DebitRunQuota(ctx, rs)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			})

			cg.UserLimits = nil
		})

		Convey("CreditRunQuota() credits quota for a given run state", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			rs := &state.RunState{
				Run: run.Run{
					ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
					ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
					Owner:         makeIdentity(tEmail),
				},
			}

			res, err := qm.CreditRunQuota(ctx, rs)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:      5,
				PreviousBalance: 4, // credit reapplies debit beforehand.
			})

			cg.UserLimits = nil
		})

		Convey("runQuotaOp() updates the same account on multiple ops", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			rs := &state.RunState{
				Run: run.Run{
					ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
					ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
					Owner:         makeIdentity(tEmail),
				},
			}

			res, err := qm.runQuotaOp(ctx, rs, "foo1", 5)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    10,
				AccountStatus: quotapb.OpResult_CREATED,
			})

			res, err = qm.runQuotaOp(ctx, rs, "foo2", 5)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:      15,
				PreviousBalance: 10,
				AccountStatus:   quotapb.OpResult_ALREADY_EXISTS,
			})

			cg.UserLimits = nil
		})

		Convey("runQuotaOp() is idempotent", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			rs := &state.RunState{
				Run: run.Run{
					ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
					ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
					Owner:         makeIdentity(tEmail),
				},
			}

			res, err := qm.runQuotaOp(ctx, rs, "foo", 5)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    10,
				AccountStatus: quotapb.OpResult_CREATED,
			})

			res, err = qm.runQuotaOp(ctx, rs, "foo", 5)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    10,
				AccountStatus: quotapb.OpResult_CREATED,
			})

			res, err = qm.runQuotaOp(ctx, rs, "foo2", 5)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:      15,
				PreviousBalance: 10,
				AccountStatus:   quotapb.OpResult_ALREADY_EXISTS,
			})

			res, err = qm.runQuotaOp(ctx, rs, "foo2", 5)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:      15,
				PreviousBalance: 10,
				AccountStatus:   quotapb.OpResult_ALREADY_EXISTS,
			})

			cg.UserLimits = nil
		})
	})
}
