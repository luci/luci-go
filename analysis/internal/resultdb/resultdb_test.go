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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestResultDB(t *testing.T) {
	t.Parallel()
	ftt.Run(`resultdb`, t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := NewMockedClient(context.Background(), ctl)
		rc, err := NewClient(mc.Ctx, "rdbhost", "project")
		assert.Loosely(t, err, should.BeNil)

		inv := "invocations/build-87654321"

		t.Run(`GetInvocation`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invProto, should.Resemble(res))
		})
	})
}
