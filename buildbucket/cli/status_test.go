// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestStatusFlag(t *testing.T) {
	t.Parallel()

	ftt.Run("StatusFlag", t, func(t *ftt.Test) {
		var status pb.Status
		f := StatusFlag(&status)

		t.Run("Get", func(t *ftt.Test) {
			assert.Loosely(t, f.Get(), should.Equal(status))
			status = pb.Status_SUCCESS
			assert.Loosely(t, f.Get(), should.Equal(status))
		})

		t.Run("String", func(t *ftt.Test) {
			assert.Loosely(t, f.String(), should.BeEmpty)
			status = pb.Status_SUCCESS
			assert.Loosely(t, f.String(), should.Equal("success"))
		})

		t.Run("Set", func(t *ftt.Test) {
			t.Run("success", func(t *ftt.Test) {
				assert.Loosely(t, f.Set("success"), should.BeNil)
				assert.Loosely(t, status, should.Equal(pb.Status_SUCCESS))
			})
			t.Run("SUCCESS", func(t *ftt.Test) {
				assert.Loosely(t, f.Set("SUCCESS"), should.BeNil)
				assert.Loosely(t, status, should.Equal(pb.Status_SUCCESS))
			})
			t.Run("unspecified", func(t *ftt.Test) {
				assert.Loosely(t, f.Set("unspecified"), should.ErrLike(`invalid status "unspecified"; expected one of canceled, ended, failure, infra_failure, scheduled, started, success`))
			})
			t.Run("garbage", func(t *ftt.Test) {
				assert.Loosely(t, f.Set("garbage"), should.ErrLike(`invalid status "garbage"; expected one of canceled, ended, failure, infra_failure, scheduled, started, success`))
			})
		})
	})
}
