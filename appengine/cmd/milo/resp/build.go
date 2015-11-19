// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate stringer -type=Status,ComponentType

package resp

import "encoding/json"

// MiloBuild denotes a full renderable Milo build page.
type MiloBuild struct {
	// Navi is the very top bar, used for Title and Navigation.
	Navi *Navigation

	// Summary is a top level summary of the page.
	Summary []*BuildComponent

	// Components is a detailed list of components and subcomponents of the page.
	// This is most often used for steps (buildbot/luci) or deps (luci).
	Components []*BuildComponent

	// PropertyGroup is a list of input and output property of this page.
	// This is used for build and emitted properties (buildbot) and quest
	// information (luci).  This is also grouped by the "type" of property
	// so different categories of properties can be separted and sorted.
	PropertyGroup []*PropertyGroup

	// Blame is a list of people and commits that is likely to be in relation to
	// the thing displayed on this page.
	Blame []*Commit

	// CurrentTime is when the page was constructed.
	CurrentTime string
}

// Property specifies the source of the property. k/v pair representing some
// sort of property, such as buildbot property, quest property, etc.
type Property struct {
	Key   string
	Value string
}

// PropertyGroup is a cluster of similar properties.  In buildbot land this would be the "source".
// This is a way to segregate different types of properties such as Quest properties,
// swarming properties, emitted properties, revision properties, etc.
type PropertyGroup struct {
	GroupName string
	Property  []*Property
}

// Commit represents a single commit to a repository, rendered as part of a blamelist.
type Commit struct {
	// Who made the commit?
	AuthorName string
	// Email of the committer.
	AuthorEmail string
	// Full URL of the main source repository.
	Repo string
	// Revision of the commit or base commit.
	Revision string
	// The commit message.
	Description string
	// The commit title, usually the first line of the commit message.
	Title string
	// Rietveld or Gerrit URL.
	ChangelistURL string
	// Browsable URL of the commit.
	CommitURL string
	// List of changed filenames.
	File []string
}

// BuildProgress is a way to show progress.  Percent should always be specified.
type BuildProgress struct {
	// The total number of entries. Shows up as a tooltip.  Leave at 0 to
	// disable the tooltip.
	total uint64

	// The number of entries completed.  Shows up as <progress> / <total>.
	progress uint64

	// A number between 0 to 100 denoting the percentage completed.
	percent uint32
}

// Status is a discrete status for the purpose of colorizing a component.
// These are based off the Material Design Bootstrap color palettes.
type Status int

const (
	// NotRun if the component has not yet been run.
	NotRun Status = iota // 100 Gray

	// Running if the component is currently running.
	Running // 100 Teal

	// Success if the component has finished executing and is not noteworthy.
	Success // A200 Green

	// Failure if the component has finished executing and contains a failure.
	Failure // A200 Red

	// InfraFailure if the component has finished incompletely due to a failure in infra.
	InfraFailure // A100 Purple

	// DependencyFailure if the component has finished incompletely due to a failure in a
	// dependency.
	DependencyFailure // 100 Amber

	// WaitingDependency if the component has finished or paused execution due to an
	// incomplete dep.
	WaitingDependency // 100 Brown
)

// MarshalJSON renders enums into String rather than an int when marshalling.
func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// ComponentType is the type of build component.
type ComponentType int

const (
	// Recipe corresponds to a full recipe run.  Dependencies are recipes.
	Recipe ComponentType = iota

	// Step is a single step of a recipe.
	Step
)

// MarshalJSON renders enums into String rather than an int when marshalling.
func (c ComponentType) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// BuildComponent represents a single Step, subsetup, attempt, or recipe.
type BuildComponent struct {
	// The main label for the component.
	Label string

	// Status of the build.
	Status Status

	// Link to show adjacent to the main label.
	MainLink *Link

	// Links to show right below the main label.
	SubLink []*Link

	// Designates the progress of the current component. Set null for no progress.
	Progress *BuildProgress

	// When did this step start. In RFC3339 format. Set nil if not started.
	Started string

	// When did this step finish. In RFC3339 format.  Set nil if not finished.
	Finished string

	// The time it took for this step to finish in seconds.  If unfinished, this
	// is the current elapsed duration.
	Duration uint64

	// The type of component.  This manifests itself as a little label on the
	// top left corner of the component.
	// This is either "RECIPE" or "STEP".  An attempt is considered a recipe.
	Type ComponentType

	// Specifies if this is a top level or a dependency.  Manifests itself as an
	// indentation level.  Valid options are 0 and 1.  Anything more than 1 is
	// automatically truncated to 1.
	LevelsDeep uint32

	// Arbitrary text to display below links.  One line per entry,
	// newlines are stripped.
	Text []string
}

// Navigation is the top bar of the page, used for navigating out of the page.
type Navigation struct {
	// Title of the site, is generally consistent throughout the whole site.
	SiteTitle *Link

	// Title of the page, usually talks about what's currently here
	PageTitle *Link

	// List of clickable thing to navigate down the hierarchy.
	Breadcrumbs []*Link

	// List of (smaller) clickable things to display on the right side.
	Right []*Link
}

// Link denotes a single labeled link.
type Link struct {
	// Title of the link.  Shows up as the main label.
	Label string

	// An icon for the link.  Not compatible with label.  Rendered as <img>
	Img string

	// A subtitle for the link.  Can be used in conjunction with label or img.
	Subtitle string

	// The destination for the link, stuck in a <a href> tag.
	URL string

	// Alt text for the image, only supported with img.
	Alt string
}
