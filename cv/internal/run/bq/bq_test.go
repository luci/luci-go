// Copyright 2021 The LUCI Authors.
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

package bq

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"

	"go.chromium.org/luci/cv/internal/common"
	//"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/migration"
)

func TestAttemptFtching(t *testing.T) {
	Convey("getAttempt", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		id := common.MakeRunID(
			"chromium", ct.Clock.Now().UTC(), 1, []byte("cafecafe"))

		attempt := &cvbqpb.Attempt{
			Key:                  "f001234",
			ConfigGroup:          "maingroup",
			LuciProject:          "chromium",
			ClGroupKey:           "b004321",
			EquivalentClGroupKey: "c003333",
			StartTime:            timestamppb.New(clock.Now(ctx).Add(-2 * time.Minute)),
			EndTime:              timestamppb.New(clock.Now(ctx).Add(-1 * time.Minute)),
			Builds: []*cvbqpb.Build{
				{
					Id:       423143214321,
					Host:     "cr-buildbucket.appspot.com",
					Origin:   cvbqpb.Build_NOT_REUSED,
					Critical: false,
				},
			},
			GerritChanges: []*cvbqpb.GerritChange{
				{
					Host:                       "https://chromium-review.googlesource.com/",
					Project:                    "chromium/src",
					Change:                     11111,
					Patchset:                   7,
					EarliestEquivalentPatchset: 6,
					Mode:                       cvbqpb.Mode_FULL_RUN,
					SubmitStatus:               cvbqpb.GerritChange_PENDING,
				},
			},
			Status:               cvbqpb.AttemptStatus_SUCCESS,
			Substatus:            cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
			HasCustomRequirement: false,
		}

		vr := &migration.VerifiedCQDRun{
			ID: id,
			Payload: &migrationpb.ReportVerifiedRunRequest{
				Run: &migrationpb.Run{
					Attempt: attempt,
					Id:      string(id),
					Cls:     []*migrationpb.RunCL{},
				},
			},
		}
		So(datastore.Put(ctx, vr), ShouldBeNil)

		Convey("returns attempt from datastore", func() {
			a, err := fetchCQDAttempt(ctx, id)
			So(err, ShouldBeNil)
			So(a, ShouldResembleProto, attempt)
		})

		Convey("returns error when not in datastore", func() {
			id := common.MakeRunID(
				"x", ct.Clock.Now().UTC(), 1, []byte("aaa"))
			a, err := fetchCQDAttempt(ctx, id)
			So(err, ShouldNotBeNil)
			So(a, ShouldBeNil)
		})
	})
}
