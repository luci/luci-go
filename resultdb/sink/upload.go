// Copyright 2019 The LUCI Authors.
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
	"time"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

// DfaultChannelOptions are the default options applied to the dispatcher
// channel for upload handlers.
var DefaultChannelOptions = dispatcher.Options{
	QPSLimit: rate.NewLimiter(rate.Inf, 1),
	Buffer: buffer.Options{
		MaxLeases:    1,
		BatchSize:    1,
		FullBehavior: &buffer.DropOldestBatch{},
		Retry: func() retry.Iterator {
			return &retry.ExponentialBackoff{
				Limited: retry.Limited{
					Delay:    200 * time.Millisecond, // initial delay
					Retries:  -1,
					MaxTotal: 5 * time.Minute,
				},
				Multiplier: 1.2,
				MaxDelay:   30 * time.Second,
			}
		},
	},
}

func createChannel(ctx context.Context, opts *dispatcher.Options) (dispatcher.Channel, error) {
	if opts == nil {
		opts = &DefaultChannelOptions
		opts.DropFn = dispatcher.DropFnSummarized(ctx, rate.NewLimiter(.1, 1))
	}
	sendFn := func(b *buffer.Batch) error {
		result, ok := b.Data[0].(*sinkpb.TestResult)
		if !ok {
			return errors.Reason("invalid message").Err()
		}
		return uploadTestResult(ctx, result)
	}
	return dispatcher.NewChannel(ctx, opts, sendFn)
}

func uploadTestResult(ctx context.Context, msg *sinkpb.TestResult) error {
	iac := make(chan *trpb.Artifact, len(msg.InputArtifacts))
	oac := make(chan *trpb.Artifact, len(msg.InputArtifacts))
	var wg sync.WaitGroup
	wg.Add(len(iac) + len(oac))

	// fan out upload requests for each of the artifacts via the dispatcher
	for n, a := range msg.InputArtifacts {
		go func() {
			iac <- uploadArtifact(n, a)
			wg.Done()
		}
	}
	for name, spba := range msg.OutputArtifacts {
		go func() {
			oac <- uploadArtifact(n, a)
			wg.Done()
		}
	}

	// send the test result to Recorder service after all the upload work
	// completed.
	go func() {
		wg.Wait()
		close(iac)
		close(oac)

		var ia []*trpb.Artifact
		var oa []*trpb.Artifact
		for a := range iac {
			ia = append(ia, a)
		}
		for a := range oac {
			oa = append(oa, a)
		}
		// TODO(crbug/1017288) - invoke Recorder.CreateTestResult()
	}()
	return nil
}

func uploadArtifact(n string, msg *sinkpb.Artifact) *trpb.Artifact {
	return &trpb.Artifact {
		Name        : n,
		FetchUrl    : "",
		ViewUrl     : "",
		ContentType : msg.ContentType,
		Size        : 0,
		Contents    : "",
	}
}
