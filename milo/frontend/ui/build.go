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

//go:generate stringer -type=Verbosity
//go:generate stringer -type=ComponentType

package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/milo/common/model"
)

// MiloBuild denotes a full renderable Milo build page.
type MiloBuild struct {
	// Summary is a top level summary of the page.
	Summary BuildComponent

	// Trigger gives information about how and with what information
	// the build was triggered.
	Trigger *Trigger

	// Components is a detailed list of components and subcomponents of the page.
	// This is most often used for steps (buildbot/luci) or deps (luci).
	Components []*BuildComponent

	// PropertyGroup is a list of input and output property of this page.
	// This is used for build and emitted properties (buildbot) and quest
	// information (luci).  This is also grouped by the "type" of property
	// so different categories of properties can be separated and sorted.
	//
	// This is not a map so code that constructs MiloBuild can control the
	// order of property groups, for example show important properties
	// first.
	PropertyGroup []*PropertyGroup

	// Blame is a list of people and commits that is likely to be in relation to
	// the thing displayed on this page.
	Blame []*Commit
}

// BuildSummary returns the BuildSummary representation of the MiloBuild.  This
// is the subset of fields that is interesting to the builder view.
func (b *MiloBuild) BuildSummary() *BuildSummary {
	if b == nil {
		return nil
	}
	result := &BuildSummary{
		Link:          b.Summary.Label,
		Status:        b.Summary.Status,
		PendingTime:   b.Summary.PendingTime,
		ExecutionTime: b.Summary.ExecutionTime,
		Text:          b.Summary.Text,
		Blame:         b.Blame,
	}
	if b.Trigger != nil {
		result.Revision = &b.Trigger.Commit
	}
	return result
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

// Commit represents a single commit to a repository, rendered as part of a blamelist.
type Commit struct {
	// Who made the commit?
	AuthorName string
	// Email of the committer.
	AuthorEmail string
	// Time of the commit.
	CommitTime time.Time
	// Full URL of the main source repository.
	Repo string
	// Branch of the repo.
	Branch string
	// Requested revision of the commit or base commit.
	RequestRevision *Link
	// Revision of the commit or base commit.
	Revision *Link
	// The commit message.
	Description string
	// Rietveld or Gerrit URL if the commit is a patch.
	Changelist *Link
	// Browsable URL of the commit.
	CommitURL string
	// List of changed filenames.
	File []string
}

// RevisionHTML returns a single rendered link for the revision, prioritizing
// Revision over RequestRevision.
func (c *Commit) RevisionHTML() template.HTML {
	switch {
	case c == nil:
		return ""
	case c.Revision != nil:
		return c.Revision.HTML()
	case c.RequestRevision != nil:
		return c.RequestRevision.HTML()
	default:
		return ""
	}
}

// Title is the first line of the commit message (Description).
func (c *Commit) Title() string {
	switch lines := strings.SplitN(c.Description, "\n", 2); len(lines) {
	case 0:
		return ""
	case 1:
		return c.Description
	default:
		return lines[0]
	}
}

// DescLines returns the description as a slice, one line per item.
func (c *Commit) DescLines() []string {
	return strings.Split(c.Description, "\n")
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

// ComponentType is the type of build component.
type ComponentType int

const (
	// Recipe corresponds to a full recipe run.  Dependencies are recipes.
	Recipe ComponentType = iota

	// Step is a single step of a recipe.
	Step

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

// Verbosity can be tagged onto a BuildComponent to indicate whether it should
// be hidden or annuciated.
type Verbosity int

const (
	// Normal items are displayed as usual.  This is the default.
	Normal Verbosity = iota

	// Hidden items are by default not displayed.
	Hidden

	// Interesting items are a signal that they should be annuciated, or
	// pre-fetched.
	Interesting
)

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
	Status model.Status

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

	// Specifies if this is a top level or a dependency.  Manifests itself as an
	// indentation level.  Valid options are 0 and 1.  Anything more than 1 is
	// automatically truncated to 1.
	LevelsDeep uint32

	// Verbosity indicates how important this step is.
	Verbosity Verbosity

	// Arbitrary text to display below links.  One line per entry,
	// newlines are stripped.
	Text []string
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

// Link denotes a single labeled link.
//
// JSON tags here are for test expectations.
type Link struct {
	model.Link

	// AriaLabel is a spoken label for the link.  Used as aria-label under the anchor tag.
	AriaLabel string `json:",omitempty"`

	// Img is an icon for the link.  Not compatible with label.  Rendered as <img>
	Img string `json:",omitempty"`

	// Alt text for the image, or title text with text link.
	Alt string `json:",omitempty"`

	// Alias, if true, means that this link is an [alias link].
	Alias bool `json:",omitempty"`
}

// NewLink does just about what you'd expect.
func NewLink(label, url, ariaLabel string) *Link {
	return &Link{Link: model.Link{Label: label, URL: url}, AriaLabel: ariaLabel}
}

// NewPatchLink is the right way (TM) to generate links to Rietveld/Gerrit CLs.
//
// Returns nil if provided buildset is not Rietveld or Gerrit CL.
func NewPatchLink(cl buildbucket.BuildSet) *Link {
	switch v := cl.(type) {
	case *buildbucket.GerritChange:
		return NewLink(
			fmt.Sprintf("Gerrit CL %d (ps#%d)", v.Change, v.Patchset),
			v.URL(),
			fmt.Sprintf("gerrit changelist number %d patchset %d", v.Change, v.Patchset))
	default:
		return nil
	}
}

// NewEmptyLink creates a Link struct acting as a pure text label.
func NewEmptyLink(label string) *Link {
	return &Link{Link: model.Link{Label: label}}
}

/// HTML methods.

var (
	linkifyTemplate = template.Must(
		template.New("linkify").
			Parse(
				`<a{{if .URL}} href="{{.URL}}"{{end}}` +
					`{{if .AriaLabel}} aria-label="{{.AriaLabel}}"{{end}}` +
					`{{if .Alt}}{{if not .Img}} title="{{.Alt}}"{{end}}{{end}}>` +
					`{{if .Img}}<img src="{{.Img}}"{{if .Alt}} alt="{{.Alt}}"{{end}}>` +
					`{{else if .Alias}}[{{.Label}}]` +
					`{{else}}{{.Label}}{{end}}` +
					`</a>`))

	linkifySetTemplate = template.Must(
		template.New("linkifySet").
			Parse(
				`{{ range $i, $link := . }}` +
					`{{ if gt $i 0 }} {{ end }}` +
					`{{ $link.HTML }}` +
					`{{ end }}`))
)

// HTML renders this Link as HTML.
func (l *Link) HTML() template.HTML {
	if l == nil {
		return ""
	}
	buf := bytes.Buffer{}
	if err := linkifyTemplate.Execute(&buf, l); err != nil {
		panic(err)
	}
	return template.HTML(buf.Bytes())
}

// String renders this Link's Label as a string.
func (l *Link) String() string {
	if l == nil {
		return ""
	}
	return l.Label
}

// HTML renders this LinkSet as HTML.
func (l LinkSet) HTML() template.HTML {
	if len(l) == 0 {
		return ""
	}
	buf := bytes.Buffer{}
	if err := linkifySetTemplate.Execute(&buf, l); err != nil {
		panic(err)
	}
	return template.HTML(buf.Bytes())
}
