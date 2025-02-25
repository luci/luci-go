// Copyright 2023 The LUCI Authors.
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

package postaction

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/quota/quotapb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/configs/validation"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestCreditQuotaOp(t *testing.T) {
	t.Parallel()

	ftt.Run("Do", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const (
			lProject  = "infra"
			userEmail = "user@example.com"
		)
		userIdentity := identity.Identity(fmt.Sprintf("%s:%s", identity.User, userEmail))

		cfg := cfgpb.Config{
			CqStatusHost: validation.CQStatusHostPublic,
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "test",
					UserLimits: []*cfgpb.UserLimit{
						{
							Name: "test-limit",
							Principals: []string{
								"user:" + userEmail,
							},
							Run: &cfgpb.UserLimit_Run{
								MaxActive: &cfgpb.UserLimit_Limit{
									Limit: &cfgpb.UserLimit_Limit_Value{
										Value: 1,
									},
								},
							},
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, lProject, &cfg)
		configGroupID := prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0]

		rm := &mockRM{}
		qm := &mockQM{quotaSpecified: true}
		executor := &Executor{
			Run: &run.Run{
				ID:            common.MakeRunID(lProject, clock.Now(ctx).Add(-1*time.Hour), 1, []byte("deadbeef")),
				Status:        run.Status_SUCCEEDED,
				BilledTo:      userIdentity,
				Mode:          run.FullRun,
				ConfigGroupID: configGroupID,
			},
			RM:                rm,
			QM:                qm,
			IsCancelRequested: func() bool { return false },
			Payload: &run.OngoingLongOps_Op_ExecutePostActionPayload{
				Name: CreditRunQuotaPostActionName,
				Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota_{
					CreditRunQuota: &run.OngoingLongOps_Op_ExecutePostActionPayload_CreditRunQuota{},
				},
			},
		}
		runCreateTime := clock.Now(ctx).UTC().Add(-1 * time.Minute)
		t.Run("credit quota and notify single pending run", func(t *ftt.Test) {
			r := &run.Run{
				ID:            common.MakeRunID(lProject, runCreateTime, 1, []byte("deadbeef")),
				Status:        run.Status_PENDING,
				BilledTo:      userIdentity,
				Mode:          run.FullRun,
				CreateTime:    runCreateTime,
				ConfigGroupID: configGroupID,
			}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			summary, err := executor.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, summary, should.Equal(fmt.Sprintf("notified next Run %q to start", r.ID)))
			assert.Loosely(t, qm.creditQuotaCalledWith, should.Match(common.RunIDs{executor.Run.ID}))
			assert.Loosely(t, rm.notifyStarted, should.Match(common.RunIDs{r.ID}))
		})
		t.Run("do not notify if quota is not specified", func(t *ftt.Test) {
			r := &run.Run{
				ID:            common.MakeRunID(lProject, runCreateTime, 1, []byte("deadbeef")),
				Status:        run.Status_PENDING,
				BilledTo:      userIdentity,
				Mode:          run.FullRun,
				CreateTime:    runCreateTime,
				ConfigGroupID: configGroupID,
			}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			qm.quotaSpecified = false
			summary, err := executor.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, summary, should.Equal(fmt.Sprintf("run quota limit is not specified for user %q", r.BilledTo.Email())))
			assert.Loosely(t, qm.creditQuotaCalledWith, should.Match(common.RunIDs{executor.Run.ID}))
			assert.Loosely(t, rm.notifyStarted, should.BeEmpty)
		})
		t.Run("do not notify pending run from different project", func(t *ftt.Test) {
			r := &run.Run{
				ID:            common.MakeRunID("another-proj", runCreateTime, 1, []byte("deadbeef")),
				Status:        run.Status_PENDING,
				BilledTo:      userIdentity,
				Mode:          run.FullRun,
				CreateTime:    runCreateTime,
				ConfigGroupID: configGroupID,
			}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			summary, err := executor.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, summary, should.BeEmpty)
			assert.Loosely(t, rm.notifyStarted, should.BeEmpty)
		})
		t.Run("do not notify pending run from different config group", func(t *ftt.Test) {
			r := &run.Run{
				ID:            common.MakeRunID(lProject, runCreateTime, 1, []byte("deadbeef")),
				Status:        run.Status_PENDING,
				BilledTo:      userIdentity,
				Mode:          run.FullRun,
				CreateTime:    runCreateTime,
				ConfigGroupID: prjcfg.MakeConfigGroupID("another-config-group", "hash"),
			}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			summary, err := executor.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, summary, should.BeEmpty)
			assert.Loosely(t, rm.notifyStarted, should.BeEmpty)
		})
		t.Run("do not notify pending run from different triggerer", func(t *ftt.Test) {
			r := &run.Run{
				ID:            common.MakeRunID(lProject, runCreateTime, 1, []byte("deadbeef")),
				Status:        run.Status_PENDING,
				BilledTo:      identity.Identity(fmt.Sprintf("%s:%s", identity.User, "another-user@example.com")),
				Mode:          run.FullRun,
				CreateTime:    runCreateTime,
				ConfigGroupID: configGroupID,
			}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			summary, err := executor.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, summary, should.BeEmpty)
			assert.Loosely(t, rm.notifyStarted, should.BeEmpty)
		})
		t.Run("do not notify pending run that has pending Dep Run", func(t *ftt.Test) {
			depRun := &run.Run{
				ID:            common.MakeRunID(lProject, runCreateTime.Add(-1*time.Minute), 1, []byte("deadbeef")),
				Status:        run.Status_PENDING,
				BilledTo:      identity.Identity(fmt.Sprintf("%s:%s", identity.User, "another-user@example.com")),
				Mode:          run.FullRun,
				CreateTime:    runCreateTime,
				ConfigGroupID: configGroupID,
			}
			r := &run.Run{
				ID:            common.MakeRunID(lProject, runCreateTime, 1, []byte("deadbeef")),
				Status:        run.Status_PENDING,
				BilledTo:      userIdentity,
				Mode:          run.FullRun,
				CreateTime:    runCreateTime,
				ConfigGroupID: configGroupID,
				DepRuns:       common.RunIDs{depRun.ID},
			}
			assert.Loosely(t, datastore.Put(ctx, depRun, r), should.BeNil)
			summary, err := executor.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, summary, should.BeEmpty)
			assert.Loosely(t, rm.notifyStarted, should.BeEmpty)
		})
		t.Run("pick the earliest Run to notify", func(t *ftt.Test) {
			seededRand := rand.New(rand.NewSource(12345))
			// Randomly create 100 Runs that are created in the past hour
			runs := make([]*run.Run, 100)
			for i := range runs {
				runCreateTime = clock.Now(ctx).UTC().Add(time.Duration(seededRand.Float64() * float64(time.Hour)))
				runs[i] = &run.Run{
					ID:            common.MakeRunID(lProject, runCreateTime, 1, []byte("deadbeef")),
					Status:        run.Status_PENDING,
					BilledTo:      userIdentity,
					Mode:          run.FullRun,
					CreateTime:    runCreateTime,
					ConfigGroupID: configGroupID,
				}
			}
			assert.Loosely(t, datastore.Put(ctx, runs), should.BeNil)
			var earliestRun *run.Run
			for _, r := range runs {
				if earliestRun == nil || r.CreateTime.Before(earliestRun.CreateTime) {
					earliestRun = r
				}
			}
			summary, err := executor.Do(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, summary, should.Equal(fmt.Sprintf("notified next Run %q to start", earliestRun.ID)))
			assert.Loosely(t, rm.notifyStarted, should.Match(common.RunIDs{earliestRun.ID}))
		})
	})
}

type mockRM struct {
	notifyStarted common.RunIDs
}

func (rm *mockRM) Start(ctx context.Context, runID common.RunID) error {
	rm.notifyStarted = append(rm.notifyStarted, runID)
	return nil
}

type mockQM struct {
	quotaSpecified        bool
	creditQuotaCalledWith common.RunIDs
}

func (qm *mockQM) RunQuotaAccountID(r *run.Run) *quotapb.AccountID {
	return &quotapb.AccountID{
		AppId:        "cv",
		Realm:        r.ID.LUCIProject(),
		Namespace:    r.ConfigGroupID.Name(),
		Name:         r.BilledTo.Email(),
		ResourceType: "mock-runs",
	}
}

func (qm *mockQM) CreditRunQuota(ctx context.Context, r *run.Run) (*quotapb.OpResult, *cfgpb.UserLimit, error) {
	qm.creditQuotaCalledWith = append(qm.creditQuotaCalledWith, r.ID)
	if !qm.quotaSpecified {
		return nil, nil, nil
	}
	return &quotapb.OpResult{
		PreviousBalance: 0,
		NewBalance:      1,
		AccountStatus:   quotapb.OpResult_ALREADY_EXISTS,
		Status:          quotapb.OpResult_SUCCESS,
	}, nil, nil
}
