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

package runner

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/time/rate"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/luciexe/runner/runnerbutler"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// Set as a variable here so that tests can mock this away.
//
// In non-tests this is 1 QPS.
var buildbucketQPS = rate.Limit(1)

// errSuccess is a sentinel error so that we can positively assert a successful
// final upload call to buildbucket.
var errSuccess = errors.New("success sentinel")

func mkBuildbucketBuffer(ctx context.Context, rawCB updateBuildCB) (dispatcher.Channel, *atomic.Value) {
	finalErr := &atomic.Value{}
	ret, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
		QPSLimit: rate.NewLimiter(buildbucketQPS, 1),
		Buffer: buffer.Options{
			MaxLeases:    1,
			BatchSize:    1,
			FIFO:         true,
			FullBehavior: &buffer.DropOldestBatch{MaxLiveItems: 1},
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
	}, func(data *buffer.Batch) error {
		build := data.Data[0].(*pb.Build)
		final := protoutil.IsEnded(build.Status)

		timeout := 5 * time.Second
		if final {
			timeout = time.Minute
		}
		tctx, cancel := clock.WithTimeout(ctx, timeout)
		defer cancel()

		var req *pb.UpdateBuildRequest
		if data.Meta == nil {
			req = &pb.UpdateBuildRequest{
				Build: build,
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{
						"build.steps",
						"build.output.properties",
						"build.output.gitiles_commit",
						"build.summary_markdown",
					},
				},
			}
			if final {
				if build.Status != pb.Status_SUCCESS {
					req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.status")
				}
			}
			data.Meta = req
		} else {
			req = data.Meta.(*pb.UpdateBuildRequest)
		}

		err := rawCB(tctx, req)
		if status.Code(errors.Unwrap(err)) == codes.InvalidArgument {
			return err // this is fatal
		}
		// noop if err == nil, retry everything else
		err = transient.Tag.Apply(err)
		if final {
			if err == nil {
				finalErr.Store(errSuccess)
			} else {
				finalErr.Store(err)
			}
		}
		return err
	})
	if err != nil {
		panic(errors.Annotate(err, "impossible, bad options").Err())
	}
	return ret, finalErr
}

func mkLogdogBuffer(ctx context.Context, butler *runnerbutler.Server) (dispatcher.Channel, func()) {
	dgs, err := butler.NewDatagramStream(&streamproto.Properties{
		LogStreamDescriptor: &logpb.LogStreamDescriptor{
			Prefix:      string(butler.Prefix),
			Name:        "build.proto",
			StreamType:  logpb.StreamType_DATAGRAM,
			ContentType: protoutil.BuildMediaType,
		},
	})
	if err != nil {
		panic(err)
	}

	ret, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
		QPSLimit: rate.NewLimiter(rate.Inf, 0),
		Buffer: buffer.Options{
			MaxLeases: 1,
			BatchSize: 1,
			FIFO:      true,
			// We expect butler to sink these very quickly; Dropping data here would
			// be equivalent to dropping random log lines, which we don't expect to be
			// a persistent state. Blocking here would also cause backup in the
			// production of new Build messages.
			//
			// If we observe this to be a problem in production, we can tune this
			// policy.
			FullBehavior: buffer.InfiniteGrowth{},
		},
	}, func(data *buffer.Batch) (err error) {
		build := data.Data[0].(*pb.Build)
		var bytes []byte
		if data.Meta == nil { // cache the assembled proto on Meta
			if bytes, err = proto.Marshal(build); err != nil {
				return err
			}
			data.Meta = bytes
		} else {
			bytes = data.Meta.([]byte)
		}
		return transient.Tag.Apply(dgs.Send(bytes))
	})
	if err != nil {
		panic(errors.Annotate(err, "impossible, bad options").Err())
	}
	return ret, dgs.Close
}

func runBuildDispatcher(ctx context.Context, butler *runnerbutler.Server, rawCB updateBuildCB) (buildChan chan<- *pb.Build, closeWait func(context.Context) error) {
	input := make(chan *pb.Build)

	buildbucketBuf, finalErr := mkBuildbucketBuffer(ctx, rawCB)
	logdogBuf, datagramCloser := mkLogdogBuffer(ctx, butler)

	go func() {
		defer close(buildbucketBuf.C)
		defer close(logdogBuf.C)

		for build := range input {
			buildbucketBuf.C <- build
			logdogBuf.C <- build
		}
	}()

	return input, func(ctx context.Context) error {
		close(input)

		logging.Infof(ctx, "waiting for buildbucket buffer to drain")
		select {
		case <-buildbucketBuf.DrainC:
		case <-ctx.Done():
		}

		logging.Infof(ctx, "waiting for logdog buffer to drain")
		select {
		case <-logdogBuf.DrainC:
		case <-ctx.Done():
		}

		logging.Infof(ctx, "closing build.proto channel")
		datagramCloser()

		if err, _ := finalErr.Load().(error); err != nil {
			if err == errSuccess {
				return nil
			}
			return err
		}
		return errors.New("final build was never processed")
	}
}
