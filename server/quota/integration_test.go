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

package quota_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/internal/quotakeys"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/redisconn"

	_ "go.chromium.org/luci/server/quota/quotatestmonkeypatch"
)

var integrationTestApp = quota.Register("ita", &quota.ApplicationOptions{
	ResourceTypes: []string{
		"qps",
		"thingBytes",
	},
})

func TestFullFlow(t *testing.T) {
	ftt.Run(`FullFlow`, t, func(t *ftt.Test) {
		ctx := context.Background()

		s, err := miniredis.Run()
		assert.Loosely(t, err, should.BeNil)
		defer s.Close()

		tc := testclock.New(testclock.TestRecentTimeUTC.Round(time.Microsecond))
		ctx = clock.Set(ctx, tc)

		s.SetTime(clock.Now(ctx))

		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

		policy := &quotapb.PolicyConfig{
			Policies: []*quotapb.PolicyConfig_Entry{
				{
					Key: &quotapb.PolicyKey{Namespace: "ns1", Name: "cool people", ResourceType: "qps"},
					Policy: &quotapb.Policy{
						Default: 123,
						Limit:   5156,
						Refill: &quotapb.Policy_Refill{
							Units:    1,
							Interval: 3,
						},
					},
				},
			},
		}
		t.Run(`policy`, func(t *ftt.Test) {
			t.Run(`valid`, func(t *ftt.Test) {
				policyConfigID, err := integrationTestApp.LoadPoliciesAuto(ctx, "@internal:integrationTestApp", policy)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, policyConfigID, should.Resemble(&quotapb.PolicyConfigID{
					AppId:         "ita",
					Realm:         "@internal:integrationTestApp",
					VersionScheme: 1,
					Version:       "W`-@.nn^o,_kP)j_BSM_.:jhMfP]d`mj]J/kKTpa",
				}))

				cfgKey := quotakeys.PolicyConfigID(policyConfigID)
				assert.Loosely(t, s.Keys(), should.Resemble([]string{cfgKey}))
				hkeys, err := s.HKeys(cfgKey)
				assert.Loosely(t, err, should.BeNil)
				polKey := &quotapb.PolicyKey{
					Namespace:    "ns1",
					Name:         "cool people",
					ResourceType: "qps",
				}
				polKeyStr := quotakeys.PolicyKey(polKey)
				assert.Loosely(t, hkeys, should.Resemble([]string{polKeyStr, "~loaded_time"}))
				assert.Loosely(t, s.HGet(cfgKey, polKeyStr), should.Equal(string([]byte{
					147, 123, 205, 20, 36, 146, 1, 3,
				})))

				t.Run(`account`, func(t *ftt.Test) {
					requestId := "somereq"
					rsp, err := quota.ApplyOps(ctx, requestId, durationpb.New(time.Hour), []*quotapb.Op{
						{
							AccountId:  integrationTestApp.AccountID("project:realm", "cvgroup1", "username", "qps"),
							PolicyId:   &quotapb.PolicyID{Config: policyConfigID, Key: polKey},
							RelativeTo: quotapb.Op_CURRENT_BALANCE, Delta: 5,
						},
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp, should.Resemble(&quotapb.ApplyOpsResponse{
						Results: []*quotapb.OpResult{
							{NewBalance: 128, AccountStatus: quotapb.OpResult_CREATED},
						},
						OriginallySet: timestamppb.New(clock.Now(ctx)),
					}))
					assert.Loosely(t, s.TTL(quotakeys.RequestDedupKey(&quotapb.RequestDedupKey{
						Ident:     string(auth.CurrentIdentity(ctx)),
						RequestId: requestId,
					})), should.Equal(time.Hour))
				})
			})

			t.Run(`bad resource`, func(t *ftt.Test) {
				policy.Policies[0].Key.ResourceType = "fakey fake"
				_, err := integrationTestApp.LoadPoliciesAuto(ctx, "@internal:integrationTestApp", policy)
				assert.Loosely(t, err, should.ErrLike("unknown resource type"))
			})
		})

		t.Run(`GetAccounts`, func(t *ftt.Test) {
			existingAccountID := &quotapb.AccountID{
				AppId:        "foo",
				Realm:        "bar",
				Namespace:    "baz",
				Name:         "qux",
				ResourceType: "quux",
			}

			nonExistingAccountID := &quotapb.AccountID{
				AppId:        "foo1",
				Realm:        "bar1",
				Namespace:    "baz1",
				Name:         "qux1",
				ResourceType: "quux1",
			}

			// Add test value to retrieve.
			_, err := quota.ApplyOps(ctx, "", nil, []*quotapb.Op{
				{
					AccountId:  existingAccountID,
					RelativeTo: quotapb.Op_ZERO,
					Delta:      1,
					Options:    uint32(quotapb.Op_IGNORE_POLICY_BOUNDS),
				},
			})
			assert.Loosely(t, err, should.BeNil)

			res, err := quota.GetAccounts(ctx, []*quotapb.AccountID{existingAccountID, nonExistingAccountID})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&quotapb.GetAccountsResponse{
				Accounts: []*quotapb.GetAccountsResponse_AccountState{
					{
						Id: existingAccountID,
						Account: &quotapb.Account{
							Balance:   1,
							UpdatedTs: timestamppb.New(clock.Now(ctx)),
						},
					},
					{
						Id: nonExistingAccountID,
					},
				},
			}))
		})

	})
}
