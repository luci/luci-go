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

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/redisconn"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"

	_ "go.chromium.org/luci/server/quota/quotatestmonkeypatch"
)

func TestManager(t *testing.T) {
	t.Parallel()

	ftt.Run("Manager", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		s, err := miniredis.Run()
		assert.Loosely(t, err, should.BeNil)
		defer s.Close()
		s.SetTime(clock.Now(ctx))

		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

		makeIdentity := func(email string) identity.Identity {
			id, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
			assert.Loosely(t, err, should.BeNil)
			return id
		}

		const tEmail = "t@example.org"
		ct.AddMember(tEmail, "googlers")
		ct.AddMember(tEmail, "partners")

		const lProject = "chromium"
		cg := &cfgpb.ConfigGroup{Name: "infra"}
		cfg := &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{cg}}
		prjcfgtest.Create(ctx, lProject, cfg)

		const gHost = "x-review.example.com"
		rid := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))
		clIDs := common.MakeCLIDs(1)
		rcls := []*run.RunCL{
			{
				ID:  1,
				Run: datastore.MakeKey(ctx, common.RunKind, string(rid)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
						Host: gHost,
					}},
				},
			},
		}
		assert.Loosely(t, datastore.Put(ctx, rcls), should.BeNil)

		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			&gerritpb.EmailInfo{Email: tEmail},
		})

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

		qm := NewManager(ct.GFactory())

		t.Run("WritePolicy() with config groups but no run limit", func(t *ftt.Test) {
			pid, err := qm.WritePolicy(ctx, lProject)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pid, should.BeNil)
		})

		t.Run("WritePolicy() with run limit values", func(t *ftt.Test) {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pid, should.Resemble(&quotapb.PolicyConfigID{
				AppId:   "cv",
				Realm:   "chromium",
				Version: pid.Version,
			}))
		})

		t.Run("WritePolicy() with run limit defaults", func(t *ftt.Test) {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
			)
			cg.UserLimitDefault = genUserLimit("chromies-limit", 5, []string{"group:chromies", "group:chrome-infra"})
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pid, should.Resemble(&quotapb.PolicyConfigID{
				AppId:   "cv",
				Realm:   "chromium",
				Version: pid.Version,
			}))
		})

		t.Run("WritePolicy() with run limit set to unlimited", func(t *ftt.Test) {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 0, []string{"group:googlers"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pid, should.Resemble(&quotapb.PolicyConfigID{
				AppId:   "cv",
				Realm:   "chromium",
				Version: pid.Version,
			}))
		})

		t.Run("findRunLimit() returns first valid user_limit", func(t *ftt.Test) {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, err := qm.findRunLimit(ctx, r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(googlerLimit))
		})

		t.Run("findRunLimit() works with user entry in principals", func(t *ftt.Test) {
			exampleLimit := genUserLimit("example-limit", 10, []string{"group:chromies", "user:t@example.org"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				exampleLimit,
				genUserLimit("googlers-limit", 5, []string{"group:googlers"}),
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}
			res, err := qm.findRunLimit(ctx, r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(exampleLimit))
		})

		t.Run("findRunLimit() returns default user_limit if no valid user_limit is found", func(t *ftt.Test) {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:invalid"}),
				genUserLimit("partners-limit", 10, []string{"group:invalid"}),
			)
			cg.UserLimitDefault = genUserLimit("", 5, nil)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, err := qm.findRunLimit(ctx, r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(genUserLimit("default", 5, nil))) // default name is overriden.
		})

		t.Run("findRunLimit() returns nil when no valid policy is found", func(t *ftt.Test) {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"group:invalid"}),
				genUserLimit("partners-limit", 10, []string{"group:invalid"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, err := qm.findRunLimit(ctx, r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("DebitRunQuota() debits quota for a given run state", func(t *ftt.Test) {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, userLimit, err := qm.DebitRunQuota(ctx, r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			}))
			assert.Loosely(t, ct.TSMonSentValue(
				ctx,
				metrics.Internal.QuotaOp,
				lProject,
				"infra",
				"googlers-limit",
				"runs",
				"debit",
				"SUCCESS",
			), should.Equal(1))
		})

		t.Run("CreditRunQuota() credits quota for a given run state", func(t *ftt.Test) {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, userLimit, err := qm.CreditRunQuota(ctx, r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:    5,
				AccountStatus: quotapb.OpResult_CREATED,
			}))
			assert.Loosely(t, ct.TSMonSentValue(
				ctx,
				metrics.Internal.QuotaOp,
				lProject,
				"infra",
				"googlers-limit",
				"runs",
				"credit",
				"SUCCESS",
			), should.Equal(1))
		})

		t.Run("runQuotaOp() updates the same account on multiple ops", func(t *ftt.Test) {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, userLimit, err := qm.runQuotaOp(ctx, r, "foo1", -1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			}))
			assert.Loosely(t, ct.TSMonSentValue(
				ctx,
				metrics.Internal.QuotaOp,
				lProject,
				"infra",
				"googlers-limit",
				"runs",
				"foo1",
				"SUCCESS",
			), should.Equal(1))

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo2", -2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:              2,
				PreviousBalance:         4,
				PreviousBalanceAdjusted: 4,
				AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
			}))
			assert.Loosely(t, ct.TSMonSentValue(
				ctx,
				metrics.Internal.QuotaOp,
				lProject,
				"infra",
				"googlers-limit",
				"runs",
				"foo2",
				"SUCCESS",
			), should.Equal(1))
		})

		t.Run("runQuotaOp() respects unlimited policy", func(t *ftt.Test) {
			googlerLimit := genUserLimit("googlers-limit", 0, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits, googlerLimit)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, userLimit, err := qm.runQuotaOp(ctx, r, "", -1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:    -1,
				AccountStatus: quotapb.OpResult_CREATED,
			}))
		})

		t.Run("runQuotaOp() bound checks", func(t *ftt.Test) {
			googlerLimit := genUserLimit("googlers-limit", 1, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits, googlerLimit)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			t.Run("quota underflow", func(t *ftt.Test) {
				res, userLimit, err := qm.runQuotaOp(ctx, r, "debit", -2)
				assert.Loosely(t, err, should.Equal(quota.ErrQuotaApply))
				assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
				assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
					AccountStatus: quotapb.OpResult_CREATED,
					Status:        quotapb.OpResult_ERR_UNDERFLOW,
				}))
				assert.Loosely(t, ct.TSMonSentValue(
					ctx,
					metrics.Internal.QuotaOp,
					lProject,
					"infra",
					"googlers-limit",
					"runs",
					"debit",
					"ERR_UNDERFLOW",
				), should.Equal(1))
			})

			t.Run("quota overflow", func(t *ftt.Test) {
				// overflow doesn't err but gets capped.
				res, userLimit, err := qm.runQuotaOp(ctx, r, "credit", 10)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
				assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
					AccountStatus: quotapb.OpResult_CREATED,
					NewBalance:    1,
				}))
				assert.Loosely(t, ct.TSMonSentValue(
					ctx,
					metrics.Internal.QuotaOp,
					lProject,
					"infra",
					"googlers-limit",
					"runs",
					"credit",
					"SUCCESS",
				), should.Equal(1))
			})
		})

		t.Run("runQuotaOp() on policy change", func(t *ftt.Test) {
			chromiesLimit := genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"})
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			partnerLimit := genUserLimit("partners-limit", 2, []string{"group:partners"})
			cg.UserLimits = append(cg.UserLimits, chromiesLimit, googlerLimit, partnerLimit)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, userLimit, err := qm.runQuotaOp(ctx, r, "", -1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			}))

			t.Run("decrease in quota allowance results in underflow", func(t *ftt.Test) {
				// Update policy
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       makeIdentity(tEmail),
					IdentityGroups: []string{"partners"},
				})

				// This is not a real scenario within CV but just checks
				// extreme examples.
				res, userLimit, err = qm.runQuotaOp(ctx, r, "", -2)
				assert.Loosely(t, err, should.Equal(quota.ErrQuotaApply))
				assert.Loosely(t, userLimit, should.Resemble(partnerLimit))
				assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
					PreviousBalance:         4,
					PreviousBalanceAdjusted: 1,
					Status:                  quotapb.OpResult_ERR_UNDERFLOW,
				}))
			})

			t.Run("decrease in quota allowance within bounds", func(t *ftt.Test) {
				// Update policy
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       makeIdentity(tEmail),
					IdentityGroups: []string{"partners"},
				})

				res, userLimit, err = qm.runQuotaOp(ctx, r, "", -1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, userLimit, should.Resemble(partnerLimit))
				assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
					NewBalance:              0,
					PreviousBalance:         4,
					PreviousBalanceAdjusted: 1,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				}))
			})

			t.Run("increase in quota allowance", func(t *ftt.Test) {
				// Update policy
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       makeIdentity(tEmail),
					IdentityGroups: []string{"chromies"},
				})

				res, userLimit, err = qm.runQuotaOp(ctx, r, "", -1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, userLimit, should.Resemble(chromiesLimit))
				assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
					NewBalance:              8,
					PreviousBalance:         4,
					PreviousBalanceAdjusted: 9,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				}))
			})

			t.Run("increase in quota allowance in the overflow case is bounded by the new limit", func(t *ftt.Test) {
				// Update policy
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:       makeIdentity(tEmail),
					IdentityGroups: []string{"chromies"},
				})

				// This is not a real scenario within CV but just checks
				// extreme examples.
				res, userLimit, err = qm.runQuotaOp(ctx, r, "", 2)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, userLimit, should.Resemble(chromiesLimit))
				assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
					NewBalance:              10,
					PreviousBalance:         4,
					PreviousBalanceAdjusted: 9,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				}))
			})
		})

		t.Run("runQuotaOp() is idempotent", func(t *ftt.Test) {
			googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"group:chromies", "group:chrome-infra"}),
				googlerLimit,
				genUserLimit("partners-limit", 10, []string{"group:partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			res, userLimit, err := qm.runQuotaOp(ctx, r, "foo", -1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			}))

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo", -1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			}))

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo2", -2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:              2,
				PreviousBalance:         4,
				PreviousBalanceAdjusted: 4,
				AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
			}))

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo2", -2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, userLimit, should.Resemble(googlerLimit))
			assert.Loosely(t, res, should.Resemble(&quotapb.OpResult{
				NewBalance:              2,
				PreviousBalance:         4,
				PreviousBalanceAdjusted: 4,
				AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
			}))
		})

		t.Run("RunQuotaAccountID() hashes emailID", func(t *ftt.Test) {
			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			emailHash := md5.Sum([]byte(tEmail))

			assert.Loosely(t, qm.RunQuotaAccountID(r), should.Resemble(&quotapb.AccountID{
				AppId:        "cv",
				Realm:        "chromium",
				Namespace:    "infra",
				Name:         hex.EncodeToString(emailHash[:]),
				ResourceType: "runs",
			}))
		})
	})
}
