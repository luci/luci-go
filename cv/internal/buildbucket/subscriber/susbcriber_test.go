// Copyright 2025 The LUCI Authors.
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

package subscriber

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/tryjob"
)

type fakeTryjobUpdater struct {
	updated []tryjob.ExternalID
}

func (fake *fakeTryjobUpdater) Update(ctx context.Context, eid tryjob.ExternalID, data any) error {
	if eid == "" {
		return errors.New("must provide external tryjob id")
	}
	fake.updated = append(fake.updated, eid)
	return nil
}

type fakeTryjobNotifier struct {
	notified []common.TryjobID
}

func (fake *fakeTryjobNotifier) ScheduleUpdate(ctx context.Context, id common.TryjobID, eid tryjob.ExternalID) error {
	if id == 0 {
		return errors.New("must provide internal tryjob id")
	}
	fake.notified = append(fake.notified, id)
	return nil
}

func TestProcessPubSubMessageUpdateTryjob(t *testing.T) {
	t.Parallel()
	ct := cvtesting.Test{}
	ctx := ct.SetUp(t)
	updater := &fakeTryjobUpdater{}
	bs := &BuildbucketSubscriber{TryjobUpdater: updater}
	const bbHost = "buildbucket.example.com"
	build := &buildbucketpb.Build{
		Id: 1,
		Infra: &buildbucketpb.BuildInfra{
			Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
				Hostname: bbHost,
			},
		},
	}
	eid := tryjob.MustBuildbucketID(bbHost, build.GetId())
	eid.MustCreateIfNotExists(ctx)
	payload := makeBuildPubsubMessagePayload(t, build, false)
	assert.NoErr(t, bs.ProcessPubSubMessage(ctx, payload))
	assert.That(t, updater.updated, should.Match([]tryjob.ExternalID{eid}))
}

func TestProcessPubSubMessageDropLargeField(t *testing.T) {
	t.Parallel()
	ct := cvtesting.Test{}
	ctx := ct.SetUp(t)
	notifier := &fakeTryjobNotifier{}
	bs := &BuildbucketSubscriber{TryjobNotifier: notifier}
	const bbHost = "buildbucket.example.com"
	build := &buildbucketpb.Build{
		Id: 1,
		Infra: &buildbucketpb.BuildInfra{
			Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
				Hostname: bbHost,
			},
		},
	}
	eid := tryjob.MustBuildbucketID(bbHost, build.GetId())
	tj := eid.MustCreateIfNotExists(ctx)
	payload := makeBuildPubsubMessagePayload(t, build, true)
	assert.NoErr(t, bs.ProcessPubSubMessage(ctx, payload))
	assert.That(t, notifier.notified, should.Match([]common.TryjobID{tj.ID}))
}

func TestProcessPubSubMessageEmptyBuildID(t *testing.T) {
	t.Parallel()
	bs := &BuildbucketSubscriber{}
	payload := makeBuildPubsubMessagePayload(t, &buildbucketpb.Build{}, false)
	assert.ErrIsLike(t, bs.ProcessPubSubMessage(context.Background(), payload), "build id is empty")
}
func TestProcessPubSubMessageEmptyHostname(t *testing.T) {
	t.Parallel()
	bs := &BuildbucketSubscriber{}
	payload := makeBuildPubsubMessagePayload(t, &buildbucketpb.Build{Id: 1}, false)
	assert.ErrIsLike(t, bs.ProcessPubSubMessage(context.Background(), payload), "build 1 has no hostname")
}

func TestProcessPubSubMessageIrrelevantBuild(t *testing.T) {
	t.Parallel()
	ct := cvtesting.Test{}
	ctx := ct.SetUp(t)
	bs := &BuildbucketSubscriber{}
	payload := makeBuildPubsubMessagePayload(t, &buildbucketpb.Build{
		Id: 1,
		Infra: &buildbucketpb.BuildInfra{
			Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
				Hostname: "buildbucket.example.com",
			},
		},
	}, false)
	assert.NoErr(t, bs.ProcessPubSubMessage(ctx, payload))
}

func makeBuildPubsubMessagePayload(t testing.TB, build *buildbucketpb.Build, dropLargeFields bool) common.PubSubMessagePayload {
	t.Helper()
	msg := &buildbucketpb.BuildsV2PubSub{
		Build:                   proto.Clone(build).(*buildbucketpb.Build),
		BuildLargeFieldsDropped: dropLargeFields,
	}
	if !dropLargeFields {
		large := &buildbucketpb.Build{
			Input: &buildbucketpb.Build_Input{
				Properties: build.GetInput().GetProperties(),
			},
			Output: &buildbucketpb.Build_Output{
				Properties: build.GetOutput().GetProperties(),
			},
			Steps: build.GetSteps(),
		}
		if build.Input != nil {
			build.Input.Properties = nil
		}
		if build.Output != nil {
			build.Output.Properties = nil
		}
		build.Steps = nil
		largeBytes, err := proto.Marshal(large)
		assert.NoErr(t, err, truth.LineContext())

		buf := &bytes.Buffer{}
		zw := zlib.NewWriter(buf)
		_, err = zw.Write(largeBytes)
		assert.NoErr(t, err, truth.LineContext())
		assert.NoErr(t, zw.Close(), truth.LineContext())
		msg.BuildLargeFields = buf.Bytes()
	}
	data, err := protojson.Marshal(msg)
	assert.NoErr(t, err, truth.LineContext())
	return common.PubSubMessagePayload{
		Message: common.PubSubMessage{
			Data: base64.StdEncoding.EncodeToString(data),
		},
	}
}
