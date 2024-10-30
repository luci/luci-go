// Copyright 2023 The LUCI Authors.
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

package rdb

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestRecorderClient(t *testing.T) {
	ftt.Run(`MarkInvocationSubmitted`, t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		mcf := NewMockRecorderClientFactory(ct.GoMockCtl)
		rc, err := mcf.MakeClient(ctx, "rdbhost")
		assert.NoErr(t, err)

		t.Run(`OK`, func(t *ftt.Test) {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, nil)

			err := rc.MarkInvocationSubmitted(ctx, inv)
			assert.NoErr(t, err)
		})

		t.Run(`Permission Denied`, func(t *ftt.Test) {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, appstatus.Error(codes.PermissionDenied, "permission denied"))

			err := rc.MarkInvocationSubmitted(ctx, inv)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("failed to mark %s submitted", inv)))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run(`Invalid Argument`, func(t *ftt.Test) {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, appstatus.Error(codes.InvalidArgument, "invalid argument"))

			err := rc.MarkInvocationSubmitted(ctx, inv)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("failed to mark %s submitted", inv)))
		})

		t.Run(`No Code`, func(t *ftt.Test) {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, errors.New("random error"))

			err := rc.MarkInvocationSubmitted(ctx, inv)
			_, ok := appstatus.Get(err)
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			assert.Loosely(t, err, should.ErrLike("random error"))
		})

		t.Run(`Transient Error`, func(t *ftt.Test) {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, appstatus.Error(codes.Unknown, "???"))

			err := rc.MarkInvocationSubmitted(ctx, inv)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.Unknown))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			assert.Loosely(t, err, should.ErrLike("???"))
		})
	})
}
