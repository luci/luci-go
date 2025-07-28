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

package build

import (
	"context"
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestErrors(t *testing.T) {
	ftt.Run(`Errors`, t, func(t *ftt.Test) {
		t.Run(`nil`, func(t *ftt.Test) {
			var err error

			assert.Loosely(t, AttachStatus(err, bbpb.Status_INFRA_FAILURE, &bbpb.StatusDetails{
				ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
			}), should.BeNil)

			status, details := ExtractStatus(err)
			assert.Loosely(t, status, should.Match(bbpb.Status_SUCCESS))
			assert.Loosely(t, details, should.BeNil)
		})

		t.Run(`err`, func(t *ftt.Test) {
			t.Run(`generic`, func(t *ftt.Test) {
				err := errors.New("some error")
				status, details := ExtractStatus(err)
				assert.Loosely(t, status, should.Match(bbpb.Status_FAILURE))
				assert.Loosely(t, details, should.BeNil)
			})

			t.Run(`AttachStatus`, func(t *ftt.Test) {
				err := errors.New("some error")
				err2 := AttachStatus(err, bbpb.Status_INFRA_FAILURE, &bbpb.StatusDetails{
					ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
				})

				status, details := ExtractStatus(err2)
				assert.Loosely(t, status, should.Match(bbpb.Status_INFRA_FAILURE))
				assert.Loosely(t, details, should.Match(&bbpb.StatusDetails{
					ResourceExhaustion: &bbpb.StatusDetails_ResourceExhaustion{},
				}))
			})

			t.Run(`context`, func(t *ftt.Test) {
				status, details := ExtractStatus(context.Canceled)
				assert.Loosely(t, status, should.Match(bbpb.Status_CANCELED))
				assert.Loosely(t, details, should.BeNil)

				status, details = ExtractStatus(context.DeadlineExceeded)
				assert.Loosely(t, status, should.Match(bbpb.Status_INFRA_FAILURE))
				assert.Loosely(t, details, should.Match(&bbpb.StatusDetails{
					Timeout: &bbpb.StatusDetails_Timeout{},
				}))
			})
		})

		t.Run(`AttachStatus panics for bad status`, func(t *ftt.Test) {
			assert.Loosely(t, func() {
				AttachStatus(nil, bbpb.Status_STARTED, nil)
			}, should.PanicLike("cannot be used with non-terminal status \"STARTED\""))
		})
	})
}
