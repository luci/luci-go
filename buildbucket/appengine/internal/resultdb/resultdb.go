// Copyright 2021 The LUCI Authors.
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

package resultdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	rdbPb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var mockRecorderClientKey = "used in tests only for setting the mock recorder client"

// SetMockRecorder set the mock resultDB recorder client for testing purpose.
func SetMockRecorder(ctx context.Context, mock rdbPb.RecorderClient) context.Context {
	return context.WithValue(ctx, &mockRecorderClientKey, mock)
}

// CreateOptions for each invocation created by CreateInvocations.
type CreateOptions struct {
	// Whether to mark the ResultDB invocation an export root.
	IsExportRoot bool
}

// CreateInvocations creates resultdb invocations for each build.
// build.Proto.Infra.Resultdb must not be nil.
// builds and opts must have the same length and match 1:1.
//
// Note: it will mutate the value of build.Proto.Infra.Resultdb.Invocation and build.ResultDBUpdateToken.
func CreateInvocations(ctx context.Context, builds []*model.Build, opts []CreateOptions) errors.MultiError {
	if len(builds) != len(opts) {
		panic("builds and opts have mismatched length")
	}
	bbHost := info.AppID(ctx) + ".appspot.com"
	merr := make(errors.MultiError, len(builds))
	if len(builds) == 0 {
		return nil
	}
	host := builds[0].Proto.GetInfra().GetResultdb().GetHostname()
	if host == "" {
		return nil
	}
	now := clock.Now(ctx).UTC()

	_ = parallel.WorkPool(64, func(ch chan<- func() error) {
		for i, b := range builds {
			proj := b.Proto.Builder.Project
			if !b.Proto.GetInfra().GetResultdb().GetEnable() {
				continue
			}
			realm := b.Realm()
			if realm == "" {
				logging.Warningf(ctx, fmt.Sprintf("the builder %q has resultDB enabled while the build %d doesn't have realm", b.Proto.Builder.Builder, b.Proto.Id))
				continue
			}
			ch <- func() error {
				// TODO(crbug/1042991): After build scheduling flow also dedups number not just the id,
				// we can combine build id invocation and number invocation into a Batch.

				// Use per-project credential to create invocation.
				recorderClient, err := newRecorderClient(ctx, host, proj)
				if err != nil {
					merr[i] = errors.Annotate(err, "failed to create resultDB recorder client for project: %s", proj).Err()
					return nil
				}

				// Make a call to create build id invocation.
				invID := fmt.Sprintf("build-%d", b.Proto.Id)
				deadline := now.Add(b.Proto.ExecutionTimeout.AsDuration()).Add(b.Proto.SchedulingTimeout.AsDuration())
				reqForBldID := &rdbPb.CreateInvocationRequest{
					InvocationId: invID,
					Invocation: &rdbPb.Invocation{
						BigqueryExports:  b.Proto.GetInfra().GetResultdb().GetBqExports(),
						Deadline:         timestamppb.New(deadline),
						ProducerResource: fmt.Sprintf("//%s/builds/%d", bbHost, b.Proto.Id),
						Realm:            realm,
						IsExportRoot:     opts[i].IsExportRoot,
					},
					RequestId: invID,
				}
				header := metadata.MD{}
				if _, err = recorderClient.CreateInvocation(ctx, reqForBldID, grpc.Header(&header)); err != nil {
					merr[i] = errors.Annotate(err, "failed to create the invocation for build id: %d", b.Proto.Id).Err()
					return nil
				}
				token, ok := header["update-token"]
				if !ok {
					merr[i] = errors.Reason("CreateInvocation response doesn't have update-token header for build id: %d", b.Proto.Id).Err()
					return nil
				}
				b.ResultDBUpdateToken = token[0]
				b.Proto.Infra.Resultdb.Invocation = fmt.Sprintf("invocations/%s", reqForBldID.InvocationId)

				// Create another invocation for the build number in which it includes the invocation for build id,
				// If the build has the Number field populated.
				if b.Proto.Number > 0 {
					sha256Builder := sha256.Sum256([]byte(protoutil.FormatBuilderID(b.Proto.Builder)))
					_, err = recorderClient.CreateInvocation(ctx, &rdbPb.CreateInvocationRequest{
						InvocationId: fmt.Sprintf("build-%s-%d", hex.EncodeToString(sha256Builder[:]), b.Proto.Number),
						Invocation: &rdbPb.Invocation{
							State:               rdbPb.Invocation_FINALIZING,
							ProducerResource:    reqForBldID.Invocation.ProducerResource,
							Realm:               realm,
							IncludedInvocations: []string{fmt.Sprintf("invocations/%s", reqForBldID.InvocationId)},
						},
						RequestId: fmt.Sprintf("build-%d-%d", b.Proto.Id, b.Proto.Number),
					})
					if err != nil {
						merr[i] = errors.Annotate(err, "failed to create the invocation for build number: %d (build id: %d)", b.Proto.Number, b.Proto.Id).Err()
						return nil
					}
				}
				return nil
			}
		}
	})

	if merr.First() != nil {
		return merr
	}
	return nil
}

// FinalizeInvocation calls ResultDB to finalize the build's invocation.
func FinalizeInvocation(ctx context.Context, buildID int64) error {
	b := &model.Build{ID: buildID}
	infra := &model.BuildInfra{
		Build: datastore.KeyForObj(ctx, b),
	}
	switch err := datastore.Get(ctx, b, infra); {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		return tq.Fatal.Apply(errors.Fmt("build %d or buildInfra not found: %w", buildID, err))
	case err != nil:
		return errors.Annotate(err, "failed to fetch build %d or buildInfra", buildID).Tag(transient.Tag).Err()
	}
	rdb := infra.Proto.Resultdb
	if rdb.Hostname == "" || rdb.Invocation == "" {
		// If there's no hostname or no invocation, it means resultdb integration
		// is not enabled for this build.
		return nil
	}

	recorderClient, err := newRecorderClient(ctx, rdb.Hostname, b.Project)
	if err != nil {
		return errors.Annotate(err, "failed to create a recorder client for build %d", buildID).Tag(tq.Fatal).Err()
	}

	ctx = metadata.AppendToOutgoingContext(ctx, "update-token", b.ResultDBUpdateToken)
	if _, err := recorderClient.FinalizeInvocation(ctx, &rdbPb.FinalizeInvocationRequest{Name: rdb.Invocation}); err != nil {
		code := grpcutil.Code(err)
		if code == codes.FailedPrecondition || code == codes.PermissionDenied {
			return errors.Annotate(err, "Fatal rpc error when finalizing %s for build %d", rdb.Invocation, buildID).Tag(tq.Fatal).Err()
		} else {
			// Retry other errors.
			return transient.Tag.Apply(err)
		}
	}
	return nil
}

func newRecorderClient(ctx context.Context, host string, project string) (rdbPb.RecorderClient, error) {
	if mockClient, ok := ctx.Value(&mockRecorderClientKey).(*rdbPb.MockRecorderClient); ok {
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, err
	}
	return rdbPb.NewRecorderPRPCClient(
		&prpc.Client{
			C:    &http.Client{Transport: t},
			Host: host,
		}), nil
}
