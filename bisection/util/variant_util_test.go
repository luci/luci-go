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

package util

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/bisection/proto/v1"
)

func TestVariantUtil(t *testing.T) {
	ftt.Run("VariantPB", t, func(t *ftt.Test) {
		t.Run("no error", func(t *ftt.Test) {
			variant, err := VariantPB(`{"builder": "testbuilder"}`)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, variant, should.Resemble(&pb.Variant{
				Def: map[string]string{"builder": "testbuilder"},
			}))
		})
		t.Run("error", func(t *ftt.Test) {
			variant, err := VariantPB("invalid json")
			assert.Loosely(t, err, should.ErrLike("invalid"))
			assert.Loosely(t, variant, should.BeNil)
		})
	})

}
