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

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/quota"
	"go.chromium.org/luci/server/quota/internal/quotakeys"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/redisconn"

	_ "go.chromium.org/luci/server/quota/quotatestmonkeypatch"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var integrationTestApp = quota.Register("ita", &quota.ApplicationOptions{
	ResourceTypes: []string{
		"qps",
		"thingBytes",
	},
})

func TestFullFlow(t *testing.T) {
	Convey(`FullFlow`, t, func() {
		ctx := context.Background()

		s, err := miniredis.Run()
		So(err, ShouldBeNil)
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
		Convey(`policy`, func() {
			Convey(`valid`, func() {
				policyConfigID, err := integrationTestApp.LoadPoliciesAuto(ctx, "@internal:integrationTestApp", policy)
				So(err, ShouldBeNil)
				So(policyConfigID, ShouldResembleProto, &quotapb.PolicyConfigID{
					AppId:         "ita",
					Realm:         "@internal:integrationTestApp",
					VersionScheme: 1,
					Version:       "W`-@.nn^o,_kP)j_BSM_.:jhMfP]d`mj]J/kKTpa",
				})

				cfgKey := quotakeys.PolicyConfigID(policyConfigID)
				So(s.Keys(), ShouldResemble, []string{cfgKey})
				hkeys, err := s.HKeys(cfgKey)
				So(err, ShouldBeNil)
				polKey := &quotapb.PolicyKey{
					Namespace:    "ns1",
					Name:         "cool people",
					ResourceType: "qps",
				}
				polKeyStr := quotakeys.PolicyKey(polKey)
				So(hkeys, ShouldResemble, []string{polKeyStr, "~loaded_time"})
				So(s.HGet(cfgKey, polKeyStr), ShouldEqual, string([]byte{
					147, 123, 205, 20, 36, 146, 1, 3,
				}))

				Convey(`account`, func() {
					requestId := "somereq"
					rsp, err := quota.ApplyOps(ctx, requestId, durationpb.New(time.Hour), []*quotapb.Op{
						{
							AccountId:  integrationTestApp.AccountID("project:realm", "cvgroup1", "username", "qps"),
							PolicyId:   &quotapb.PolicyID{Config: policyConfigID, Key: polKey},
							RelativeTo: quotapb.Op_CURRENT_BALANCE, Delta: 5,
						},
					})
					So(err, ShouldBeNil)
					So(rsp, ShouldResembleProto, &quotapb.ApplyOpsResponse{
						Results: []*quotapb.OpResult{
							{NewBalance: 128, AccountStatus: quotapb.OpResult_CREATED},
						},
						OriginallySet: timestamppb.New(clock.Now(ctx)),
					})
					So(s.TTL(quotakeys.RequestDedupKey(&quotapb.RequestDedupKey{
						Ident:     string(auth.CurrentIdentity(ctx)),
						RequestId: requestId,
					})), ShouldEqual, time.Hour)
				})
			})

			Convey(`bad resource`, func() {
				policy.Policies[0].Key.ResourceType = "fakey fake"
				_, err := integrationTestApp.LoadPoliciesAuto(ctx, "@internal:integrationTestApp", policy)
				So(err, ShouldErrLike, "unknown resource type")
			})
		})

		Convey(`GetAccounts`, func() {
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
			So(err, ShouldBeNil)

			res, err := quota.GetAccounts(ctx, []*quotapb.AccountID{existingAccountID, nonExistingAccountID})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &quotapb.GetAccountsResponse{
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
			})
		})

	})
}
