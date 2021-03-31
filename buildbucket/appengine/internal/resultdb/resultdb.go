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
	"go.chromium.org/luci/grpc/prpc"
	rdbPb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var mockRecorderClientKey = "used in tests only for setting the mock recorder client"

// CreateInvocations creates resultdb invocations for each build.
// It would only create invocations if ResultDB hostname is globally set.
// cfgs is the builder config map with the struct of Bucket ID -> Builder name -> *pb.Builder.
//
// Note: it will mutate the value of build.Proto.Infra.Resultdb.Invocation and build.ResultDBUpdateToken.
func CreateInvocations(ctx context.Context, builds []*model.Build, cfgs map[string]map[string]*pb.Builder) error {
	// TODO(yuanjunh): in the next CL, fetch settings.cfg for rdbHost; get proper bbHost.
	rdbHost := "rdbHost"
	bbHost := "bbHost"

	// We need to do one batch request per project, since the rpc to create invocations uses per-project credentials.
	batchReqs := make(map[string]*rdbPb.BatchCreateInvocationsRequest)
	invNameToBuild := make(map[string]*model.Build)
	for _, b := range builds {
		proj := b.Proto.Builder.Project
		cfg := cfgs[protoutil.FormatBucketID(proj, b.Proto.Builder.Bucket)][b.Proto.Builder.Builder]
		if !cfg.Resultdb.Enable {
			continue
		}
		realm := b.Realm()
		if realm == "" {
			logging.Warningf(ctx, "the builder has resultDB enabled while the Build.Experiments doesn't have '+luci.use_realms' value")
			continue
		}

		if _, ok := batchReqs[proj]; !ok {
			batchReqs[proj] = &rdbPb.BatchCreateInvocationsRequest{}
		}
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
		}
		batchReqs[proj].Requests = append(batchReqs[proj].Requests, reqForBldID)
		b.Proto.Infra.Resultdb.Invocation = fmt.Sprintf("invocations/%s", reqForBldID.InvocationId)
		invNameToBuild[b.Proto.Infra.Resultdb.Invocation] = b

		// The invocation for the build id is always created. But if the build have the Number field populated,
		// we will created another invocation for the build number in which it refer to the invocation for build id.
		if b.Proto.Number > 0 {
			sha256Builder := sha256.Sum256([]byte(protoutil.FormatBuilderID(b.Proto.Builder)))
			reqForBldNum := &rdbPb.CreateInvocationRequest{
				InvocationId: fmt.Sprintf("build-%s-%d", hex.EncodeToString(sha256Builder[:]), b.Proto.Number),
				Invocation: &rdbPb.Invocation{
					State:               rdbPb.Invocation_FINALIZING,
					ProducerResource:    reqForBldID.Invocation.ProducerResource,
					Realm:               realm,
					IncludedInvocations: []string{fmt.Sprintf("invocations/%s", reqForBldID.InvocationId)},
				},
			}
			batchReqs[proj].Requests = append(batchReqs[proj].Requests, reqForBldNum)
		}
	}

	if len(batchReqs) == 0 {
		return nil
	}
	// make BatchCreateInvocations calls
	resps := make([]*rdbPb.BatchCreateInvocationsResponse, len(batchReqs))
	err := parallel.WorkPool(64, func(ch chan<- func() error) {
		count := 0
		for proj, req := range batchReqs {
			i := count
			count++
			proj, req := proj, req
			// build-<first build id>+<number of other builds in the batch>
			req.RequestId = fmt.Sprintf("%s+%d", req.Requests[0].InvocationId, len(req.Requests))
			ch <- func() error {
				client, err := newRecorderClient(ctx, rdbHost, proj)
				if err != nil {
					return err
				}
				resps[i], err = client.BatchCreateInvocations(ctx, req)
				return err
			}
		}
	})
	if err != nil {
		return errors.Annotate(err, "failed to make BatchCreateInvocations calls").Err()
	}

	// populate the ResultDBUpdateToken in Build.
	for _, res := range resps {
		for i, invRes := range res.Invocations {
			if build, ok := invNameToBuild[invRes.Name]; ok {
				build.ResultDBUpdateToken = res.UpdateTokens[i]
			}
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
