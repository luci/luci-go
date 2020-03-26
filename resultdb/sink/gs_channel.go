// Copyright 2020 The LUCI Authors.
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
package sink

import (
	"context"

	"github.com/golang/protobuf/ptypes/timestamp"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

type gsChannel struct {
	uploadCh *dispatcher.Channel
	raw      storageRaw
}

type uploadTask struct {
	name string
	art  *sinkpb.Artifact
}

func (gsc *gsChannel) init(ctx context.Context, cfg ServerConfig) error {
	raw := cfg.testStorageRaw
	if raw == nil {
		var err error
		if raw, err = newGSRaw(ctx, cfg.GStorage, cfg.GSBucket); err != nil {
			return err
		}
	}
	gsc.raw = raw

	// TODO(ddoman): add metrics to instrument uploadCh and tune the buffer options
	// if necessary.
	uchOpts := &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1),
		Buffer: buffer.Options{
			BatchSize:    1,
			MaxLeases:    10, // upload at most 10 artifacts in parallel
			FullBehavior: &buffer.BlockNewItems{MaxItems: 2000},
		},
	}

	ch, err := dispatcher.NewChannel(ctx, uchOpts, func(b *buffer.Batch) error {
		ut := b.Data[0].(*uploadTask)
		return gsc.raw.uploadArtifact(ctx, ut.name, ut.art)
	})
	if err != nil {
		return err
	}

	gsc.uploadCh = &ch
	return nil
}

func (gsc *gsChannel) closeAndDrain(ctx context.Context) {
	gsc.uploadCh.CloseAndDrain(ctx)
}

func (gsc *gsChannel) uploadArtifact(name string, art *sinkpb.Artifact) (string, *timestamp.Timestamp) {
	gsc.uploadCh.C <- &uploadTask{name, art}
	return gsc.raw.fetchURL(name, art)
}
