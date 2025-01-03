// Copyright 2024 The LUCI Authors.
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

package treetest

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	tspb "go.chromium.org/luci/tree_status/proto/v1"
)

func TestFakeSever(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fs := NewFakeServer(ctx)
	t.Cleanup(fs.Shutdown)

	const lProject = "infra"

	fs.ModifyState(lProject, tspb.GeneralState_CLOSED)
	status, err := fs.GetStatus(ctx, &tspb.GetStatusRequest{
		Name: fmt.Sprintf("trees/%s/status/latest", lProject),
	})
	assert.NoErr(t, err)
	assert.That(t, status.GeneralState, should.Equal(tspb.GeneralState_CLOSED))

	fs.InjectErr(lProject, errors.New("bad"))
	status, err = fs.GetStatus(ctx, &tspb.GetStatusRequest{
		Name: fmt.Sprintf("trees/%s/status/latest", lProject),
	})
	assert.ErrIsLike(t, err, "bad")
	assert.Loosely(t, status, should.BeNil)
}
