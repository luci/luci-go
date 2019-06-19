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

package ui

import (
	"go.chromium.org/luci/milo/common/model"
)

// BuildSummaryLegacy is a summary of a build, with just enough information for display
// on a builder page.
type BuildSummaryLegacy struct {
	// Link to the build.
	Link *Link

	// Status of the build.
	Status model.Status

	// Pending is time interval that this build was pending.
	PendingTime Interval

	// Execution is time interval that this build was executing.
	ExecutionTime Interval

	// Revision is the main revision of the build.
	Revision *Commit

	// Arbitrary text to display below links.  One line per entry,
	// newlines are stripped.
	Text []string

	// Blame is for tracking whose change the build belongs to, if any.
	Blame []*Commit
}

// BuilderLegacy denotes an ordered list of MiloBuilds
type BuilderLegacy struct {
	// Name of the builder
	Name string

	// Indicates that this Builder should render a blamelist for each build.
	// This is only supported for Buildbot (crbug.com/807846)
	HasBlamelist bool

	// Warning text, if any.
	Warning string

	CurrentBuilds []*BuildSummaryLegacy
	PendingBuilds []*BuildSummaryLegacy
	// PendingBuildNum is the number of pending builds, since the slice above
	// may be a snapshot instead of the full set.
	PendingBuildNum int
	FinishedBuilds  []*BuildSummaryLegacy

	// MachinePool is primarily used by buildbot builders to list the set of
	// machines that can run in a builder.  It has no meaning in buildbucket or dm
	// and is expected to be nil.
	MachinePool *MachinePool

	// Groups is a list of links to builder groups that contain this builder.
	Groups []*Link

	// PrevCursor is a cursor to the previous page.
	PrevCursor string `json:",omitempty"`
	// NextCursor is a cursor to the next page.
	NextCursor string `json:",omitempty"`
}
