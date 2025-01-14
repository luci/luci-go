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

package lucictx

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLUCIExe(t *testing.T) {
	ftt.Run(`test luciexe`, t, func(t *ftt.Test) {
		t.Run(`can set and clear in ctx`, func(t *ftt.Test) {
			ctx := context.Background()
			ctx = SetLUCIExe(ctx, &LUCIExe{CacheDir: "hello"})
			assert.Loosely(t, GetLUCIExe(ctx), should.Resemble(&LUCIExe{CacheDir: "hello"}))

			ctx = SetLUCIExe(ctx, nil)
			assert.Loosely(t, GetLUCIExe(ctx), should.Resemble((*LUCIExe)(nil)))
		})
	})
}
