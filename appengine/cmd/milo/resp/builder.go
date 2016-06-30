// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package resp

import "time"

// BuildSummary is a summary of a build, with just enough information for display
// on a builders page, with an optional field to return the whole build
// information if available.
type BuildSummary struct {
	// Link to the build.
	Link *Link

	// Status of the build.
	Status Status

	// When did this build start.
	Started time.Time

	// When did this build finish.
	Finished time.Time

	// The time it took for this build to finish.  If unfinished, this is the
	// current elapsed duration.
	Duration time.Duration

	// Revision is the main revision of the build.
	// TODO(hinoka): Maybe use a commit object instead?
	Revision string

	// Arbitrary text to display below links.  One line per entry,
	// newlines are stripped.
	Text []string

	// Blame is for tracking whose change the build belongs to, if any.
	Blame []*Commit

	// Build is a reference to the full underlying MiloBuild, if it's available.
	// The only reason this would be calculated is if populating the BuildSummary
	// requires fetching the entire build anyways.  This is assumed to not
	// be available.
	Build *MiloBuild
}

// Builder denotes an ordered list of MiloBuilds
type Builder struct {
	// Name of the builder
	Name string

	// Warning text, if any.
	Warning string

	CurrentBuilds  []*BuildSummary
	PendingBuilds  []*BuildSummary
	FinishedBuilds []*BuildSummary

	// MachinePool is primarily used by buildbot builders to list the set of
	// machines that can run in a builder.  It has no meaning in buildbucket or dm
	// and is expected to be nil.
	MachinePool *MachinePool
}

// MachinePool represents the capacity and availability of a builder.
type MachinePool struct {
	Connected int
	Total     int
	Free      int
	Used      int
}
