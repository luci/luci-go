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

package buildbucket

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/buildbucket"
	bucketApi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

// Buildbucket timestamps round off to milliseconds, so define a reference.
var RefTime = time.Date(2016, time.February, 3, 4, 5, 6, 0, time.UTC)

func makeReq(build bucketApi.ApiCommonBuildMessage) io.ReadCloser {
	bmsg := struct {
		Build    bucketApi.ApiCommonBuildMessage `json:"build"`
		Hostname string                          `json:"hostname"`
	}{build, "hostname"}
	bm, _ := json.Marshal(bmsg)

	sub := "projects/luci-milo/subscriptions/buildbucket-public"
	msg := common.PubSubSubscription{
		Subscription: sub,
		Message: common.PubSubMessage{
			Data: base64.StdEncoding.EncodeToString(bm),
		},
	}
	jmsg, _ := json.Marshal(msg)
	return ioutil.NopCloser(bytes.NewReader(jmsg))
}

func TestPubSub(t *testing.T) {
	t.Parallel()

	Convey(`TestPubSub`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		datastore.GetTestable(c).Consistent(true)
		c, _ = testclock.UseTime(c, RefTime)
		c = testconfig.WithCommonClient(c, memcfg.New(bktConfigFull))
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			IdentityGroups: []string{"all"},
		})
		c = caching.WithRequestCache(c)

		// Initialize the appropriate builder.
		builderSummary := &model.BuilderSummary{
			BuilderID: "buildbucket/luci.fake.bucket/fake_builder",
		}
		datastore.Put(c, builderSummary)

		// We'll copy this ApiCommonBuildMessage base for convenience.
		buildBase := bucketApi.ApiCommonBuildMessage{
			Bucket:    "luci.fake.bucket",
			Tags:      []string{"builder:fake_builder"},
			CreatedBy: string(identity.AnonymousIdentity),
			CreatedTs: buildbucket.FormatTimestamp(RefTime.Add(2 * time.Hour)),
		}

		Convey("New in-process build", func() {
			bKey := MakeBuildKey(c, "hostname", "1234")
			buildExp := buildBase
			buildExp.Id = 1234
			buildExp.Status = "STARTED"
			buildExp.CreatedTs = buildbucket.FormatTimestamp(RefTime.Add(2 * time.Hour))
			buildExp.StartedTs = buildbucket.FormatTimestamp(RefTime.Add(3 * time.Hour))
			buildExp.UpdatedTs = buildbucket.FormatTimestamp(RefTime.Add(5 * time.Hour))
			buildExp.Experimental = true

			h := httptest.NewRecorder()
			r := &http.Request{Body: makeReq(buildExp)}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
			})
			So(h.Code, ShouldEqual, 200)

			Convey("stores BuildSummary and BuilderSummary", func() {
				buildAct := model.BuildSummary{BuildKey: bKey}
				err := datastore.Get(c, &buildAct)
				So(err, ShouldBeNil)
				So(buildAct.BuildKey.String(), ShouldEqual, bKey.String())
				So(buildAct.BuilderID, ShouldEqual, "buildbucket/luci.fake.bucket/fake_builder")
				So(buildAct.Summary, ShouldResemble, model.Summary{
					Status: model.Running,
					Start:  RefTime.Add(3 * time.Hour),
				})
				So(buildAct.Created, ShouldResemble, RefTime.Add(2*time.Hour))
				So(buildAct.Experimental, ShouldBeTrue)

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				So(err, ShouldBeNil)
				So(blder.LastFinishedStatus, ShouldResemble, model.NotRun)
				So(blder.LastFinishedBuildID, ShouldEqual, "")
			})
		})

		Convey("Completed build", func() {
			bKey := MakeBuildKey(c, "hostname", "2234")
			buildExp := buildBase
			buildExp.Id = 2234
			buildExp.Status = "COMPLETED"
			buildExp.Result = "SUCCESS"
			buildExp.CreatedTs = buildbucket.FormatTimestamp(RefTime.Add(2 * time.Hour))
			buildExp.StartedTs = buildbucket.FormatTimestamp(RefTime.Add(3 * time.Hour))
			buildExp.UpdatedTs = buildbucket.FormatTimestamp(RefTime.Add(6 * time.Hour))
			buildExp.CompletedTs = buildbucket.FormatTimestamp(RefTime.Add(6 * time.Hour))

			h := httptest.NewRecorder()
			r := &http.Request{Body: makeReq(buildExp)}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
			})
			So(h.Code, ShouldEqual, 200)

			Convey("stores BuildSummary and BuilderSummary", func() {
				buildAct := model.BuildSummary{BuildKey: bKey}
				err := datastore.Get(c, &buildAct)
				So(err, ShouldBeNil)
				So(buildAct.BuildKey.String(), ShouldEqual, bKey.String())
				So(buildAct.BuilderID, ShouldEqual, "buildbucket/luci.fake.bucket/fake_builder")
				So(buildAct.Summary, ShouldResemble, model.Summary{
					Status: model.Success,
					Start:  RefTime.Add(3 * time.Hour),
					End:    RefTime.Add(6 * time.Hour),
				})
				So(buildAct.Created, ShouldResemble, RefTime.Add(2*time.Hour))

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				So(err, ShouldBeNil)
				So(blder.LastFinishedCreated, ShouldResemble, RefTime.Add(2*time.Hour))
				So(blder.LastFinishedStatus, ShouldResemble, model.Success)
				So(blder.LastFinishedBuildID, ShouldEqual, "buildbucket/2234")
			})

			Convey("results in earlier update not being ingested", func() {
				eBuild := bucketApi.ApiCommonBuildMessage{
					Id:        2234,
					Bucket:    "luci.fake.bucket",
					Tags:      []string{"builder:fake_builder"},
					CreatedBy: string(identity.AnonymousIdentity),
					CreatedTs: buildbucket.FormatTimestamp(RefTime.Add(2 * time.Hour)),
					StartedTs: buildbucket.FormatTimestamp(RefTime.Add(3 * time.Hour)),
					UpdatedTs: buildbucket.FormatTimestamp(RefTime.Add(4 * time.Hour)),
					Status:    "STARTED",
				}

				h := httptest.NewRecorder()
				r := &http.Request{Body: makeReq(eBuild)}
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
				})
				So(h.Code, ShouldEqual, 200)

				buildAct := model.BuildSummary{BuildKey: bKey}
				err := datastore.Get(c, &buildAct)
				So(err, ShouldBeNil)
				So(buildAct.Summary, ShouldResemble, model.Summary{
					Status: model.Success,
					Start:  RefTime.Add(3 * time.Hour),
					End:    RefTime.Add(6 * time.Hour),
				})
				So(buildAct.Created, ShouldResemble, RefTime.Add(2*time.Hour))

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				So(err, ShouldBeNil)
				So(blder.LastFinishedCreated, ShouldResemble, RefTime.Add(2*time.Hour))
				So(blder.LastFinishedStatus, ShouldResemble, model.Success)
				So(blder.LastFinishedBuildID, ShouldEqual, "buildbucket/2234")
			})
		})
	})

}
