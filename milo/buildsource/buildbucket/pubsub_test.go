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
	"context"
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
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes"
	. "github.com/smartystreets/goconvey/convey"
)

func newMockClient(c context.Context, t *testing.T) (context.Context, *gomock.Controller, *buildbucketpb.MockBuildsClient) {
	ctrl := gomock.NewController(t)
	client := buildbucketpb.NewMockBuildsClient(ctrl)
	factory := func(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (buildbucketpb.BuildsClient, error) {
		return client, nil
	}
	return WithClientFactory(c, factory), ctrl, client
}

// Buildbucket timestamps round off to milliseconds, so define a reference.
var RefTime = time.Date(2016, time.February, 3, 4, 5, 6, 0, time.UTC)

func makeReq(build bbv1.LegacyApiCommonBuildMessage) io.ReadCloser {
	bmsg := struct {
		Build    bbv1.LegacyApiCommonBuildMessage `json:"build"`
		Hostname string                           `json:"hostname"`
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
		c, ctrl, mbc := newMockClient(c, t)
		defer ctrl.Finish()

		// Initialize the appropriate builder.
		builderSummary := &model.BuilderSummary{
			BuilderID: "buildbucket/luci.fake.bucket/fake_builder",
		}
		datastore.Put(c, builderSummary)

		// We'll copy this LegacyApiCommonBuildMessage base for convenience.
		buildBase := bbv1.LegacyApiCommonBuildMessage{
			Bucket:    "luci.fake.bucket",
			Tags:      []string{"builder:fake_builder"},
			CreatedBy: string(identity.AnonymousIdentity),
			CreatedTs: bbv1.FormatTimestamp(RefTime.Add(2 * time.Hour)),
		}

		Convey("New in-process build", func() {
			bKey := MakeBuildKey(c, "hostname", "1234")
			buildExp := buildBase
			buildExp.Id = 1234
			created, _ := ptypes.TimestampProto(RefTime.Add(2 * time.Hour))
			started, _ := ptypes.TimestampProto(RefTime.Add(3 * time.Hour))
			updated, _ := ptypes.TimestampProto(RefTime.Add(5 * time.Hour))

			mbc.EXPECT().GetBuild(gomock.Any(), gomock.Any()).Return(&buildbucketpb.Build{
				Id:         1234,
				Status:     buildbucketpb.Status_STARTED,
				CreateTime: created,
				StartTime:  started,
				UpdateTime: updated,
				Builder: &buildbucketpb.BuilderID{
					Project: "fake",
					Bucket:  "bucket",
					Builder: "fake_builder",
				},
				Input: &buildbucketpb.Build_Input{
					Experimental: true,
				},
			}, nil).AnyTimes()

			h := httptest.NewRecorder()
			r := &http.Request{Body: makeReq(buildExp)}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
			})
			So(h.Code, ShouldEqual, 200)
			datastore.GetTestable(c).CatchupIndexes()

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
			created, _ := ptypes.TimestampProto(RefTime.Add(2 * time.Hour))
			started, _ := ptypes.TimestampProto(RefTime.Add(3 * time.Hour))
			updated, _ := ptypes.TimestampProto(RefTime.Add(6 * time.Hour))
			completed, _ := ptypes.TimestampProto(RefTime.Add(6 * time.Hour))

			mbc.EXPECT().GetBuild(gomock.Any(), gomock.Any()).Return(&buildbucketpb.Build{
				Id:         2234,
				Status:     buildbucketpb.Status_SUCCESS,
				CreateTime: created,
				StartTime:  started,
				EndTime:    completed,
				UpdateTime: updated,
				Builder: &buildbucketpb.BuilderID{
					Project: "fake",
					Bucket:  "bucket",
					Builder: "fake_builder",
				},
				Input: &buildbucketpb.Build_Input{},
			}, nil).AnyTimes()

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
				eBuild := bbv1.LegacyApiCommonBuildMessage{
					Id:        2234,
					Bucket:    "luci.fake.bucket",
					Tags:      []string{"builder:fake_builder"},
					CreatedBy: string(identity.AnonymousIdentity),
					CreatedTs: bbv1.FormatTimestamp(RefTime.Add(2 * time.Hour)),
					StartedTs: bbv1.FormatTimestamp(RefTime.Add(3 * time.Hour)),
					UpdatedTs: bbv1.FormatTimestamp(RefTime.Add(4 * time.Hour)),
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
