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

package exe

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// ModifyBuildOutput runs your callback with process-exclusive access to modify
// the current build's Output message.
//
// The Build will be queued for sending on the return of `cb`.
//
// You must not retain pointers to anything you assign to the Build_Output
// message after the return of `cb`, or this could lead to tricky race
// conditions.
//
// Returns an error if `ctx` is missing build; passes through `cb`'s result
// otherwise.
func ModifyBuildOutput(ctx context.Context, cb func(*bbpb.Build_Output) error) error {
	b := getBuild(ctx)

	return b.modLock(func() error {
		b.outMu.Lock()
		defer b.outMu.Unlock()
		return cb(b.build.Output)
	})
}

// BuildStatus is a temporary struct for containing the Build's global 'status'
// properties.
//
// Used by ModifyBuildStatus.
type BuildStatus struct {
	SummaryMarkdown string
	Critical        bbpb.Trinary
	Status          bbpb.Status
	StatusDetails   *bbpb.StatusDetails
}

// ModifyBuildStatus runs your callback with process-exclusive access to modify
// the current build's various 'status' fields.
//
// The Build will be queued for sending on the return of `cb`.
//
// You must not retain pointers to anything you assign to the BuildStatus
// message after the return of `cb`, or this could lead to tricky race
// conditions.
//
// Returns an error if `ctx` is missing build; passes through `cb`'s result
// otherwise.
func ModifyBuildStatus(ctx context.Context, cb func(*BuildStatus) error) error {
	b := getBuild(ctx)

	return b.modLock(func() error {
		b.statusMu.Lock()
		defer b.statusMu.Unlock()
		bs := &BuildStatus{
			SummaryMarkdown: b.build.SummaryMarkdown,
			Critical:        b.build.Critical,
			Status:          b.build.Status,
			StatusDetails:   b.build.StatusDetails,
		}

		err := cb(bs)
		b.build.SummaryMarkdown = bs.SummaryMarkdown
		b.build.Critical = bs.Critical
		b.build.Status = bs.Status
		b.build.StatusDetails = bs.StatusDetails
		return err
	})
}
