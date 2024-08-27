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

package pubsub

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"io"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/compilefailuredetection"
	"go.chromium.org/luci/bisection/internal/config"
	configpb "go.chromium.org/luci/bisection/proto/config"
	taskpb "go.chromium.org/luci/bisection/task/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuildBucketPubsub(t *testing.T) {
	t.Parallel()
	compilefailuredetection.RegisterTaskClass()

	Convey("Buildbucket Pubsub Handler", t, func() {
		c := memory.Use(context.Background())
		// Setup config.
		projectCfg := config.CreatePlaceholderProjectConfig()
		cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
		So(config.SetTestProjectConfig(c, cfg), ShouldBeNil)

		message := pubsub.Message{
			Attributes: map[string]string{
				"version": "v2",
			},
		}

		Convey("Should create new task", func() {
			c, scheduler := tq.TestingContext(c, nil)
			largeField, err := largeField("bg")
			So(err, ShouldBeNil)

			buildPubsub := &buildbucketpb.BuildsV2PubSub{
				Build: &buildbucketpb.Build{
					Id: 8000,
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
					},
					Status: buildbucketpb.Status_FAILURE,
				},
				BuildLargeFields: largeField,
			}
			err = BuildbucketPubSubHandler(c, message, buildPubsub)
			So(err, ShouldBeNil)
			// Check that a task was created.
			task := &taskpb.FailedBuildIngestionTask{
				Bbid: 8000,
			}
			expected := proto.Clone(task).(*taskpb.FailedBuildIngestionTask)
			So(scheduler.Tasks().Payloads()[0], ShouldResembleProto, expected)
		})

		Convey("Unsupported project", func() {
			c, _ := tsmon.WithDummyInMemory(c)
			buildPubsub := &buildbucketpb.BuildsV2PubSub{
				Build: &buildbucketpb.Build{
					Builder: &buildbucketpb.BuilderID{
						Project: "chrome",
						Bucket:  "ci",
					},
					Status: buildbucketpb.Status_FAILURE,
				},
			}
			err := BuildbucketPubSubHandler(c, message, buildPubsub)
			So(err, ShouldBeNil)
			So(bbCounter.Get(c, "chrome", "unsupported"), ShouldEqual, 1)
		})

		Convey("Excluded builder group", func() {
			c, _ := tsmon.WithDummyInMemory(c)
			largeField, err := largeField("chromium.clang")
			So(err, ShouldBeNil)
			buildPubsub := &buildbucketpb.BuildsV2PubSub{
				Build: &buildbucketpb.Build{
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
					},
					Status: buildbucketpb.Status_FAILURE,
				},
				BuildLargeFields: largeField,
			}
			err = BuildbucketPubSubHandler(c, message, buildPubsub)
			So(err, ShouldBeNil)
			So(bbCounter.Get(c, "chromium", "unsupported"), ShouldEqual, 1)
		})

		Convey("Rerun metrics captured", func() {
			c, _ := tsmon.WithDummyInMemory(c)

			// Receiving a pubsub message for a terminal status should increase counter.
			buildPubsub := &buildbucketpb.BuildsV2PubSub{
				Build: &buildbucketpb.Build{
					Id: 8000,
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "findit",
						Builder: "gofindit-culprit-verification",
					},
					Status: buildbucketpb.Status_INFRA_FAILURE,
				},
			}
			err := BuildbucketPubSubHandler(c, message, buildPubsub)
			So(err, ShouldBeNil)
			So(rerunCounter.Get(c, "chromium", "INFRA_FAILURE", "compile"), ShouldEqual, 1)

			// Receiving a pubsub message for a terminal status should not increase counter.
			buildPubsub = &buildbucketpb.BuildsV2PubSub{
				Build: &buildbucketpb.Build{
					Id: 8001,
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "findit",
						Builder: "gofindit-culprit-verification",
					},
					Status: buildbucketpb.Status_SCHEDULED,
				},
			}
			err = BuildbucketPubSubHandler(c, message, buildPubsub)
			So(err, ShouldBeNil)
			So(rerunCounter.Get(c, "chromium", "SCHEDULED", "compile"), ShouldEqual, 0)
		})
	})
}

func makeBBReq(message *buildbucketpb.BuildsV2PubSub) io.ReadCloser {
	bm, err := protojson.Marshal(message)
	if err != nil {
		panic(err)
	}

	attributes := map[string]any{
		"version": "v2",
	}

	msg := struct {
		Message struct {
			Data       []byte
			Attributes map[string]any
		}
	}{struct {
		Data       []byte
		Attributes map[string]any
	}{Data: bm, Attributes: attributes}}
	jmsg, _ := json.Marshal(msg)
	return io.NopCloser(bytes.NewReader(jmsg))
}

func largeField(builderGroup string) ([]byte, error) {
	large := &buildbucketpb.Build{
		Input: &buildbucketpb.Build_Input{
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"builder_group": structpb.NewStringValue(builderGroup),
				},
			},
		},
	}
	largeBytes, err := proto.Marshal(large)
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	zw := zlib.NewWriter(buf)
	if _, err := zw.Write(largeBytes); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
