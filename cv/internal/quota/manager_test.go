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
	"google.golang.org/genproto/googleapis/type/dayofweek"
	"google.golang.org/genproto/googleapis/type/timeofday"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

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
		assert.NoErr(t, err)
		defer s.Close()
		s.SetTime(clock.Now(ctx))

		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

		makeIdentity := func(email string) identity.Identity {
			id, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
			assert.NoErr(t, err)
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
		assert.NoErr(t, datastore.Put(ctx, rcls))

		ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
			{Email: tEmail},
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
			assert.NoErr(t, err)
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

			assert.NoErr(t, err)
			assert.That(t, pid, should.Match(&quotapb.PolicyConfigID{
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

			assert.NoErr(t, err)
			assert.That(t, pid, should.Match(&quotapb.PolicyConfigID{
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

			assert.NoErr(t, err)
			assert.That(t, pid, should.Match(&quotapb.PolicyConfigID{
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
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(googlerLimit))
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
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(exampleLimit))
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
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(genUserLimit("default", 5, nil))) // default name is overridden.
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
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
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
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
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
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
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
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
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
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
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
				assert.That(t, userLimit, should.Match(googlerLimit))
				assert.That(t, res, should.Match(&quotapb.OpResult{
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
				assert.NoErr(t, err)
				assert.That(t, userLimit, should.Match(googlerLimit))
				assert.That(t, res, should.Match(&quotapb.OpResult{
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
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
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
				assert.That(t, userLimit, should.Match(partnerLimit))
				assert.That(t, res, should.Match(&quotapb.OpResult{
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
				assert.NoErr(t, err)
				assert.That(t, userLimit, should.Match(partnerLimit))
				assert.That(t, res, should.Match(&quotapb.OpResult{
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
				assert.NoErr(t, err)
				assert.That(t, userLimit, should.Match(chromiesLimit))
				assert.That(t, res, should.Match(&quotapb.OpResult{
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
				assert.NoErr(t, err)
				assert.That(t, userLimit, should.Match(chromiesLimit))
				assert.That(t, res, should.Match(&quotapb.OpResult{
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
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			}))

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo", -1)
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
				NewBalance:    4,
				AccountStatus: quotapb.OpResult_CREATED,
			}))

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo2", -2)
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
				NewBalance:              2,
				PreviousBalance:         4,
				PreviousBalanceAdjusted: 4,
				AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
			}))

			res, userLimit, err = qm.runQuotaOp(ctx, r, "foo2", -2)
			assert.NoErr(t, err)
			assert.That(t, userLimit, should.Match(googlerLimit))
			assert.That(t, res, should.Match(&quotapb.OpResult{
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

			assert.That(t, qm.RunQuotaAccountID(r), should.Match(&quotapb.AccountID{
				AppId:        "cv",
				Realm:        "chromium",
				Namespace:    "infra",
				Name:         hex.EncodeToString(emailHash[:]),
				ResourceType: "runs",
			}))
		})

		t.Run("user limit overrides", func(t *ftt.Test) {
			laLoc, err := time.LoadLocation("America/Los_Angeles")
			assert.NoErr(t, err)

			// Base limit (off-peak): 12
			// Override 0 (Mon-Fri 08:00-20:00 America/Los_Angeles): 6
			defaultLimit := genUserLimit("default_user_limit", 12, nil)
			defaultLimit.Run.ReachLimitMsg = "Base limit message."
			defaultLimit.Overrides = []*cfgpb.UserLimit_Override{
				{
					Schedule: &cfgpb.UserLimit_Override_Schedule{
						Timezone: "America/Los_Angeles",
						Days: []dayofweek.DayOfWeek{
							dayofweek.DayOfWeek_MONDAY,
							dayofweek.DayOfWeek_TUESDAY,
							dayofweek.DayOfWeek_WEDNESDAY,
							dayofweek.DayOfWeek_THURSDAY,
							dayofweek.DayOfWeek_FRIDAY,
						},
						StartTime: &timeofday.TimeOfDay{Hours: 8},
						Duration:  durationpb.New(12 * time.Hour),
					},
					Run: &cfgpb.UserLimit_Run{
						MaxActive: &cfgpb.UserLimit_Limit{
							Limit: &cfgpb.UserLimit_Limit_Value{Value: 6},
						},
					},
				},
			}
			cg.UserLimitDefault = defaultLimit
			prjcfgtest.Update(ctx, lProject, cfg)

			r := &run.Run{
				ID:            rid,
				ConfigGroupID: prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra"),
				BilledTo:      makeIdentity(tEmail),
				CLs:           clIDs,
			}

			t.Run("makeRunQuotaPolicies() registers a policy per override", func(t *ftt.Test) {
				cg.UserLimits = []*cfgpb.UserLimit{genUserLimit("googlers-limit", 5, []string{"group:googlers"})}
				unlimitedOverride := proto.CloneOf(defaultLimit.Overrides[0])
				unlimitedOverride.Run.MaxActive.Limit = &cfgpb.UserLimit_Limit_Unlimited{Unlimited: true}
				cg.UserLimits[0].Overrides = []*cfgpb.UserLimit_Override{defaultLimit.Overrides[0], unlimitedOverride}

				limits := map[string]uint64{}
				var names []string
				for _, p := range makeRunQuotaPolicies(lProject, []*prjcfg.ConfigGroup{{Content: cg}}) {
					names = append(names, p.GetKey().GetName())
					limits[p.GetKey().GetName()] = p.GetPolicy().GetLimit()
				}
				assert.That(t, names, should.Match([]string{
					"googlers-limit",
					"googlers-limit:schedule_override:0",
					"googlers-limit:schedule_override:1",
					"default",
					"default:schedule_override:0",
				}))
				assert.That(t, limits, should.Match(map[string]uint64{
					"googlers-limit":                     5,
					"googlers-limit:schedule_override:0": 6,
					// Unlimited is registered as 0 and enforced via IGNORE_POLICY_BOUNDS.
					"googlers-limit:schedule_override:1": 0,
					"default":                            12,
					"default:schedule_override:0":        6,
				}))
				// The config itself isn't mutated by renaming the default limit.
				assert.That(t, cg.UserLimitDefault.GetName(), should.Equal("default_user_limit"))
			})

			t.Run("transitions dynamically between peak and off-peak", func(t *ftt.Test) {
				// Wednesday 10:00 AM PT (peak -> limit 6, policy "default:schedule_override:0").
				wedPeak := time.Date(2026, time.October, 21, 10, 0, 0, 0, laLoc)
				ct.Clock.Set(wedPeak.UTC())
				s.SetTime(clock.Now(ctx))

				res, ul, err := qm.runQuotaOp(ctx, r, "peak-debit-6", -6)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default:schedule_override:0"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(6)))
				// Inherits ReachLimitMsg from parent UserLimit when Override.Run.ReachLimitMsg is empty.
				assert.That(t, ul.GetRun().GetReachLimitMsg(), should.Equal("Base limit message."))
				assert.That(t, res, should.Match(&quotapb.OpResult{
					NewBalance:    0,
					AccountStatus: quotapb.OpResult_CREATED,
				}))

				// 7th run during peak underflows.
				res, ul, err = qm.runQuotaOp(ctx, r, "peak-debit-7", -1)
				assert.Loosely(t, err, should.Equal(quota.ErrQuotaApply))
				assert.That(t, res.GetStatus(), should.Equal(quotapb.OpResult_ERR_UNDERFLOW))

				// Advance to Wednesday 8:00 PM PT (exact off-peak boundary -> limit 12, policy "default").
				wedOffPeak := time.Date(2026, time.October, 21, 20, 0, 0, 0, laLoc)
				ct.Clock.Set(wedOffPeak.UTC())
				s.SetTime(clock.Now(ctx))

				// 7th run now succeeds! Balance adjusts by +(12-6) = +6 before debiting -1 -> NewBalance = 5.
				res, ul, err = qm.runQuotaOp(ctx, r, "offpeak-debit-7", -1)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(12)))
				assert.That(t, res, should.Match(&quotapb.OpResult{
					NewBalance:              5,
					PreviousBalance:         0,
					PreviousBalanceAdjusted: 6,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				}))

				// Advance to Thursday 8:00 AM PT (exact peak start -> limit 6) while 7 runs are still active.
				thuPeak := time.Date(2026, time.October, 22, 8, 0, 0, 0, laLoc)
				ct.Clock.Set(thuPeak.UTC())
				s.SetTime(clock.Now(ctx))

				// Starting an 8th run during peak underflows (adjusted balance is 5 + (6-12) = -1).
				res, ul, err = qm.runQuotaOp(ctx, r, "thu-peak-debit-8", -1)
				assert.Loosely(t, err, should.Equal(quota.ErrQuotaApply))
				assert.That(t, ul.GetName(), should.Equal("default:schedule_override:0"))
				assert.That(t, res, should.Match(&quotapb.OpResult{
					PreviousBalance:         5,
					PreviousBalanceAdjusted: -1,
					Status:                  quotapb.OpResult_ERR_UNDERFLOW,
				}))

				// Crediting 2 finished runs during peak brings balance from -1 -> +1.
				res, ul, err = qm.runQuotaOp(ctx, r, "thu-peak-credit-2", 2)
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&quotapb.OpResult{
					NewBalance:              1,
					PreviousBalance:         5,
					PreviousBalanceAdjusted: -1,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				}))

				// Weekend at 12:00 PM PT (Saturday -> off-peak limit 12).
				satNoon := time.Date(2026, time.October, 24, 12, 0, 0, 0, laLoc)
				ct.Clock.Set(satNoon.UTC())
				s.SetTime(clock.Now(ctx))
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(12)))
				// The returned limit has no overrides, and the config isn't mutated.
				assert.Loosely(t, ul.GetOverrides(), should.BeEmpty)
				assert.That(t, cg.UserLimitDefault.GetName(), should.Equal("default_user_limit"))
				assert.That(t, len(cg.UserLimitDefault.GetOverrides()), should.Equal(1))
			})

			t.Run("overnight schedule wrapping past midnight", func(t *ftt.Test) {
				// Overnight schedule: Mon-Fri 22:00 to 06:00 America/Los_Angeles -> limit 12, base 6.
				cg.UserLimitDefault = genUserLimit("default_user_limit", 6, nil)
				cg.UserLimitDefault.Overrides = []*cfgpb.UserLimit_Override{
					{
						Schedule: &cfgpb.UserLimit_Override_Schedule{
							Timezone: "America/Los_Angeles",
							Days: []dayofweek.DayOfWeek{
								dayofweek.DayOfWeek_MONDAY,
								dayofweek.DayOfWeek_TUESDAY,
								dayofweek.DayOfWeek_WEDNESDAY,
								dayofweek.DayOfWeek_THURSDAY,
								dayofweek.DayOfWeek_FRIDAY,
							},
							StartTime: &timeofday.TimeOfDay{Hours: 22},
							Duration:  durationpb.New(8 * time.Hour),
						},
						Run: &cfgpb.UserLimit_Run{
							MaxActive: &cfgpb.UserLimit_Limit{
								Limit: &cfgpb.UserLimit_Limit_Value{Value: 12},
							},
							ReachLimitMsg: "Overnight message.",
						},
					},
				}
				prjcfgtest.Update(ctx, lProject, cfg)
				r.ConfigGroupID = prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra")

				// Monday 03:00 AM PT (started Sunday 22:00, Sunday is NOT in Mon-Fri -> inactive, base limit 6!).
				monEarly := time.Date(2026, time.October, 19, 3, 0, 0, 0, laLoc)
				ct.Clock.Set(monEarly.UTC())
				ul, err := qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(6)))

				// Tuesday 03:00 AM PT (started Monday 22:00, Monday IS in Mon-Fri -> active, limit 12!).
				tueEarly := time.Date(2026, time.October, 20, 3, 0, 0, 0, laLoc)
				ct.Clock.Set(tueEarly.UTC())
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default:schedule_override:0"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(12)))
				assert.That(t, ul.GetRun().GetReachLimitMsg(), should.Equal("Overnight message."))

				// Friday 23:00 PM PT (started Friday 22:00, Friday IS in Mon-Fri -> active, limit 12!).
				friLate := time.Date(2026, time.October, 23, 23, 0, 0, 0, laLoc)
				ct.Clock.Set(friLate.UTC())
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default:schedule_override:0"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(12)))

				// Saturday 03:00 AM PT (started Friday 22:00, Friday IS in Mon-Fri -> active, limit 12!).
				satEarly := time.Date(2026, time.October, 24, 3, 0, 0, 0, laLoc)
				ct.Clock.Set(satEarly.UTC())
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default:schedule_override:0"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(12)))

				// Saturday 06:00 AM PT (exact window end boundary -> inactive, base limit 6!).
				satEnd := time.Date(2026, time.October, 24, 6, 0, 0, 0, laLoc)
				ct.Clock.Set(satEnd.UTC())
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(6)))

				// Saturday 23:00 PM PT (started Saturday 22:00, Saturday is NOT in Mon-Fri -> inactive, base limit 6!).
				satLate := time.Date(2026, time.October, 24, 23, 0, 0, 0, laLoc)
				ct.Clock.Set(satLate.UTC())
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("default"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(6)))
			})

			t.Run("named user_limits with multiple overrides and boundary timestamps", func(t *ftt.Test) {
				googlerLimit := genUserLimit("googlers-limit", 5, []string{"group:googlers"})
				googlerLimit.Overrides = []*cfgpb.UserLimit_Override{
					// Override 0: Narrower peak override (09:00-17:00 -> limit 2).
					{
						Schedule: &cfgpb.UserLimit_Override_Schedule{
							Timezone:  "America/Los_Angeles",
							Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_WEDNESDAY},
							StartTime: &timeofday.TimeOfDay{Hours: 9},
							Duration:  durationpb.New(8 * time.Hour),
						},
						Run: &cfgpb.UserLimit_Run{
							MaxActive: &cfgpb.UserLimit_Limit{
								Limit: &cfgpb.UserLimit_Limit_Value{Value: 2},
							},
						},
					},
					// Override 1: Broader weekday override (08:00-20:00 -> limit 4).
					{
						Schedule: &cfgpb.UserLimit_Override_Schedule{
							Timezone:  "America/Los_Angeles",
							Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_WEDNESDAY},
							StartTime: &timeofday.TimeOfDay{Hours: 8},
							Duration:  durationpb.New(12 * time.Hour),
						},
						Run: &cfgpb.UserLimit_Run{
							MaxActive: &cfgpb.UserLimit_Limit{
								Limit: &cfgpb.UserLimit_Limit_Value{Value: 4},
							},
						},
					},
					// Override 2: Full-day weekend override (00:00 + 24h -> unlimited).
					{
						Schedule: &cfgpb.UserLimit_Override_Schedule{
							Timezone:  "America/Los_Angeles",
							Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_SATURDAY, dayofweek.DayOfWeek_SUNDAY},
							StartTime: &timeofday.TimeOfDay{Hours: 0},
							Duration:  durationpb.New(24 * time.Hour),
						},
						Run: &cfgpb.UserLimit_Run{
							MaxActive: &cfgpb.UserLimit_Limit{
								Limit: &cfgpb.UserLimit_Limit_Unlimited{Unlimited: true},
							},
						},
					},
				}
				cg.UserLimits = []*cfgpb.UserLimit{googlerLimit}
				cg.UserLimitDefault = nil
				prjcfgtest.Update(ctx, lProject, cfg)
				r.ConfigGroupID = prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra")

				// 08:59:59 PT on Wednesday -> Override 0 is inactive, Override 1 is active (limit 4, "googlers-limit:schedule_override:1").
				ct.Clock.Set(time.Date(2026, time.October, 21, 8, 59, 59, 0, laLoc).UTC())
				ul, err := qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit:schedule_override:1"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(4)))

				// 09:00:00 PT on Wednesday -> Override 0 wins over Override 1 (limit 2, "googlers-limit:schedule_override:0").
				ct.Clock.Set(time.Date(2026, time.October, 21, 9, 0, 0, 0, laLoc).UTC())
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit:schedule_override:0"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(2)))

				// 16:59:59 PT on Wednesday -> Override 0 still active (limit 2).
				ct.Clock.Set(time.Date(2026, time.October, 21, 16, 59, 59, 0, laLoc).UTC())
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit:schedule_override:0"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(2)))

				// 17:00:00 PT on Wednesday -> Override 0 ends (exclusive), Override 1 is active (limit 4).
				ct.Clock.Set(time.Date(2026, time.October, 21, 17, 0, 0, 0, laLoc).UTC())
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit:schedule_override:1"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(4)))

				// Saturday 23:59:59 PT -> Full-day Override 2 is active (unlimited).
				ct.Clock.Set(time.Date(2026, time.October, 24, 23, 59, 59, 0, laLoc).UTC())
				s.SetTime(clock.Now(ctx))
				res, ul, err := qm.runQuotaOp(ctx, r, "sat-unlimited-debit", -100)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit:schedule_override:2"))
				assert.That(t, ul.GetRun().GetMaxActive().GetUnlimited(), should.BeTrue)
				assert.That(t, res, should.Match(&quotapb.OpResult{
					NewBalance:    -100,
					AccountStatus: quotapb.OpResult_CREATED,
				}))
			})

			t.Run("unlimited base limit overridden by finite override and DST transitions", func(t *ftt.Test) {
				// Base limit: unlimited (0).
				// Override 0: Mon-Fri 09:00-17:00 America/Los_Angeles -> finite limit 2.
				googlerLimit := genUserLimit("googlers-limit", 0, []string{"user:t@example.org"})
				googlerLimit.Overrides = []*cfgpb.UserLimit_Override{
					{
						Schedule: &cfgpb.UserLimit_Override_Schedule{
							Timezone: "America/Los_Angeles",
							Days: []dayofweek.DayOfWeek{
								dayofweek.DayOfWeek_MONDAY,
								dayofweek.DayOfWeek_TUESDAY,
								dayofweek.DayOfWeek_WEDNESDAY,
								dayofweek.DayOfWeek_THURSDAY,
								dayofweek.DayOfWeek_FRIDAY,
							},
							StartTime: &timeofday.TimeOfDay{Hours: 9},
							Duration:  durationpb.New(8 * time.Hour),
						},
						Run: &cfgpb.UserLimit_Run{
							MaxActive: &cfgpb.UserLimit_Limit{
								Limit: &cfgpb.UserLimit_Limit_Value{Value: 2},
							},
						},
					},
				}
				cg.UserLimits = []*cfgpb.UserLimit{googlerLimit}
				cg.UserLimitDefault = nil
				prjcfgtest.Update(ctx, lProject, cfg)
				r.ConfigGroupID = prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra")

				// Summer PDT (UTC-7): Wednesday July 15, 2026 at 16:00 UTC == 09:00 PDT -> active (limit 2).
				summerPDT := time.Date(2026, time.July, 15, 16, 0, 0, 0, time.UTC)
				ct.Clock.Set(summerPDT)
				s.SetTime(clock.Now(ctx))
				res, ul, err := qm.runQuotaOp(ctx, r, "pdt-debit-3", -3)
				assert.Loosely(t, err, should.Equal(quota.ErrQuotaApply))
				assert.That(t, ul.GetName(), should.Equal("googlers-limit:schedule_override:0"))
				assert.That(t, res.GetStatus(), should.Equal(quotapb.OpResult_ERR_UNDERFLOW))

				// Winter PST (UTC-8): Wednesday November 18, 2026 at 16:00 UTC == 08:00 PST -> inactive (base unlimited!).
				winterPST8AM := time.Date(2026, time.November, 18, 16, 0, 0, 0, time.UTC)
				ct.Clock.Set(winterPST8AM)
				s.SetTime(clock.Now(ctx))
				res, ul, err = qm.runQuotaOp(ctx, r, "pst-8am-debit-3", -3)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit"))
				assert.That(t, ul.GetRun().GetMaxActive().GetUnlimited(), should.BeTrue)
				assert.That(t, res.GetStatus(), should.Equal(quotapb.OpResult_SUCCESS))

				// Winter PST (UTC-8): Wednesday November 18, 2026 at 17:00 UTC == 09:00 PST -> active (limit 2).
				winterPST9AM := time.Date(2026, time.November, 18, 17, 0, 0, 0, time.UTC)
				ct.Clock.Set(winterPST9AM)
				ul, err = qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit:schedule_override:0"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(2)))
			})

			t.Run("balance adjusts across unlimited base and finite override", func(t *ftt.Test) {
				// Base limit: unlimited (registered as 0).
				// Override 0: Wednesday 09:00-17:00 America/Los_Angeles -> limit 2.
				googlerLimit := genUserLimit("googlers-limit", 0, []string{"user:t@example.org"})
				googlerLimit.Overrides = []*cfgpb.UserLimit_Override{
					{
						Schedule: &cfgpb.UserLimit_Override_Schedule{
							Timezone:  "America/Los_Angeles",
							Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_WEDNESDAY},
							StartTime: &timeofday.TimeOfDay{Hours: 9},
							Duration:  durationpb.New(8 * time.Hour),
						},
						Run: &cfgpb.UserLimit_Run{
							MaxActive: &cfgpb.UserLimit_Limit{
								Limit: &cfgpb.UserLimit_Limit_Value{Value: 2},
							},
						},
					},
				}
				cg.UserLimits = []*cfgpb.UserLimit{googlerLimit}
				cg.UserLimitDefault = nil
				prjcfgtest.Update(ctx, lProject, cfg)
				r.ConfigGroupID = prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra")
				setTime := func(hour int) {
					ct.Clock.Set(time.Date(2026, time.October, 21, hour, 0, 0, 0, laLoc).UTC())
					s.SetTime(clock.Now(ctx))
				}

				// 08:00 (base, unlimited): start 3 runs.
				setTime(8)
				res, ul, err := qm.runQuotaOp(ctx, r, "base-debit-3", -3)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit"))
				assert.That(t, res.GetNewBalance(), should.Equal(int64(-3)))

				// 09:00 (override, limit 2): 3 runs are active, so the adjusted
				// balance is -3 + (2 - 0) = -1 and starting another run fails.
				setTime(9)
				res, ul, err = qm.runQuotaOp(ctx, r, "override-debit-1", -1)
				assert.Loosely(t, err, should.Equal(quota.ErrQuotaApply))
				assert.That(t, ul.GetName(), should.Equal("googlers-limit:schedule_override:0"))
				assert.That(t, res, should.Match(&quotapb.OpResult{
					PreviousBalance:         -3,
					PreviousBalanceAdjusted: -1,
					Status:                  quotapb.OpResult_ERR_UNDERFLOW,
				}))

				// A run that was debited under the base limit ends under the override.
				res, _, err = qm.runQuotaOp(ctx, r, "override-credit-1", 1)
				assert.NoErr(t, err)
				assert.That(t, res, should.Match(&quotapb.OpResult{
					NewBalance:              0,
					PreviousBalance:         -3,
					PreviousBalanceAdjusted: -1,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				}))

				// 17:00 (back to base, unlimited): a run that was active under the
				// override ends under the base limit. The adjusted balance is
				// 0 + (0 - 2) = -2, i.e. 2 active runs, and becomes -1 after the credit.
				setTime(17)
				res, ul, err = qm.runQuotaOp(ctx, r, "base-credit-1", 1)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit"))
				assert.That(t, res, should.Match(&quotapb.OpResult{
					NewBalance:              -1,
					PreviousBalance:         0,
					PreviousBalanceAdjusted: -2,
					AccountStatus:           quotapb.OpResult_ALREADY_EXISTS,
				}))
			})

			t.Run("malformed override schedule is skipped", func(t *ftt.Test) {
				googlerLimit := genUserLimit("googlers-limit", 5, []string{"user:t@example.org"})
				googlerLimit.Overrides = []*cfgpb.UserLimit_Override{
					{
						Schedule: &cfgpb.UserLimit_Override_Schedule{
							Timezone:  "Invalid/Timezone",
							Days:      []dayofweek.DayOfWeek{dayofweek.DayOfWeek_WEDNESDAY},
							StartTime: &timeofday.TimeOfDay{Hours: 9},
							Duration:  durationpb.New(8 * time.Hour),
						},
						Run: &cfgpb.UserLimit_Run{
							MaxActive: &cfgpb.UserLimit_Limit{
								Limit: &cfgpb.UserLimit_Limit_Value{Value: 2},
							},
						},
					},
				}
				cg.UserLimits = []*cfgpb.UserLimit{googlerLimit}
				cg.UserLimitDefault = nil
				prjcfgtest.Update(ctx, lProject, cfg)
				r.ConfigGroupID = prjcfg.MakeConfigGroupID(prjcfg.MustComputeHash(cfg), "infra")

				ct.Clock.Set(time.Date(2026, time.October, 21, 10, 0, 0, 0, laLoc).UTC())
				ul, err := qm.findRunLimit(ctx, r)
				assert.NoErr(t, err)
				assert.That(t, ul.GetName(), should.Equal("googlers-limit"))
				assert.That(t, ul.GetRun().GetMaxActive().GetValue(), should.Equal(int64(5)))
				assert.Loosely(t, ul.GetOverrides(), should.BeEmpty)
			})
		})
	})
}
