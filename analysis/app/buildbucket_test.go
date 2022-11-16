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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/services/buildjoiner"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

const (
	// Host name of buildbucket.
	bbHost = "cr-buildbucket.appspot.com"
)

func TestHandleBuild(t *testing.T) {
	buildjoiner.RegisterTaskClass()

	Convey(`Test BuildbucketPubSubHandler`, t, func() {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		ctx, skdr := tq.TestingContext(ctx, nil)

		pubSubMessage := bbv1.LegacyApiCommonBuildMessage{
			Project: "buildproject",
			Bucket:  "bucket",
			Id:      14141414,
		}
		rsp := httptest.NewRecorder()
		rctx := &router.Context{
			Context: ctx,
			Writer:  rsp,
		}
		Convey(`Completed build`, func() {
			pubSubMessage.Status = bbv1.StatusCompleted

			rctx.Request = &http.Request{Body: makeBBReq(pubSubMessage, bbHost)}
			BuildbucketPubSubHandler(rctx)
			So(rsp.Code, ShouldEqual, http.StatusOK)
			So(buildCounter.Get(ctx, "buildproject", "success"), ShouldEqual, 1)

			So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
			resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.JoinBuild)

			expectedTask := &taskspb.JoinBuild{
				Host:    bbHost,
				Project: "buildproject",
				Id:      14141414,
			}
			So(resultsTask, ShouldResembleProto, expectedTask)
		})
		Convey(`Uncompleted build`, func() {
			pubSubMessage.Status = bbv1.StatusStarted

			rctx.Request = &http.Request{Body: makeBBReq(pubSubMessage, bbHost)}
			BuildbucketPubSubHandler(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNoContent)
			So(buildCounter.Get(ctx, "buildproject", "ignored"), ShouldEqual, 1)

			So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
		})
		Convey(`Invalid data`, func() {
			rctx.Request = &http.Request{Body: makeReq([]byte("Hello"))}
			BuildbucketPubSubHandler(rctx)
			So(rsp.Code, ShouldEqual, http.StatusAccepted)
			So(buildCounter.Get(ctx, "unknown", "permanent-failure"), ShouldEqual, 1)

			So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
		})
	})
}

func makeBBReq(build bbv1.LegacyApiCommonBuildMessage, hostname string) io.ReadCloser {
	bmsg := struct {
		Build    bbv1.LegacyApiCommonBuildMessage `json:"build"`
		Hostname string                           `json:"hostname"`
	}{build, hostname}
	bm, _ := json.Marshal(bmsg)
	return makeReq(bm)
}
