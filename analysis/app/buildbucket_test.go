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
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

const (
	// Host name of buildbucket.
	bbHost = "cr-buildbucket.appspot.com"
)

func TestHandleBuild(t *testing.T) {
	ftt.Run(`Test BuildbucketPubSubHandler`, t, func(t *ftt.Test) {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		ctx, skdr := tq.TestingContext(ctx, nil)

		pubSubMessage := &buildbucketpb.BuildsV2PubSub{
			Build: &buildbucketpb.Build{
				Builder: &buildbucketpb.BuilderID{
					Project: "buildproject",
				},
				Id: 14141414,
				Infra: &buildbucketpb.BuildInfra{
					Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
						Hostname: bbHost,
					},
				},
			},
		}
		t.Run(`Completed build`, func(t *ftt.Test) {
			pubSubMessage.Build.Status = buildbucketpb.Status_SUCCESS

			err := BuildbucketPubSubHandler(ctx, pubsub.Message{}, pubSubMessage)
			assert.That(t, err, should.ErrLike(nil))
			assert.Loosely(t, buildCounter.Get(ctx, "buildproject", "success"), should.Equal(1))

			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(1))
			resultsTask := skdr.Tasks().Payloads()[0].(*taskspb.JoinBuild)

			expectedTask := &taskspb.JoinBuild{
				Host:    bbHost,
				Project: "buildproject",
				Id:      14141414,
			}
			assert.Loosely(t, resultsTask, should.Match(expectedTask))
		})
		t.Run(`Uncompleted build`, func(t *ftt.Test) {
			pubSubMessage.Build.Status = buildbucketpb.Status_STARTED

			err := BuildbucketPubSubHandler(ctx, pubsub.Message{}, pubSubMessage)
			assert.That(t, pubsub.Ignore.In(err), should.BeTrue)
			assert.Loosely(t, buildCounter.Get(ctx, "buildproject", "ignored"), should.Equal(1))

			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)
		})
	})
}

func makeBBV2Req(message *buildbucketpb.BuildsV2PubSub) pubsub.Message {
	bm, err := protojson.Marshal(message)
	if err != nil {
		panic(err)
	}

	attributes := map[string]string{
		"version": "v2",
	}
	return pubsub.Message{
		Data:       bm,
		Attributes: attributes,
	}
}
