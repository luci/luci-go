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
	"io"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// This interface encapsulate the communication with tryjob component.
type tryjobNotifier interface {
	ScheduleUpdate(context.Context, common.TryjobID, tryjob.ExternalID) error
}

type tryjobUpdater interface {
	// Update updates the Tryjob entity associated with the given `eid`.
	//
	// `data` should contain the latest information of the Tryjob from the
	// Tryjob backend system (e.g. Build proto from Buildbucket pubsub).
	//
	// No-op if the Tryjob data stored in CV appears to be newer than the provided
	// data (e.g. has newer Tryjob.Result.UpdateTime)
	Update(ctx context.Context, eid tryjob.ExternalID, data any) error
}

// BuildbucketSubscriber processes pubsub messages from Buildbucket.
type BuildbucketSubscriber struct {
	TryjobNotifier tryjobNotifier
	TryjobUpdater  tryjobUpdater
}

// ProcessPubSubMessage processes a pub/sub message from Buildbucket.
//
// It directly updates the Tryjob entity associated with the Buildbucket build
// in the pub/sub message or schedule a task to update them.
func (bs *BuildbucketSubscriber) ProcessPubSubMessage(ctx context.Context, payload common.PubSubMessagePayload) error {
	buildPubSub, err := parseBuildPubSub(payload.Message)
	if err != nil {
		return err
	}
	build := buildPubSub.GetBuild()
	hostname, buildID := build.GetInfra().GetBuildbucket().GetHostname(), build.GetId()
	switch {
	case buildID == 0:
		return errors.New("build id is empty")
	case hostname == "":
		return errors.Fmt("build %d has no hostname", buildID)
	}
	eid, err := tryjob.BuildbucketID(hostname, buildID)
	if err != nil {
		return err
	}
	switch id, err := eid.Resolve(ctx); {
	case err != nil:
		return err
	case id == 0:
		// If eid can't be resolved, it means the build is not tracked by LUCI CV.
		return nil
	case buildPubSub.GetBuildLargeFieldsDropped():
		return bs.TryjobNotifier.ScheduleUpdate(ctx, id, eid)
	default:
		if err := recoverLargeFields(buildPubSub); err != nil {
			return errors.Fmt("failed to recover large fields for build %d: %w", buildID, err)
		}
		return bs.TryjobUpdater.Update(ctx, eid, build)
	}
}

// parseBuildPubSub parses Buildbucket pubsub message data.
func parseBuildPubSub(msg common.PubSubMessage) (*buildbucketpb.BuildsV2PubSub, error) {
	data, err := base64.StdEncoding.DecodeString(msg.Data)
	if err != nil {
		return nil, errors.Fmt("failed to decode message data: %w", err)
	}
	buildPubSub := &buildbucketpb.BuildsV2PubSub{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, buildPubSub); err != nil {
		return nil, errors.Fmt("failed to unmarshal pubsub message into BuildsV2PubSub proto: %w", err)
	}
	return buildPubSub, nil
}

func recoverLargeFields(buildPubSub *buildbucketpb.BuildsV2PubSub) error {
	if len(buildPubSub.GetBuildLargeFields()) == 0 {
		return nil
	}
	largeFieldsData, err := zlibDecompress(buildPubSub.GetBuildLargeFields())
	if err != nil {
		return errors.Fmt("decompress build_large_fields failed: %w", err)
	}
	if err := (proto.UnmarshalOptions{Merge: true, DiscardUnknown: true}).Unmarshal(largeFieldsData, buildPubSub); err != nil {
		return errors.Fmt("unmarshal build_large_fields failed: %w", err)
	}
	return nil
}

// zlibDecompress decompresses data using zlib.
func zlibDecompress(compressed []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}
