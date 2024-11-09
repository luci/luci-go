// Copyright 2015 The LUCI Authors.
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

//go:generate stringer -type=ComponentType

package ui

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/milo/internal/model/milostatus"
)

// MiloBuildLegacy denotes a full renderable Milo build page.
// This is slated to be deprecated in April 2019 after the BuildBot turndown.
// This is to be replaced by a new BuildPage struct,
// which encapsulates a Buildbucket Build Proto.
type MiloBuildLegacy struct {
	// Summary is a top level summary of the page.
	Summary BuildComponent

	// Trigger gives information about how and with what information
	// the build was triggered.
	Trigger *Trigger

	// Components is a detailed list of components and subcomponents of the page.
	// This is most often used for steps or deps.
	Components []*BuildComponent

	// PropertyGroup is a list of input and output property of this page.
	// This is used for build and emitted properties (buildbot) and quest
	// information (luci).  This is also grouped by the "type" of property
	// so different categories of properties can be separated and sorted.
	//
	// This is not a map so code that constructs MiloBuildLegacy can control the
	// order of property groups, for example show important properties
	// first.
	PropertyGroup []*PropertyGroup

	// Blame is a list of people and commits that is likely to be in relation to
	// the thing displayed on this page.
	Blame []*Commit

	// Mode to render the steps.
	StepDisplayPref StepDisplayPref

	// Iff true, show all log links whose name starts with '$'.
	ShowDebugLogsPref bool
}

var statusPrecendence = map[milostatus.Status]int{
	milostatus.Canceled:     0,
	milostatus.Expired:      1,
	milostatus.Exception:    2,
	milostatus.InfraFailure: 3,
	milostatus.Failure:      4,
	milostatus.Warning:      5,
	milostatus.Success:      6,
}

// fixComponent fixes possible display inconsistencies with the build, including:
//
// * If the build is complete, all open steps should be closed.
// * Parent steps containing failed steps should also be marked as failed.
// * Components' Collapsed field is set based on StepDisplayPref field.
// * Parent step name prefix is trimmed from the nested substeps.
// * Enforce correct values for StepDisplayPref (set to Collapsed if incorrect).
func fixComponent(comp *BuildComponent, buildFinished time.Time, stripPrefix string, collapseGreen bool) {
	// If the build is finished but the step is not finished.
	if !buildFinished.IsZero() && comp.ExecutionTime.Finished.IsZero() {
		// Then set the finish time to be the same as the build finish time.
		comp.ExecutionTime.Finished = buildFinished
		comp.ExecutionTime.Duration = buildFinished.Sub(comp.ExecutionTime.Started)
		comp.Status = milostatus.InfraFailure
	}

	// Fix substeps recursively.
	for _, substep := range comp.Children {
		fixComponent(
			substep, buildFinished, comp.Label.String()+".", collapseGreen)
	}

	// When parent step finishes running, compute its final status as worst
	// status, as determined by statusPrecendence map above, among direct children
	// and its own status.
	if comp.Status.Terminal() {
		for _, substep := range comp.Children {
			substepStatusPrecedence, ok := statusPrecendence[substep.Status]
			if ok && substepStatusPrecedence < statusPrecendence[comp.Status] {
				comp.Status = substep.Status
			}
		}
	}

	comp.Collapsed = collapseGreen && comp.Status == milostatus.Success

	// Strip parent component name from the title.
	if comp.Label != nil {
		comp.Label.Label = strings.TrimPrefix(comp.Label.Label, stripPrefix)
	}
}

var (
	farFutureTime = time.Date(2038, time.January, 19, 3, 14, 07, 0, time.UTC)
	farPastTime   = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
)

// fixComponentDuration makes all parent steps have the max duration of all
// of its children.
func fixComponentDuration(c context.Context, comp *BuildComponent) Interval {
	// Leaf nodes do not require fixing.
	if len(comp.Children) == 0 {
		return comp.ExecutionTime
	}
	// Set start and end times to be out of bounds.
	// Each variable can have 3 states:
	// 1. Undefined.  In which case they're set to farFuture/PastTime
	// 2. Current (fin only).  In which case it's set to zero time (time.Time{})
	// 3. Definite.  In which case it's set to a time that isn't either of the above.
	start := farFutureTime
	fin := farPastTime
	for _, subcomp := range comp.Children {
		i := fixComponentDuration(c, subcomp)
		if i.Started.Before(start) {
			start = i.Started
		}
		switch {
		case fin.IsZero():
			continue // fin is current, it can't get any farther in the future than that.
		case i.Finished.IsZero(), i.Finished.After(fin):
			fin = i.Finished // Both of these cased are considered "after".
		}
	}
	comp.ExecutionTime = NewInterval(c, start, fin)
	return comp.ExecutionTime
}

// Fix fixes various inconsistencies that users expect to see as part of the
// Build, but didn't make sense as part of the individual components, including:
// * If the build is complete, all open steps should be closed.
// * Parent steps containing failed steps should also be marked as failed.
// * Components' Collapsed field is set based on StepDisplayPref field.
// * Parent step name prefix is trimmed from the nested substeps.
// * Enforce correct values for StepDisplayPref (set to Default if incorrect).
// * Set parent step durations to be the combination of all children.
func (b *MiloBuildLegacy) Fix(c context.Context) {
	if b.StepDisplayPref != StepDisplayExpanded && b.StepDisplayPref != StepDisplayNonGreen {
		b.StepDisplayPref = StepDisplayDefault
	}

	for _, comp := range b.Components {
		fixComponent(
			comp, b.Summary.ExecutionTime.Finished, "",
			b.StepDisplayPref == StepDisplayDefault)
		// Run duration fixing after component fixing to make sure all of the
		// end times are set correctly.
		fixComponentDuration(c, comp)
	}
}

// Trigger is the combination of pointing to a single commit, with information
// about where that commit came from (e.g. the repository), and the project
// that triggered it.
type Trigger struct {
	Commit
	// Source is the trigger source.  In buildbot, this would be the "Reason".
	// This has no meaning in SwarmBucket and DM yet.
	Source string
	// Project is the name of the LUCI project responsible for
	// triggering the build.
	Project string
}

// Property specifies k/v pair representing some
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

func (p PropertyGroup) Len() int { return len(p.Property) }
func (p PropertyGroup) Swap(i, j int) {
	p.Property[i], p.Property[j] = p.Property[j], p.Property[i]
}
func (p PropertyGroup) Less(i, j int) bool { return p.Property[i].Key < p.Property[j].Key }

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

// ComponentType is the type of build component.
type ComponentType int

const (
	// Recipe corresponds to a full recipe run.  Dependencies are recipes.
	Recipe ComponentType = iota

	// StepLegacy is a single step of a recipe.
	StepLegacy

	// Summary denotes that this does not pretain to any particular step.
	Summary
)

// MarshalJSON renders enums into String rather than an int when marshalling.
func (c ComponentType) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// LogoBanner is a banner of logos that define the OS and devices that a
// component is associated with.
type LogoBanner struct {
	OS     []Logo
	Device []Logo
}

// Interval is a time interval which has a start, an end and a duration.
type Interval struct {
	// Started denotes the start time of this interval.
	Started time.Time
	// Finished denotest the end time of this interval.
	Finished time.Time
	// Duration is the length of the interval.
	Duration time.Duration
}

// NewInterval returns a new interval struct.  If end time is empty (eg. Not completed)
// set end time to empty but set duration to the difference between start and now.
// Getting called with an empty start time and non-empty end time is undefined.
func NewInterval(c context.Context, start, end time.Time) Interval {
	i := Interval{Started: start, Finished: end}
	if start.IsZero() {
		return i
	}
	if end.IsZero() {
		i.Duration = clock.Now(c).Sub(start)
	} else {
		i.Duration = end.Sub(start)
	}
	return i
}

// BuildComponent represents a single Step, subsetup, attempt, or recipe.
type BuildComponent struct {
	// The parent of this component.  For buildbot and swarmbucket builds, this
	// refers to the builder.  For DM, this refers to whatever triggered the Quest.
	ParentLabel *Link `json:",omitempty"`

	// The main label for the component.
	Label *Link

	// Status of the build.
	Status milostatus.Status

	// Banner is a banner of logos that define the OS and devices this
	// component is associated with.
	Banner *LogoBanner `json:",omitempty"`

	// Bot is the machine or execution instance that this component ran on.
	Bot *Link `json:",omitempty"`

	// Recipe is a link to the recipe this component is based on.
	Recipe *Link `json:",omitempty"`

	// Source is a link to the external (buildbot, swarming, dm, etc) data
	// source that this component relates to.
	Source *Link `json:",omitempty"`

	// Links to show adjacent to the main label.
	MainLink LinkSet `json:",omitempty"`

	// Links to show right below the main label. Top-level slice are rows of
	// links, second level shows up as
	SubLink []LinkSet `json:",omitempty"`

	// Designates the progress of the current component. Set null for no progress.
	Progress *BuildProgress `json:",omitempty"`

	// Pending is time interval that this build was pending.
	PendingTime Interval

	// Execution is time interval that this build was executing.
	ExecutionTime Interval

	// The type of component.  This manifests itself as a little label on the
	// top left corner of the component.
	// This is either "RECIPE" or "STEP".  An attempt is considered a recipe.
	Type ComponentType

	// Arbitrary text to display below links.  One line per entry,
	// newlines are stripped.
	Text []string

	// Children of the step. Undefined for other types of build components.
	Children []*BuildComponent

	// Render a step as collapsed or expanded. Undefined for other types of build
	// components.
	Collapsed bool
}

var rLineBreak = regexp.MustCompile("<br */?>")

// TextBR returns Text, but also splits each line by <br>
func (bc *BuildComponent) TextBR() []string {
	var result []string
	for _, t := range bc.Text {
		result = append(result, rLineBreak.Split(t, -1)...)
	}
	return result
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

// LinkSet is an ordered collection of Link objects that will be rendered on the
// same line.
type LinkSet []*Link

// IsDebugLink returns true iff the first link in the linkset starts with "$".
func (l LinkSet) IsDebugLink() bool {
	if len(l) > 0 {
		return strings.HasPrefix(l[0].Label, "$")
	}
	return false
}
