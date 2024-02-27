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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"
	_ "go.chromium.org/luci/server/quota/quotatestmonkeypatch"
	"go.chromium.org/luci/server/redisconn"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

		Convey("findRunLimit() returns first valid user_limit", func() {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, err := findRunLimit(ctx, r)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, googlerLimit)
		})

		Convey("findRunLimit() works with user entry in principals", func() {
			exampleLimit := genUserLimit("example-limit", 10, []string{"group:chromies", "user:t@example.org"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				exampleLimit,
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}
			res, err := findRunLimit(ctx, r)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, exampleLimit)
		})

		Convey("findRunLimit() returns default user_limit if no valid user_limit is found", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:invalid"}),
				genUserLimit("partners-limit", 10, []string{"group:invalid"}),
			)
			cg.UserLimitDefault = genUserLimit("", 5, nil)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, err := findRunLimit(ctx, r)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, genUserLimit("default", 5, nil)) // default name is overriden.
		})

		Convey("findRunLimit() returns nil when no valid policy is found", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:invalid"}),
				genUserLimit("partners-limit", 10, []string{"group:invalid"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, err := findRunLimit(ctx, r)
			So(err, ShouldBeNil)
			So(res, ShouldBeNil)
		})

		Convey("DebitRunQuota() debits quota for a given run state", func() {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, userLimit, err := qm.DebitRunQuota(ctx, r)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			})
			So(ct.TSMonSentValue(
				ctx,
				metrics.Internal.QuotaOp,
				lProject,
				"infra",
				"googlers-limit",
				"runs",
				"debit",
				"SUCCESS",
			), ShouldEqual, 1)
		})

		Convey("CreditRunQuota() credits quota for a given run state", func() {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, userLimit, err := qm.CreditRunQuota(ctx, r)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    5,
				AccountStatus: quotapb.OpResult_CREATED,
			})
			So(ct.TSMonSentValue(
				ctx,
				metrics.Internal.QuotaOp,
				lProject,
				"infra",
				"googlers-limit",
				"runs",
				"credit",
				"SUCCESS",
			), ShouldEqual, 1)
		})

		Convey("runQuotaOp() updates the same account on multiple ops", func() {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, userLimit, err := qm.runQuotaOp(ctx, r, "foo1", -1)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			})
			So(ct.TSMonSentValue(
				ctx,
				metrics.Internal.QuotaOp,
				lProject,
				"infra",
				"googlers-limit",
				"runs",
				"foo1",
				"SUCCESS",
			), ShouldEqual, 1)

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo2", -2)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:              2,
				PreviousBalance:         4,
				PreviousBalanceAdjusted: 4,
				AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
			})
			So(ct.TSMonSentValue(
				ctx,
				metrics.Internal.QuotaOp,
				lProject,
				"infra",
				"googlers-limit",
				"runs",
				"foo2",
				"SUCCESS",
			), ShouldEqual, 1)
		})

		Convey("runQuotaOp() respects unlimited policy", func() {
			googlerLimit := genUserLimit("googlers-limit", 0, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits, googlerLimit)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, userLimit, err := qm.runQuotaOp(ctx, r, "", -1)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    -1,
				AccountStatus: quotapb.OpResult_CREATED,
			})
		})

		Convey("runQuotaOp() bound checks", func() {
			googlerLimit := genUserLimit("googlers-limit", 1, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits, googlerLimit)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			Convey("quota underflow", func() {
				res, userLimit, err := qm.runQuotaOp(ctx, r, "debit", -2)
				So(err, ShouldEqual, quota.ErrQuotaApply)
				So(userLimit, ShouldResembleProto, googlerLimit)
				So(res, ShouldResembleProto, &quotapb.OpResult{
					AccountStatus: quotapb.OpResult_CREATED,
					Status:        quotapb.OpResult_ERR_UNDERFLOW,
				})
				So(ct.TSMonSentValue(
					ctx,
					metrics.Internal.QuotaOp,
					lProject,
					"infra",
					"googlers-limit",
					"runs",
					"debit",
					"ERR_UNDERFLOW",
				), ShouldEqual, 1)
			})

			Convey("quota overflow", func() {
				// overflow doesn't err but gets capped.
				res, userLimit, err := qm.runQuotaOp(ctx, r, "credit", 10)
				So(err, ShouldBeNil)
				So(userLimit, ShouldResembleProto, googlerLimit)
				So(res, ShouldResembleProto, &quotapb.OpResult{
					AccountStatus: quotapb.OpResult_CREATED,
					NewBalance:    1,
				})
				So(ct.TSMonSentValue(
					ctx,
					metrics.Internal.QuotaOp,
					lProject,
					"infra",
					"googlers-limit",
					"runs",
					"credit",
					"SUCCESS",
				), ShouldEqual, 1)
			})
		})

		Convey("runQuotaOp() on policy change", func() {
			chromiesLimit := genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"})
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			partnerLimit := genUserLimit("partners-limit", 2, []string{"group:partners"})
			cg.UserLimits = append(cg.UserLimits, chromiesLimit, googlerLimit, partnerLimit)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, userLimit, err := qm.runQuotaOp(ctx, r, "", -1)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			})

			Convey("decrease in quota allowance results in underflow", func() {
				// Update policy
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       makeIdentity(tEmail),
					IdentityGroups: []string{"partners"},
				})

				// This is not a real scenario within CV but just checks
				// extreme examples.
				res, userLimit, err = qm.runQuotaOp(ctx, r, "", -2)
				So(err, ShouldEqual, quota.ErrQuotaApply)
				So(userLimit, ShouldResembleProto, partnerLimit)
				So(res, ShouldResembleProto, &quotapb.OpResult{
					PreviousBalance:         4,
					PreviousBalanceAdjusted: 1,
					Status:                  quotapb.OpResult_ERR_UNDERFLOW,
				})
			})

			Convey("decrease in quota allowance within bounds", func() {
				// Update policy
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       makeIdentity(tEmail),
					IdentityGroups: []string{"partners"},
				})

				res, userLimit, err = qm.runQuotaOp(ctx, r, "", -1)
				So(err, ShouldBeNil)
				So(userLimit, ShouldResembleProto, partnerLimit)
				So(res, ShouldResembleProto, &quotapb.OpResult{
					NewBalance:              0,
					PreviousBalance:         4,
					PreviousBalanceAdjusted: 1,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				})
			})

			Convey("increase in quota allowance", func() {
				// Update policy
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       makeIdentity(tEmail),
					IdentityGroups: []string{"chromies"},
				})

				res, userLimit, err = qm.runQuotaOp(ctx, r, "", -1)
				So(err, ShouldBeNil)
				So(userLimit, ShouldResembleProto, chromiesLimit)
				So(res, ShouldResembleProto, &quotapb.OpResult{
					NewBalance:              8,
					PreviousBalance:         4,
					PreviousBalanceAdjusted: 9,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				})
			})

			Convey("increase in quota allowance in the overflow case is bounded by the new limit", func() {
				// Update policy
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       makeIdentity(tEmail),
					IdentityGroups: []string{"chromies"},
				})

				// This is not a real scenario within CV but just checks
				// extreme examples.
				res, userLimit, err = qm.runQuotaOp(ctx, r, "", 2)
				So(err, ShouldBeNil)
				So(userLimit, ShouldResembleProto, chromiesLimit)
				So(res, ShouldResembleProto, &quotapb.OpResult{
					NewBalance:              10,
					PreviousBalance:         4,
					PreviousBalanceAdjusted: 9,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				})
			})
		})

		Convey("runQuotaOp() is idempotent", func() {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			res, userLimit, err := qm.runQuotaOp(ctx, r, "foo", -1)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			})

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo", -1)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			})

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo2", -2)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:              2,
				PreviousBalance:         4,
				PreviousBalanceAdjusted: 4,
				AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
			})

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo2", -2)
			So(err, ShouldBeNil)
			So(userLimit, ShouldResembleProto, googlerLimit)
			So(res, ShouldResembleProto, &quotapb.OpResult{
				NewBalance:              2,
				PreviousBalance:         4,
				PreviousBalanceAdjusted: 4,
				AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
			})
		})

		Convey("RunQuotaAccountID() hashes emailID", func() {
			r := &run.Run{
				ID:            common.MakeRunID(lProject, time.Now(), 1, []byte{}),
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.ComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
			}

			emailHash := md5.Sum([]byte(tEmail))

			So(qm.RunQuotaAccountID(r), ShouldResembleProto, &quotapb.AccountID{
				AppId:        "cv",
				Realm:        "chromium",
				Namespace:    "infra",
				Name:         hex.EncodeToString(emailHash[:]),
				ResourceType: "runs",
			})
		})
	})
}
