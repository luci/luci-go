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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gomodule/redigo/redis"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/server/quota/quotapb"
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

		tc := testclock.New(testclock.TestRecentTimeUTC.Round(time.Microsecond))
		ctx = clock.Set(ctx, tc)

		s.SetTime(clock.Now(ctx))

		ctx = redisconn.UsePool(ctx, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

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
				genUserLimit("chromies-limit", 10, []string{"chromies", "chrome-infra"}),
				genUserLimit("googlers-limit", 5, []string{"googlers"}),
				genUserLimit("partners-limit", 10, []string{"partners"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			So(err, ShouldBeNil)
			So(pid, ShouldResembleProto, &quotapb.PolicyConfigID{
				AppId:         "cv",
				Realm:         "chromium",
				VersionScheme: 1,
				Version:       pid.Version,
			})

			cg.UserLimits = nil
		})

		Convey("WritePolicy() with run limit defaults", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"chromies", "chrome-infra"}),
			)
			cg.UserLimitDefault = genUserLimit("chromies-limit", 5, []string{"chromies", "chrome-infra"})
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			So(err, ShouldBeNil)
			So(pid, ShouldResembleProto, &quotapb.PolicyConfigID{
				AppId:         "cv",
				Realm:         "chromium",
				VersionScheme: 1,
				Version:       pid.Version,
			})

			cg.UserLimits = nil
			cg.UserLimitDefault = nil
		})

		Convey("WritePolicy() with run limit set to unlimited", func() {
			cg.UserLimits = append(cg.UserLimits,
				genUserLimit("chromies-limit", 10, []string{"chromies", "chrome-infra"}),
				genUserLimit("googlers-limit", 0, []string{"googlers"}),
			)
			prjcfgtest.Update(ctx, lProject, cfg)
			pid, err := qm.WritePolicy(ctx, lProject)

			So(err, ShouldBeNil)
			So(pid, ShouldResembleProto, &quotapb.PolicyConfigID{
				AppId:         "cv",
				Realm:         "chromium",
				VersionScheme: 1,
				Version:       pid.Version,
			})

			cg.UserLimits = nil
		})
	})
}
