// Copyright 2026 The LUCI Authors.
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

package e2e

import (
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/redisconn"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/validation"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	_ "go.chromium.org/luci/server/quota/quotatestmonkeypatch"
)

func TestQuotaBlockedRunRescuedByOtherVoter(t *testing.T) {
	t.Parallel()

	ftt.Run("PENDING Run blocked on run quota is started by a later CQ+2 from a voter with quota", t, func(t *ftt.Test) {
		/////////////////////////    Setup   ////////////////////////////////
		ct := Test{}
		ctx := ct.SetUp(t)

		// Run quota is backed by Redis.
		s, err := miniredis.Run()
		assert.NoErr(t, err)
		t.Cleanup(s.Close)
		s.SetTime(ct.Clock.Now())
		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) { return redis.Dial("tcp", s.Addr()) },
		})

		const lProject = "infra"
		const gHost = "g-review"
		const gRepo = "re/po"
		const gRef = "refs/heads/main"
		const gChangeFirst = 33
		const gChangeBlocked = 44
		const user = "user-1"
		// The numeric suffix gives the bot a distinct Gerrit account ID.
		const bot = "auto-submit-bot-99"
		userEmail := gf.U(user).GetEmail()
		botEmail := gf.U(bot).GetEmail()

		// A required builder keeps Runs RUNNING, so that the user's quota is not
		// credited back during the test.
		cfg := MakeCfgSingular("cg0", gHost, gRepo, gRef, &cfgpb.Verifiers_Tryjob_Builder{
			Host: buildbucketHost,
			Name: fmt.Sprintf("%s/try/test-builder", lProject),
		})
		cfg.ConfigGroups[0].UserLimits = []*cfgpb.UserLimit{
			{
				Name:       "user-limit",
				Principals: []string{"user:" + userEmail},
				Run: &cfgpb.UserLimit_Run{MaxActive: &cfgpb.UserLimit_Limit{
					Limit: &cfgpb.UserLimit_Limit_Value{Value: 1},
				}},
			},
			{
				Name:       "auto-submit-limit",
				Principals: []string{"user:" + botEmail},
				Run: &cfgpb.UserLimit_Run{MaxActive: &cfgpb.UserLimit_Limit{
					Limit: &cfgpb.UserLimit_Limit_Value{Value: 10},
				}},
			},
		}
		ct.BuildbucketFake.EnsureBuilders(cfg)
		cfg.CqStatusHost = validation.CQStatusHostPublic
		prjcfgtest.Create(ctx, lProject, cfg)
		ct.AddCommitter(user)
		ct.AddCommitter(bot)

		tStart := ct.Clock.Now()
		ct.GFake.AddFrom(gf.WithCIs(gHost, gf.ACLRestricted(lProject),
			gf.CI(gChangeFirst, gf.Project(gRepo), gf.Ref(gRef), gf.Owner(user),
				gf.Updated(tStart), gf.CQ(+2, tStart, gf.U(user)), gf.Approve()),
			gf.CI(gChangeBlocked, gf.Project(gRepo), gf.Ref(gRef), gf.Owner(user),
				gf.Updated(tStart), gf.CQ(+2, tStart, gf.U(user)), gf.Approve()),
		))

		/////////////////////////    Run CV   ////////////////////////////////
		ct.LogPhase(ctx, "CV starts one Run and leaves the other PENDING on the user's run quota")
		assert.NoErr(t, ct.PMNotifier.UpdateConfig(ctx, lProject))
		var first, blocked *run.Run
		blockedChange := gChangeBlocked
		ct.RunUntil(ctx, func() bool {
			first = ct.LatestRunWithGerritCL(ctx, gHost, gChangeFirst)
			blocked = ct.LatestRunWithGerritCL(ctx, gHost, gChangeBlocked)
			blockedChange = gChangeBlocked
			if first == nil || blocked == nil {
				return false
			}
			// Either may win the quota; normalize so that `first` is RUNNING.
			if first.Status == run.Status_PENDING && blocked.Status == run.Status_RUNNING {
				first, blocked = blocked, first
				blockedChange = gChangeFirst
			}
			return first.Status == run.Status_RUNNING && blocked.Status == run.Status_PENDING &&
				blocked.QuotaExhaustionMsgLongOpRequested
		})
		assert.Loosely(t, blocked.BilledTo.Email(), should.Equal(userEmail))

		ct.LogPhase(ctx, "Auto-submit bot votes CQ+2 after the Run is already PENDING")
		ct.GFake.MutateChange(gHost, blockedChange, func(c *gf.Change) {
			gf.CQ(+2, ct.Clock.Now(), gf.U(bot))(c.Info)
			gf.Updated(ct.Clock.Now())(c.Info)
		})

		ct.LogPhase(ctx, "CV notices the new vote and starts the Run, billed to the bot")
		ct.RunUntil(ctx, func() bool {
			blocked = ct.LoadRun(ctx, blocked.ID)
			return blocked.Status == run.Status_RUNNING
		})

		/////////////////////////    Verify    ////////////////////////////////
		assert.Loosely(t, blocked.BilledTo.Email(), should.Equal(botEmail))
		// The user's other Run is still running, so the blocked Run was not
		// started by the user's quota being credited back.
		assert.Loosely(t, ct.LoadRun(ctx, first.ID).Status, should.Equal(run.Status_RUNNING))
		assert.Loosely(t, ct.LoadRun(ctx, first.ID).BilledTo.Email(), should.Equal(userEmail))
	})
}
