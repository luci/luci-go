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
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/emptypb"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/retry/transient"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestRecorderClient(t *testing.T) {
	Convey(`MarkInvocationSubmitted`, t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		mcf := NewMockRecorderClientFactory(ct.GoMockCtl)
		rc, err := mcf.MakeClient(ctx, "rdbhost")
		So(err, ShouldBeNil)

		Convey(`OK`, func() {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, nil)

			err := rc.MarkInvocationSubmitted(ctx, inv)
			So(err, ShouldBeNil)
		})

		Convey(`Permission Denied`, func() {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, appstatus.Error(codes.PermissionDenied, "permission denied"))

			err := rc.MarkInvocationSubmitted(ctx, inv)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, fmt.Sprintf("failed to mark %s submitted", inv))
			So(transient.Tag.In(err), ShouldBeFalse)
		})

		Convey(`Invalid Argument`, func() {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, appstatus.Error(codes.InvalidArgument, "invalid argument"))

			err := rc.MarkInvocationSubmitted(ctx, inv)
			So(err, ShouldHaveAppStatus, codes.InvalidArgument)
			So(transient.Tag.In(err), ShouldBeFalse)
			So(err, ShouldErrLike, fmt.Sprintf("failed to mark %s submitted", inv))
		})

		Convey(`No Code`, func() {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, errors.New("random error"))

			err := rc.MarkInvocationSubmitted(ctx, inv)
			_, ok := appstatus.Get(err)
			So(ok, ShouldBeFalse)
			So(transient.Tag.In(err), ShouldBeFalse)
			So(err, ShouldErrLike, "random error")
		})

		Convey(`Transient Error`, func() {
			inv := "invocations/build-100001"
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: inv,
			})).Return(&emptypb.Empty{}, appstatus.Error(codes.Unknown, "???"))

			err := rc.MarkInvocationSubmitted(ctx, inv)
			So(err, ShouldHaveAppStatus, codes.Unknown)
			So(transient.Tag.In(err), ShouldBeTrue)
			So(err, ShouldErrLike, "???")
		})
	})
}
