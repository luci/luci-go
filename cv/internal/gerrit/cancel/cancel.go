// Copyright 2021 The LUCI Authors.
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

// Package cancel implements canceling triggers or Runs by removing CQ+1 or CQ+2
// votes and posting comment.
package cancel

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/run"
)

// ErrPreconditionFailedTag is an error tag indicating that Cancel precondition
// failed.
var ErrPreconditionFailedTag = errors.BoolTag{
	Key: errors.NewTagKey("cancel precondition not met"),
}

// Cancel cancels either an existing Run or just a CQ+1/CQ+2 trigger, which
// couldn't be converted to a Run for some reason.
//
// Posts comment if not empty OR if CQ votes couldn't be removed due to
// lack of permission, in which case special "botdata" message to the comment.
// Checks Gerrit's CL state before posting comment to avoid duplicates.
//
// Returns nil if cancelation succeeded, even if no action was even necessary.
//
// Returns error tagged with ErrPreconditionFailedTag if cancelation can't be
// done due to:
//   * new patchset was uploaded (vs one in cl.Snapshot)
//   * there is another CQ vote (vs those in cl.Snapshot)
//
// Returns permanent error on fatal failures, such as lack of permission to even
// post comment.
func Cancel(ctx context.Context, cl *changelist.CL, t *run.Trigger, comment string) error {
	if cl.Snapshot == nil {
		panic("cl.Snapshot must be non-nil")
	}
	// TODO(qyearsley): implement.
	// 1. Check cl's latest state in Datastore and if newer, use it to eval
	// precondition and potential bail out early.
	// 2. Fetch cl's latest state from GoB, and if newer, re-eval precondition
	// again.
	// 3. Do mutations, re-checking Gerrit state after failures.
	return errors.New("not implemented")
}
