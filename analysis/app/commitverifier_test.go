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

package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/tsmon"
	cvv1 "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/server/router"

	_ "go.chromium.org/luci/analysis/internal/services/verdictingester" // Needed to ensure task class is registered.

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCVRunHandler(t *testing.T) {
	Convey(`Test CVRunHandler`, t, func() {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())

		rID := "id_full_run"
		fullRunID := fullRunID("cvproject", rID)

		h := &CVRunHandler{}
		rsp := httptest.NewRecorder()
		rctx := &router.Context{
			Writer: rsp,
		}
		Convey(`Valid message`, func() {
			message := &cvv1.PubSubRun{
				Id:       fullRunID,
				Status:   cvv1.Run_SUCCEEDED,
				Hostname: "cvhost",
			}

			called := false
			var processed bool
			h.handleCVRunLegacy = func(ctx context.Context, psRun *cvv1.PubSubRun) (project string, wasProcessed bool, err error) {
				So(called, ShouldBeFalse)
				So(psRun, ShouldResembleProto, message)

				called = true
				return "cvproject", processed, nil
			}
			h.handleCVRun = func(ctx context.Context, psRun *cvv1.PubSubRun) (project string, wasProcessed bool, err error) {
				So(called, ShouldBeTrue)
				So(psRun, ShouldResembleProto, message)

				return "cvproject", processed, nil
			}

			Convey(`Processed`, func() {
				processed = true

				rctx.Request = (&http.Request{Body: makeCVRunReq(message)}).WithContext(ctx)
				h.Handle(rctx)
				So(rsp.Code, ShouldEqual, http.StatusOK)
				So(cvRunCounter.Get(ctx, "cvproject", "success"), ShouldEqual, 1)
			})
			Convey(`Not processed`, func() {
				processed = false

				rctx.Request = (&http.Request{Body: makeCVRunReq(message)}).WithContext(ctx)
				h.Handle(rctx)
				So(rsp.Code, ShouldEqual, http.StatusNoContent)
				So(cvRunCounter.Get(ctx, "cvproject", "ignored"), ShouldEqual, 1)
			})
		})
		Convey(`Invalid data`, func() {
			h.handleCVRun = func(ctx context.Context, psRun *cvv1.PubSubRun) (project string, wasProcessed bool, err error) {
				panic("Should not be reached.")
			}

			rctx.Request = (&http.Request{Body: makeReq([]byte("Hello"), nil)}).WithContext(ctx)
			h.Handle(rctx)
			So(rsp.Code, ShouldEqual, http.StatusAccepted)
			So(cvRunCounter.Get(ctx, "unknown", "permanent-failure"), ShouldEqual, 1)
		})
	})
}

func makeCVRunReq(message *cvv1.PubSubRun) io.ReadCloser {
	blob, _ := protojson.Marshal(message)
	return makeReq(blob, nil)
}

func fullRunID(project, runID string) string {
	return fmt.Sprintf("projects/%s/runs/%s", project, runID)
}
