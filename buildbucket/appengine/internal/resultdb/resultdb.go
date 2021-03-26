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
// Note: this function will mutate the value of build.Proto.Infra.Resultdb.Invocation and build.ResultDBUpdateToken.
func CreateInvocations(ctx context.Context, builds []*model.Build, cfgs map[string]map[string]*pb.Builder) error {
	// TODO(yuanjunh): in next CL, filter out builds which resultDB isn't enabled; fetch settings.cfg for rdbHost; get proper bbHost.
	rdbHost := "rdbHost"
	bbHost := "bbHost"

	// We need to do one batch request per project, since the rpc to create invocations uses per-project credentials.
	batchReqs := make(map[string]*rdbPb.BatchCreateInvocationsRequest)
	for _, b := range builds {
		proj := b.Proto.Builder.Project
		if _, ok := batchReqs[proj]; !ok {
			batchReqs[proj] = &rdbPb.BatchCreateInvocationsRequest{}
		}
		cfg := cfgs[protoutil.FormatBucketID(proj, b.Proto.Builder.Bucket)][b.Proto.Builder.Builder]
		invReq1 := &rdbPb.CreateInvocationRequest{
			InvocationId: fmt.Sprintf("build-%d", b.Proto.Id),
			Invocation: &rdbPb.Invocation{
				BigqueryExports:  cfg.Resultdb.BqExports,
				ProducerResource: fmt.Sprintf("//%s/builds/%d", bbHost, b.Proto.Id),
				Realm:            b.Realm(),
				HistoryOptions: &rdbPb.HistoryOptions{
					UseInvocationTimestamp: cfg.Resultdb.HistoryOptions.UseInvocationTimestamp,
				},
			},
		}
		batchReqs[proj].Requests = append(batchReqs[proj].Requests, invReq1)
		b.Proto.Infra.Resultdb.Invocation = fmt.Sprintf("invocations/%s", invReq1.InvocationId)
		sha256Builder := sha256.Sum256([]byte(protoutil.FormatBuilderID(b.Proto.Builder)))
		if b.Proto.Number > 0 {
			invReq2 := &rdbPb.CreateInvocationRequest{
				InvocationId: fmt.Sprintf("build-%s-%d", hex.EncodeToString(sha256Builder[:]), b.Proto.Number),
				Invocation: &rdbPb.Invocation{
					State:               rdbPb.Invocation_FINALIZING,
					ProducerResource:    invReq1.Invocation.ProducerResource,
					Realm:               b.Realm(),
					IncludedInvocations: []string{fmt.Sprintf("invocations/%s", invReq1.InvocationId)},
				},
			}
			batchReqs[proj].Requests = append(batchReqs[proj].Requests, invReq2)
		}
	}

	resps := make([]*rdbPb.BatchCreateInvocationsResponse, len(batchReqs))
	err := parallel.FanOutIn(func(ch chan<- func() error) {
		count := 0
		for proj, req := range batchReqs {
			i := count
			count++
			req, proj := req, proj
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

	// The invocations count may be larger than builds count, because for the build with Number field, we create two invocations.
	// However, there are exactly as many UpdateTokens as there are invocations, and their indices are expected to match.
	// The invToToken map is the invocation name -> the update token.
	// The invocation name in the response is the same as the one we set in b.Proto.Infra.Resultdb.Invocation earlier in this function.
	invToToken := make(map[string]string)
	for _, res := range resps {
		for i, inv := range res.Invocations {
			invToToken[inv.Name] = res.UpdateTokens[i]
		}
	}
	for _, b := range builds {
		b.ResultDBUpdateToken = invToToken[b.Proto.Infra.Resultdb.Invocation]
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
