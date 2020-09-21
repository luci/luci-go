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

package eval

import (
	"context"
	"time"
)

// RejectedPatchSet is a patchset rejected by CQ.
type RejectedPatchSet struct {
	Patchset  GerritPatchset `json:"patchset"`
	Timestamp time.Time      `json:"timestamp"`

	// FailedTests are the tests that caused the rejection.
	FailedTests []*Test `json:"failedTests"`
}

// rejectedPatchSetSource retrieves rejected patchsets and the tests that caused
// the rejection.
type rejectedPatchSetSource struct {
	*evalRun
}

func (s *rejectedPatchSetSource) Read(ctx context.Context) ([]*RejectedPatchSet, error) {
	// TODO(crbug.com/1112125): implement.
	panic("not implemented")
}
