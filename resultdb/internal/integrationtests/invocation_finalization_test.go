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
	"testing"
	"time"

	"go.chromium.org/luci/grpc/prpc"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/resultdb/internal/recorder"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInvocatoinFinalization(t *testing.T) {
	Convey(`ShouldFinalize`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		app, err := newTestApp(ctx)
		So(err, ShouldBeNil)
		go app.ListenAndServe()
		defer app.Shutdown()

		// Create invocations 1, 2 and 3.
		createInv := func(id string) (updateToken string) {
			md := metadata.MD{}
			req := &pb.CreateInvocationRequest{InvocationId: id}
			_, err := app.recorder.CreateInvocation(ctx, req, prpc.Header(&md))
			So(err, ShouldBeNil)
			So(md.Get(recorder.UpdateTokenMetadataKey), ShouldHaveLength, 1)
			updateToken = md.Get(recorder.UpdateTokenMetadataKey)[0]
			return
		}

		getState := func(name string) pb.Invocation_State {
			inv, err := app.resultdb.GetInvocation(ctx, &pb.GetInvocationRequest{Name: name})
			So(err, ShouldBeNil)
			return inv.State
		}

		include := func(including, included, updateToken string) {
			ctx := metadata.AppendToOutgoingContext(ctx, recorder.UpdateTokenMetadataKey, updateToken)
			_, err = app.recorder.Include(ctx, &pb.IncludeRequest{
				IncludingInvocation: including,
				IncludedInvocation:  included,
			})
			So(err, ShouldBeNil)
		}

		finalizeInv := func(name, updateToken string) {
			ctx := metadata.AppendToOutgoingContext(ctx, recorder.UpdateTokenMetadataKey, updateToken)
			_, err := app.recorder.FinalizeInvocation(ctx, &pb.FinalizeInvocationRequest{Name: name})
			So(err, ShouldBeNil)
		}

		invAToken := createInv("u:a")
		invBToken := createInv("u:b")
		invCToken := createInv("u:c")

		// Make a include b and b include x.
		include("invocations/u:a", "invocations/u:b", invAToken)
		include("invocations/u:b", "invocations/u:c", invBToken)

		// Finalize invocations/u:a.
		finalizeInv("invocations/u:a", invAToken)
		So(getState("invocations/u:a"), ShouldEqual, pb.Invocation_FINALIZING)

		// Finalize invocations/u:b.
		finalizeInv("invocations/u:b", invBToken)
		So(getState("invocations/u:b"), ShouldEqual, pb.Invocation_FINALIZING)

		// Finalize invocations/u:c.
		finalizeInv("invocations/u:c", invCToken)
		// Assert that all three invocations are finalized within 10 seconds.
		ctx, _ = context.WithTimeout(ctx, 10*time.Second)
		for {
			time.Sleep(100 * time.Millisecond)
			if getState("invocations/u:a") != pb.Invocation_FINALIZED {
				continue
			}
			if getState("invocations/u:b") != pb.Invocation_FINALIZED {
				continue
			}
			if getState("invocations/u:c") != pb.Invocation_FINALIZED {
				continue
			}

			break
		}
	})
}
