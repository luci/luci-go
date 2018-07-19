// Copyright 2017 The LUCI Authors.
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

package upload

import (
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOperation(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)

		ctx := gaetesting.TestingContext()
		ctx, _ = testclock.UseTime(ctx, testTime)

		Convey("ToProto", func() {
			op := Operation{
				ID:        123,
				Status:    api.UploadStatus_UPLOADING,
				UploadURL: "http://upload-url.example.com",
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: strings.Repeat("a", 64),
				Error:     "zzz",
			}

			// No Object when in UPLOADING.
			So(op.ToProto("wrappedID"), ShouldResemble, &api.UploadOperation{
				OperationId:  "wrappedID",
				UploadUrl:    "http://upload-url.example.com",
				Status:       api.UploadStatus_UPLOADING,
				ErrorMessage: "zzz",
			})

			// With Object when in PUBLISHED.
			op.Status = api.UploadStatus_PUBLISHED
			So(op.ToProto("wrappedID"), ShouldResemble, &api.UploadOperation{
				OperationId: "wrappedID",
				UploadUrl:   "http://upload-url.example.com",
				Status:      api.UploadStatus_PUBLISHED,
				Object: &api.ObjectRef{
					HashAlgo:  op.HashAlgo,
					HexDigest: op.HexDigest,
				},
				ErrorMessage: "zzz",
			})
		})

		Convey("Advance works", func() {
			op := &Operation{
				ID:     123,
				Status: api.UploadStatus_UPLOADING,
			}

			Convey("Success", func() {
				So(datastore.Put(ctx, op), ShouldBeNil)
				newOp, err := op.Advance(ctx, func(_ context.Context, o *Operation) error {
					o.Status = api.UploadStatus_ERRORED
					o.Error = "zzz"
					return nil
				})
				So(err, ShouldBeNil)
				So(newOp, ShouldResemble, &Operation{
					ID:        123,
					Status:    api.UploadStatus_ERRORED,
					Error:     "zzz",
					UpdatedTS: testTime,
				})

				// Original one untouched.
				So(op.Error, ShouldEqual, "")

				// State in the datastore is really updated.
				So(datastore.Get(ctx, op), ShouldBeNil)
				So(op, ShouldResemble, newOp)
			})

			Convey("Skips because of unexpected status", func() {
				cpy := *op
				cpy.Status = api.UploadStatus_ERRORED
				So(datastore.Put(ctx, &cpy), ShouldBeNil)

				newOp, err := op.Advance(ctx, func(context.Context, *Operation) error {
					panic("must not be called")
				})
				So(err, ShouldBeNil)
				So(newOp, ShouldResemble, &cpy)
			})

			Convey("Callback error", func() {
				So(datastore.Put(ctx, op), ShouldBeNil)
				_, err := op.Advance(ctx, func(_ context.Context, o *Operation) error {
					o.Error = "zzz" // must be ignored
					return errors.New("omg")
				})
				So(err, ShouldErrLike, "omg")
				So(datastore.Get(ctx, op), ShouldBeNil)
				So(op.Error, ShouldEqual, "")
			})

			Convey("Missing entity", func() {
				_, err := op.Advance(ctx, func(context.Context, *Operation) error {
					panic("must not be called")
				})
				So(err, ShouldErrLike, "no such entity")
			})
		})
	})
}
