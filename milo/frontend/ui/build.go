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
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/milo/common/model"
)

// Step encapsulates a buildbucketpb.Step, and also allows it to carry
// nesting information.
type Step struct {
	*buildbucketpb.Step
	Children  []*Step
	Collapsed bool
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

// BuildPage represents a build page on Milo.
// The core of the build page is the underlying build proto, but can contain
// extra information depending on the context, for example a blamelist,
// and the user's display preferences.
type BuildPage struct {
	// Build is the underlying build proto for the build page.
	buildbucketpb.Build

	// Blame is a list of people and commits that likely caused the build result.
	// It is usually used as the list of commits between the previous run of the
	// build on the same builder, and this run.
	Blame []*Commit

	// Errors contains any non-critical errors encountered while rendering the page.
	Errors []error

	// Mode to render the steps.
	StepDisplayPref StepDisplayPref

	// timelineData caches the results from Timeline().
	timelineData string

	// steps caches the result of Steps().
	steps []*Step
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
		s := &Step{
			Step:      step,
			Collapsed: collapseGreen && step.Status == buildbucketpb.Status_SUCCESS,
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

func (bp *BuildPage) Builder() *Link {
	if bp.Build.Builder == nil {
		panic("Invalid build")
	}
	b := bp.Build.Builder
	return NewLink(
		b.Builder,
		fmt.Sprintf("/p/%s/builders/%s/%s", b.Project, b.Bucket, b.Builder),
		fmt.Sprintf("Builder %s in bucket %s", b.Builder, b.Bucket))
}

func (bp *BuildPage) BuildID() *Link {
	if bp.Build.Builder == nil {
		panic("invalid build")
	}
	num := bp.Build.Id
	if bp.Build.Number != 0 {
		num = int64(bp.Build.Number)
	}
	b := bp.Build.Builder
	return NewLink(
		fmt.Sprintf("%d", num),
		fmt.Sprintf("/p/%s/builder/%s/%s/%d", b.Project, b.Bucket, b.Builder, num),
		fmt.Sprintf("Build %d", num))
}

// Banner returns names of icons to display next to the build number.
// Currently displayed:
// * OS, as determined by swarming dimensions.
// TODO(hinoka): For device builders, display device type, and number of devices.
func (bp *BuildPage) Banners() (result []Logo) {
	if bp.Infra == nil {
		return
	}
	if bp.Infra.Swarming == nil {
		return
	}
	for _, dim := range bp.Infra.Swarming.BotDimensions {
		if dim.Key != "os" {
			continue
		}
		os := dim.Value
		parts := strings.SplitN(os, "-", 2)
		var ver string
		if len(parts) == 2 {
			os = parts[0]
			ver = parts[1]
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
		result = append(result, Logo{
			LogoBase: base,
			Subtitle: ver,
			Count:    1,
		})
	}
	return // We didn't find an OS dimension.
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
// TODO(hinoka): Reimplement me.
func (bp *BuildPage) Timeline() string {
	// Return the cached version, if it exists already.
	if bp.timelineData != "" {
		return bp.timelineData
	}
	// TODO(hinoka): This doesn't currently return correct data.
	return "\"timeline goes here\""

	// stepData is extra data to deliver with the groups and items (see below) for the
	// Javascript vis Timeline component.
	type stepData struct {
		Label           string    `json:"label"`
		Text            string    `json:"text"`
		Duration        string    `json:"duration"`
		MainLink        LinkSet   `json:"mainLink"`
		SubLink         []LinkSet `json:"subLink"`
		StatusClassName string    `json:"statusClassName"`
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
		statusClassName := fmt.Sprintf("status-%s", step.Status)
		data := stepData{
			Label: html.EscapeString(step.Name),
			Text:  html.EscapeString(step.SummaryMarkdown),
			// TODO(hinoka): HumanDuration
			Duration:        "duration goes here",
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

func sanitize(values []string) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = html.EscapeString(value)
	}
	return result
}

func sanitizeLinkSet(linkSet LinkSet) LinkSet {
	result := make(LinkSet, len(linkSet))
	for i, link := range linkSet {
		result[i] = &Link{
			Link: model.Link{
				Label: html.EscapeString(link.Label),
				URL:   html.EscapeString(link.URL),
			},
			AriaLabel: html.EscapeString(link.AriaLabel),
			Img:       html.EscapeString(link.Img),
			Alt:       html.EscapeString(link.Alt),
		}
	}
	return result
}

func sanitizeLinkSets(linkSets []LinkSet) []LinkSet {
	result := make([]LinkSet, len(linkSets))
	for i, linkSet := range linkSets {
		result[i] = sanitizeLinkSet(linkSet)
	}
	return result
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
