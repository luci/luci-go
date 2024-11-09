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
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/resultdb/internal/services/recorder"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// testClient is a convenient resultdb client, to keep tests simple.
// Asserts that all requests succeed.
// Memorizes update tokens of invocations it created.
type testClient struct {
	app *testApp

	updateTokens map[string]string
}

func (c *testClient) CreateInvocation(ctx context.Context, id string) {
	md := metadata.MD{}
	req := &pb.CreateInvocationRequest{InvocationId: id, Invocation: &pb.Invocation{Realm: "testproject:testrealm"}}
	inv, err := c.app.Recorder.CreateInvocation(ctx, req, grpc.Header(&md))
	assert.Loosely(c.app.t, err, should.BeNil, truth.LineContext())
	assert.Loosely(c.app.t, md.Get(pb.UpdateTokenMetadataKey), should.HaveLength(1))

	if c.updateTokens == nil {
		c.updateTokens = map[string]string{}
	}
	c.updateTokens[inv.Name] = md.Get(pb.UpdateTokenMetadataKey)[0]
}

func (c *testClient) withUpdateTokenFor(ctx context.Context, invocation string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, pb.UpdateTokenMetadataKey, c.updateTokens[invocation])
}

func (c *testClient) GetState(ctx context.Context, name string) pb.Invocation_State {
	inv, err := c.app.ResultDB.GetInvocation(ctx, &pb.GetInvocationRequest{Name: name})
	assert.Loosely(c.app.t, err, should.BeNil)
	return inv.State
}

func (c *testClient) Include(ctx context.Context, including, included string) {
	ctx = c.withUpdateTokenFor(ctx, including)
	_, err := c.app.Recorder.UpdateIncludedInvocations(ctx, &pb.UpdateIncludedInvocationsRequest{
		IncludingInvocation: including,
		AddInvocations:      []string{included},
	})
	assert.Loosely(c.app.t, err, should.BeNil)
}

func (c *testClient) FinalizeInvocation(ctx context.Context, name string) {
	ctx = c.withUpdateTokenFor(ctx, name)
	_, err := c.app.Recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: name})
	assert.Loosely(c.app.t, err, should.BeNil)
}

// MakeInvocationOverdue uses a magic constant to set an invocation's deadline
// sometime in the past.
func (c *testClient) MakeInvocationOverdue(ctx context.Context, name string) {
	ctx = c.withUpdateTokenFor(ctx, name)
	_, err := c.app.Recorder.UpdateInvocation(ctx, &pb.UpdateInvocationRequest{
		Invocation: &pb.Invocation{
			Name:     name,
			Deadline: pbutil.MustTimestampProto(time.Unix(recorder.TestMagicOverdueDeadlineUnixSecs, 0).UTC()),
		},
		UpdateMask: &field_mask.FieldMask{
			Paths: []string{"deadline"},
		},
	})
	assert.Loosely(c.app.t, err, should.BeNil)
}
