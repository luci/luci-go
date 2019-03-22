// Copyright 2018 The LUCI Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes"
	structpb "github.com/golang/protobuf/ptypes/struct"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
)

// Step encapsulates a buildbucketpb.Step, and also allows it to carry
// nesting information.
type Step struct {
	*buildbucketpb.Step
	Children  []*Step
	Collapsed bool
	Interval  common.Interval
}

// ShortName returns the leaf name of a potentially nested step.
// Eg. With a name of GrandParent|Parent|Child, this returns "Child"
func (s *Step) ShortName() string {
	parts := strings.Split(s.Name, "|")
	if len(parts) == 0 {
		return "ERROR: EMPTY NAME"
	}
	return parts[len(parts)-1]
}

// Build wraps a buildbucketpb.Build to provide useful templating functions.
type Build struct {
	*buildbucketpb.Build
}

// BuildPage represents a build page on Milo.
// The core of the build page is the underlying build proto, but can contain
// extra information depending on the context, for example a blamelist,
// and the user's display preferences.
type BuildPage struct {
	// Build is the underlying build proto for the build page.
	Build

	// RelatedBuilds are build summaries with the same buildset.
	RelatedBuilds []*Build

	// Related is a flag for turning on the related builds tab.
	Related bool

	// Blame is a list of people and commits that likely caused the build result.
	// It is usually used as the list of commits between the previous run of the
	// build on the same builder, and this run.
	Blame []*Commit

	// BuildBugLink is a URL to be used a feedback link for the build. If the
	// link could not be generated an empty string will be returned. There will be
	// no link, for example, if the project has not set up their build bug template.
	BuildBugLink string

	// Errors contains any non-critical errors encountered while rendering the page.
	Errors []error

	// Mode to render the steps.
	StepDisplayPref StepDisplayPref

	// timelineData caches the results from Timeline().
	timelineData string

	// steps caches the result of Steps().
	steps []*Step

	// Now is the current time, used for generating step intervals.
	Now time.Time

	// BlamelistError holds errors related to the blamelist.
	// This determines the behavior of clicking the "blamelist" tab.
	BlamelistError error

	// ForcedBlamelist indicates that the user forced a blamelist load.
	ForcedBlamelist bool
}

func NewBuildPage(c context.Context, b *buildbucketpb.Build) *BuildPage {
	return &BuildPage{
		Build: Build{b},
		Now:   clock.Now(c),
	}
}

// Summary returns a summary of failures in the build.
// TODO(hinoka): Remove after recipe engine emits SummaryMarkdown natively.
func (b *Build) Summary() (result []string) {
	for _, step := range b.Steps {
		parts := strings.Split(step.Name, "|")
		name := parts[len(parts)-1]
		if name == "Failure reason" {
			continue
		}
		switch step.Status {
		case buildbucketpb.Status_INFRA_FAILURE:
			result = append(result, "Infra Failure "+name)
		case buildbucketpb.Status_FAILURE:
			result = append(result, "Failure "+name)
		}
	}
	return
}

// Steps converts the flat Steps from the underlying Build into a tree.
// The tree is only calculated on the first call, all subsequent calls return cached information.
// TODO(hinoka): Print nicer error messages instead of panicking for invalid build protos.
func (bp *BuildPage) Steps() []*Step {
	if bp.steps != nil {
		return bp.steps
	}
	collapseGreen := bp.StepDisplayPref == StepDisplayDefault
	// Use a map to store all the known steps, so that children can find their parents.
	// This assumes that parents will always be traversed before children,
	// which is always true in the build proto.
	stepMap := map[string]*Step{}
	for _, step := range bp.Build.Steps {
		i := common.ToInterval(step.GetStartTime(), step.GetEndTime())
		i.Now = bp.Now
		s := &Step{
			Step:      step,
			Collapsed: collapseGreen && step.Status == buildbucketpb.Status_SUCCESS,
			Interval:  i,
		}
		stepMap[step.Name] = s
		switch nameParts := strings.Split(step.Name, "|"); len(nameParts) {
		case 0:
			panic("Invalid build.proto: Step with missing name.")
		case 1:
			// Root step.
			bp.steps = append(bp.steps, s)
		default:
			parentName := step.Name[:strings.LastIndex(step.Name, "|")]
			parent, ok := stepMap[parentName]
			if !ok {
				panic("Invalid build.proto: Missing parent.")
			}
			parent.Children = append(parent.Children, s)
		}
	}
	return bp.steps
}

// Status returns a human friendly string for the status.
func (b *Build) HumanStatus() string {
	switch b.Status {
	case buildbucketpb.Status_SCHEDULED:
		return "Pending"
	case buildbucketpb.Status_STARTED:
		return "Running"
	case buildbucketpb.Status_SUCCESS:
		return "Success"
	case buildbucketpb.Status_FAILURE:
		return "Failure"
	case buildbucketpb.Status_INFRA_FAILURE:
		return "Infra Failure"
	case buildbucketpb.Status_CANCELED:
		return "Cancelled"
	default:
		return "Unknown status"
	}
}

type property struct {
	// Name is the name of the property relative to a build.
	// Note: We call this a "Name" not a "Key", since this was the term used in BuildBot.
	Name string
	// Value is a JSON string of the value.
	Value string
}

// properties returns the values in the proto struct fields as
// a json rendered slice of pairs, sorted by key.
func properties(props *structpb.Struct) []property {
	if props == nil {
		return nil
	}
	// Render the fields to JSON.
	m := jsonpb.Marshaler{}
	buf := bytes.NewBuffer(nil)
	if err := m.Marshal(buf, props); err != nil {
		panic(err) // This shouldn't happen.
	}
	d := json.NewDecoder(buf)
	jsonProps := map[string]json.RawMessage{}
	if err := d.Decode(&jsonProps); err != nil {
		panic(err) // This shouldn't happen.
	}

	// Sort the names.
	names := make([]string, 0, len(jsonProps))
	for n := range jsonProps {
		names = append(names, n)
	}
	sort.Strings(names)

	// Rearrange the fields into a slice.
	results := make([]property, len(jsonProps))
	for i, n := range names {
		buf.Reset()
		json.Indent(buf, jsonProps[n], "", "  ")
		results[i] = property{
			Name:  n,
			Value: buf.String(),
		}
	}
	return results
}

func (bp *BuildPage) InputProperties() []property {
	return properties(bp.GetInput().GetProperties())
}

func (bp *BuildPage) OutputProperties() []property {
	return properties(bp.GetOutput().GetProperties())
}

// BuilderLink returns a link to the builder in b.
func (b *Build) BuilderLink() *Link {
	if b.Builder == nil {
		panic("Invalid build")
	}
	builder := b.Builder
	return NewLink(
		builder.Builder,
		fmt.Sprintf("/p/%s/builders/%s/%s", builder.Project, builder.Bucket, builder.Builder),
		fmt.Sprintf("Builder %s in bucket %s", builder.Builder, builder.Bucket))
}

// Link is a self link to the build.
func (b *Build) Link() *Link {
	if b.Builder == nil {
		panic("invalid build")
	}
	num := b.Id
	if b.Number != 0 {
		num = int64(b.Number)
	}
	builder := b.Builder
	return NewLink(
		fmt.Sprintf("%d", num),
		fmt.Sprintf("/p/%s/builders/%s/%s/%d", builder.Project, builder.Bucket, builder.Builder, num),
		fmt.Sprintf("Build %d", num))
}

// Banner returns names of icons to display next to the build number.
// Currently displayed:
// * OS, as determined by swarming dimensions.
// TODO(hinoka): For device builders, display device type, and number of devices.
func (b *Build) Banners() (result []Logo) {
	var os, ver string
	// A swarming dimension may have multiple values.  Eg.
	// Linux, Ubuntu, Ubuntu-14.04.  We want the most specific one.
	// The most specific one always comes last.
	for _, dim := range b.GetInfra().GetSwarming().GetBotDimensions() {
		if dim.Key != "os" {
			continue
		}
		os = dim.Value
		parts := strings.SplitN(os, "-", 2)
		if len(parts) == 2 {
			os = parts[0]
			ver = parts[1]
		}
	}
	var base LogoBase
	switch os {
	case "Ubuntu":
		base = Ubuntu
	case "Windows":
		base = Windows
	case "Mac":
		base = OSX
	case "Android":
		base = Android
	default:
		return
	}
	return []Logo{{
		LogoBase: base,
		Subtitle: ver,
		Count:    1,
	}}
}

// StepDisplayPref is the display preference for the steps.
type StepDisplayPref string

const (
	// StepDisplayDefault means that all steps are visible, green steps are
	// collapsed.
	StepDisplayDefault StepDisplayPref = "default"
	// StepDisplayExpanded means that all steps are visible, nested steps are
	// expanded.
	StepDisplayExpanded StepDisplayPref = "expanded"
	// StepDisplayNonGreen means that only non-green steps are visible, nested
	// steps are expanded.
	StepDisplayNonGreen StepDisplayPref = "non-green"
)

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

// Timeline returns a JSON parsable string that can be fed into a viz timeline component.
func (bp *BuildPage) Timeline() string {
	// Return the cached version, if it exists already.
	if bp.timelineData != "" {
		return bp.timelineData
	}

	// stepData is extra data to deliver with the groups and items (see below) for the
	// Javascript vis Timeline component. Note that the step data is encoded in markdown
	// in the step.SummaryMarkdown field. We do not show this data on the timeline at this
	// time.
	type stepData struct {
		Label           string `json:"label"`
		Duration        string `json:"duration"`
		LogURL          string `json:"logUrl"`
		StatusClassName string `json:"statusClassName"`
	}

	// group corresponds to, and matches the shape of, a Group for the Javascript
	// vis Timeline component http://visjs.org/docs/timeline/#groups. Data
	// rides along as an extra property (unused by vis Timeline itself) used
	// in client side rendering. Each Group is rendered as its own row in the
	// timeline component on to which Items are rendered. Currently we only render
	// one Item per Group, that is one thing per row.
	type group struct {
		ID   string   `json:"id"`
		Data stepData `json:"data"`
	}

	// item corresponds to, and matches the shape of, an Item for the Javascript
	// vis Timeline component http://visjs.org/docs/timeline/#items. Data
	// rides along as an extra property (unused by vis Timeline itself) used
	// in client side rendering. Each Item is rendered to a Group which corresponds
	// to a row. Currently we only render one Item per Group, that is one thing per
	// row.
	type item struct {
		ID        string   `json:"id"`
		Group     string   `json:"group"`
		Start     int64    `json:"start"`
		End       int64    `json:"end"`
		Type      string   `json:"type"`
		ClassName string   `json:"className"`
		Data      stepData `json:"data"`
	}

	groups := make([]group, len(bp.Build.Steps))
	items := make([]item, len(bp.Build.Steps))
	for i, step := range bp.Build.Steps {
		groupID := strconv.Itoa(i)
		logURL := ""
		if len(step.Logs) > 0 {
			logURL = html.EscapeString(step.Logs[0].ViewUrl)
		}
		statusClassName := fmt.Sprintf("status-%s", step.Status)
		data := stepData{
			Label:           html.EscapeString(step.Name),
			Duration:        common.Duration(step.StartTime, step.EndTime, bp.Now),
			LogURL:          logURL,
			StatusClassName: statusClassName,
		}
		groups[i] = group{groupID, data}
		start, _ := ptypes.Timestamp(step.StartTime)
		end, _ := ptypes.Timestamp(step.EndTime)
		items[i] = item{
			ID:        groupID,
			Group:     groupID,
			Start:     milliseconds(start),
			End:       milliseconds(end),
			Type:      "range",
			ClassName: statusClassName,
			Data:      data,
		}
	}

	timeline, err := json.Marshal(map[string]interface{}{
		"groups": groups,
		"items":  items,
	})
	if err != nil {
		bp.Errors = append(bp.Errors, err)
		return "error"
	}
	return string(timeline)
}

// milliseconds returns the given time in number of milliseconds elapsed since epoch.
func milliseconds(time time.Time) int64 {
	return time.UnixNano() / 1e6
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
func NewPatchLink(cl buildbucketpb.BuildSet) *Link {
	switch v := cl.(type) {
	case *buildbucketpb.GerritChange:
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
