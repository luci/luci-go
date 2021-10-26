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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/info"
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
// build.Proto.Infra.Resultdb must not be nil.
//
// cfgs is the builder config map with the struct of Bucket ID -> Builder name -> *pb.Builder.
//
// Note: it will mutate the value of build.Proto.Infra.Resultdb.Invocation and build.ResultDBUpdateToken.
func CreateInvocations(ctx context.Context, builds []*model.Build, cfgs map[string]map[string]*pb.Builder, host string) errors.MultiError {
	bbHost := info.AppID(ctx) + ".appspot.com"
	merr := make(errors.MultiError, len(builds))

	_ = parallel.WorkPool(64, func(ch chan<- func() error) {
		for i, b := range builds {
			i := i
			b := b
			proj := b.Proto.Builder.Project
			cfg := cfgs[protoutil.FormatBucketID(proj, b.Proto.Builder.Bucket)][b.Proto.Builder.Builder]
			if !cfg.GetResultdb().GetEnable() {
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
				reqForBldID := &rdbPb.CreateInvocationRequest{
					InvocationId: invID,
					Invocation: &rdbPb.Invocation{
						BigqueryExports:  cfg.Resultdb.BqExports,
						ProducerResource: fmt.Sprintf("//%s/builds/%d", bbHost, b.Proto.Id),
						Realm:            realm,
						HistoryOptions: &rdbPb.HistoryOptions{
							UseInvocationTimestamp: cfg.Resultdb.HistoryOptions.GetUseInvocationTimestamp(),
						},
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
