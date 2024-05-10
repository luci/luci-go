// Copyright 2022 The LUCI Authors.
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
	"testing"

	"github.com/golang/mock/gomock"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestResultDB(t *testing.T) {
	t.Parallel()
	Convey(`resultdb`, t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := NewMockedClient(context.Background(), ctl)
		rc, err := NewClient(mc.Ctx, "rdbhost", "project")
		So(err, ShouldBeNil)

		inv := "invocations/build-87654321"

		Convey(`GetInvocation`, func() {
			realm := "realm"
			req := &rdbpb.GetInvocationRequest{
				Name: inv,
			}
			res := &rdbpb.Invocation{
				Name:  inv,
				Realm: realm,
			}
			mc.GetInvocation(req, res)

			invProto, err := rc.GetInvocation(mc.Ctx, inv)
			So(err, ShouldBeNil)
			So(invProto, ShouldResembleProto, res)
		})
	})
}
