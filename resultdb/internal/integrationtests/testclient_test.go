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

package integrationtests

import (
	"context"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/resultdb/internal/recorder"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	"google.golang.org/grpc/metadata"

	. "github.com/smartystreets/goconvey/convey"
)

type testClient struct {
	app *testApp

	updateTokens map[string]string
}

func (c *testClient) createInv(ctx context.Context, id string) {
	md := metadata.MD{}
	req := &pb.CreateInvocationRequest{InvocationId: id}
	inv, err := c.app.recorder.CreateInvocation(ctx, req, prpc.Header(&md))
	So(err, ShouldBeNil)
	So(md.Get(recorder.UpdateTokenMetadataKey), ShouldHaveLength, 1)

	if c.updateTokens == nil {
		c.updateTokens = map[string]string{}
	}
	c.updateTokens[inv.Name] = md.Get(recorder.UpdateTokenMetadataKey)[0]
}

func (c *testClient) withUpdateTokenFor(ctx context.Context, invocation string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, recorder.UpdateTokenMetadataKey, c.updateTokens[invocation])
}

func (c *testClient) getState(ctx context.Context, name string) pb.Invocation_State {
	inv, err := c.app.resultdb.GetInvocation(ctx, &pb.GetInvocationRequest{Name: name})
	So(err, ShouldBeNil)
	return inv.State
}

func (c *testClient) include(ctx context.Context, including, included string) {
	ctx = c.withUpdateTokenFor(ctx, including)
	_, err := c.app.recorder.Include(ctx, &pb.IncludeRequest{
		IncludingInvocation: including,
		IncludedInvocation:  included,
	})
	So(err, ShouldBeNil)
}

func (c *testClient) finalize(ctx context.Context, name string) {
	ctx = c.withUpdateTokenFor(ctx, name)
	_, err := c.app.recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: name})
	So(err, ShouldBeNil)
}
