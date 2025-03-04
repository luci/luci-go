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

package cv

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	cvv0 "go.chromium.org/luci/cv/api/v0"
)

func TestGetRun(t *testing.T) {
	t.Parallel()

	ftt.Run("Get run", t, func(t *ftt.Test) {
		rID := "projects/chromium/runs/run-id"
		runs := map[string]*cvv0.Run{
			rID: {},
		}
		ctx := UseFakeClient(context.Background(), runs)
		client, err := NewClient(ctx, "host")
		assert.NoErr(t, err)
		req := &cvv0.GetRunRequest{
			Id: rID,
		}
		_, err = client.GetRun(ctx, req)
		assert.NoErr(t, err)
	})
}
