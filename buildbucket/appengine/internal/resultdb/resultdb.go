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

	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/prpc"
	rdbPb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var mockRecorderClientKey = "used in tests only for setting the mock recorder client"

// CreateInvocations creates resultdb invocations for each build.
// It would only create invocations if ResultDB hostname is globally set.
//
// cfgs is the builder config map with the struct of Bucket ID -> Builder name -> *pb.Builder.
//
// Note: it will mutate the value of build.Proto.Infra.Resultdb.Invocation and build.ResultDBUpdateToken.
func CreateInvocations(ctx context.Context, builds []*model.Build, cfgs map[string]map[string]*pb.Builder) error {
	// TODO(yuanjunh): in the next CL, fetch settings.cfg for rdbHost; get proper bbHost.
	rdbHost := "rdbHost"
	bbHost := "bbHost"

	// a map of project -> recoder client, since the rpc to create invocations uses per-project credentials.
	projToClient := make(map[string]rdbPb.RecorderClient)
	var err error
	err = parallel.WorkPool(64, func(ch chan<- func() error) {
		for _, b := range builds {
			b := b
			proj := b.Proto.Builder.Project
			cfg := cfgs[protoutil.FormatBucketID(proj, b.Proto.Builder.Bucket)][b.Proto.Builder.Builder]
			if !cfg.Resultdb.Enable {
				continue
			}
			realm := b.Realm()
			if realm == "" {
				logging.Warningf(ctx, fmt.Sprintf("the builder %s has resultDB enabled while the build %d doesn't have realm", b.Proto.Builder.Builder, b.Proto.Id))
				continue
			}
			if _, ok := projToClient[proj]; !ok {
				projToClient[proj], err = newRecorderClient(ctx, rdbHost, proj)
				if err != nil {
					err = errors.Annotate(err, "failed to create resultDB recorder client for project: %s", proj).Err()
					return
				}
			}
			ch <- func() error {
				// TODO(crbug/1042991): After build scheduling flow also dedups number not just the id,
				// we can combine build id invocation and number invocation into a Batch.

				recoderClient, err := newRecorderClient(ctx, rdbHost, proj)
				if err != nil {
					return errors.Annotate(err, "failed to create resultDB recorder client for project: %s", proj).Err()
				}

				// Make a call to create build id invocation.
				invId := fmt.Sprintf("build-%d", b.Proto.Id)
				reqForBldID := &rdbPb.CreateInvocationRequest{
					InvocationId: fmt.Sprintf("build-%d", b.Proto.Id),
					Invocation: &rdbPb.Invocation{
						BigqueryExports:  cfg.Resultdb.BqExports,
						ProducerResource: fmt.Sprintf("//%s/builds/%d", bbHost, b.Proto.Id),
						Realm:            realm,
						HistoryOptions: &rdbPb.HistoryOptions{
							UseInvocationTimestamp: cfg.Resultdb.HistoryOptions.UseInvocationTimestamp,
						},
					},
					RequestId: invId,
				}
				header := metadata.MD{}
				_, err = recoderClient.CreateInvocation(ctx, reqForBldID, grpc.Header(&header))
				if err = filterAlreadyExistsErr(err); err != nil {
					return errors.Annotate(err, "failed to create the invocation for build id: %d", b.Proto.Id).Err()
				}

				// Create another invocation for the build number in which it refer to the invocation for build id,
				// If the build has the Number field populated.
				if b.Proto.Number > 0 {
					sha256Builder := sha256.Sum256([]byte(protoutil.FormatBuilderID(b.Proto.Builder)))
					_, err = recoderClient.CreateInvocation(ctx, &rdbPb.CreateInvocationRequest{
						InvocationId: fmt.Sprintf("build-%s-%d", hex.EncodeToString(sha256Builder[:]), b.Proto.Number),
						Invocation: &rdbPb.Invocation{
							State:               rdbPb.Invocation_FINALIZING,
							ProducerResource:    reqForBldID.Invocation.ProducerResource,
							Realm:               realm,
							IncludedInvocations: []string{fmt.Sprintf("invocations/%s", reqForBldID.InvocationId)},
						},
						RequestId: fmt.Sprintf("build-%d-%d", b.Proto.Id, b.Proto.Number),
					}, grpc.Header(&header))
					if err = filterAlreadyExistsErr(err); err != nil {
						return errors.Annotate(err, "failed to create the invocation for build number: %d (build id: %d)", b.Proto.Number, b.Proto.Id).Err()
					}
				}

				// If there is a build number invocation request, the header is from that response.
				// Otherwise the header is from response of build id invocation request.
				token, ok := header["update-token"]
				if !ok {
					return errors.Reason("CreateInvocation response doesn't have update-token header for build id: %d", b.Proto.Id).Err()
				}
				b.ResultDBUpdateToken = token[0]
				b.Proto.Infra.Resultdb.Invocation = fmt.Sprintf("invocations/%s", reqForBldID.InvocationId)
				return nil
			}
		}
	})

	if err != nil {
		return err
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

// filterAlreadyExistsErr returns nil if an error is a grpc err with the AlreadyExists code.
// Otherwise, it return the original error.
func filterAlreadyExistsErr(err error) error {
	gStatus, ok := grpcStatus.FromError(err)
	if !ok || gStatus.Code() != codes.AlreadyExists {
		return err
	}
	return nil
}
