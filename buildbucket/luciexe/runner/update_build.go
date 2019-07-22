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
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// updateBuild calls rawCB.
// If final is true, may update the build status, making it immutable.
//
// Final calls will retry with exponential backoff for up to 5 minutes.
// Non-final calls will do NO retries.
//
// May return a transient error (in the event that the final RPC was actually
// a retryable error).
func updateBuild(ctx context.Context, build *pb.Build, final bool, rawCB updateBuildCB) error {
	req := &pb.UpdateBuildRequest{
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
		// If the build has failed, update the build status.
		// If it succeeded, do not set it just yet, since there are more ways
		// the build can fail.
		switch {
		case !protoutil.IsEnded(build.Status):
			return errors.Reason("build status %q is not final", build.Status).Err()
		case build.Status != pb.Status_SUCCESS:
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.status")
		}
	}

	// Make the RPC.
	//
	// If this is the final update then we use a retry iterator to do our best to
	// ensure the final build message goes through.
	retryFactory := retry.None
	if final {
		retryFactory = func() retry.Iterator {
			return &retry.ExponentialBackoff{
				Limited: retry.Limited{
					Retries:  -1, // no limit
					MaxTotal: 5 * time.Minute,
				},
				Multiplier: 1.2,
				MaxDelay:   30 * time.Second,
			}
		}
	}

	return retry.Retry(
		ctx, retryFactory,
		func() error {
			err := rawCB(ctx, req)
			switch status.Code(errors.Unwrap(err)) {
			case codes.OK:
				return nil

			case codes.InvalidArgument:
				// This is fatal.
				return err

			default:
				return transient.Tag.Apply(err)
			}
		},
		retry.LogCallback(ctx, "luciexe.runner.updateBuild"),
	)
}
