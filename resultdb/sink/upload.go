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

func createChannel(ctx context.Context, scfg ServerConfig) (dispatcher.Channel, error) {
	opts := &DefaultChannelOptions
	opts.DropFn = dispatcher.DropFnSummarized(ctx, rate.NewLimiter(.1, 1))
	if scfg.ChannelOptions != nil {
		opts = scfg.ChannelOptions
	}

	sendFn := func(b *buffer.Batch) error {
		msg, ok := b.Data[0].(*sinkpb.SinkMessageContainer)
		if !ok {
			return errors.Reason("invalid message").Err()
		}

		var err error
		if tr := msg.GetTestResult(); tr != nil {
			err = uploadTestResult(ctx, tr)
		} else if trf := msg.GetTestResultFile(); trf != nil {
			err = processUploadTestResultFile(ctx, trf)
		} else if hs := msg.GetHandshake(); hs != nil {
			// this must be a bug.
			logging.Errorf(ctx, "Received Handshake from the channel")
		} else {
			err = errors.Reason("Unknown SinkMessageContainer").Err()
		}
		return err
	}
	return dispatcher.NewChannel(ctx, opts, sendFn)
}

func uploadTestResult(ctx context.Context, msg *sinkpb.TestResult) error {
	return nil
}

func uploadTestResultFile(ctx context.Context, msg *sinkpb.TestResultFile) error {
	return nil
}
