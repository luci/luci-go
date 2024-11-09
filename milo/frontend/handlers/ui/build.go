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
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/utils"
)

var crosMainRE = regexp.MustCompile(`^cros/parent_buildbucket_id/(\d+)$`)

// Step encapsulates a buildbucketpb.Step, and also allows it to carry
// nesting information.
type Step struct {
	*buildbucketpb.Step
	Children  []*Step        `json:"children,omitempty"`
	Collapsed bool           `json:"collapsed,omitempty"`
	Interval  utils.Interval `json:"interval,omitempty"`
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
// It is used in both BuildPage (in this file) and BuilderPage (builder.go).
type Build struct {
	*buildbucketpb.Build

	// Now is the current time, used to generate durations that may depend
	// on the current time.
	Now *timestamppb.Timestamp `json:"now,omitempty"`
}

// CommitLinkHTML returns an HTML link pointing to the output commit, or input commit
// if the output commit is not available.
func (b *Build) CommitLinkHTML() template.HTML {
	c := b.GetOutput().GetGitilesCommit()
	if c == nil {
		c = b.GetInput().GetGitilesCommit()
	}
	if c == nil {
		return ""
	}

	// Choose a link label.
	var label string
	switch {
	case c.Position != 0:
		label = fmt.Sprintf("%s@{#%d}", c.Ref, c.Position)
	case strings.HasPrefix(c.Ref, "refs/tags/"):
		label = c.Ref
	case c.Id != "":
		label = c.Id
	case c.Ref != "":
		label = c.Ref
	default:
		return ""
	}

	return NewLink(label, protoutil.GitilesCommitURL(c), "commit "+label).HTML()
}

// BuildPage represents a build page on Milo.
// The core of the build page is the underlying build proto, but can contain
// extra information depending on the context, for example a blamelist,
// and the user's display preferences.
type BuildPage struct {
	// Build is the underlying build proto for the build page.
	Build

	// Blame is a list of people and commits that likely caused the build result.
	// It is usually used as the list of commits between the previous run of the
	// build on the same builder, and this run.
	Blame []*Commit `json:"blame,omitempty"`

	// BuildbucketHost is the hostname for the buildbucket instance this build came from.
	BuildbucketHost string `json:"buildbucket_host,omitempty"`

	// Errors contains any non-critical errors encountered while rendering the page.
	Errors []error `json:"errors,omitempty"`

	// Mode to render the steps.
	StepDisplayPref StepDisplayPref `json:"step_display_pref,omitempty"`

	// Iff true, show all log links whose name starts with '$'.
	ShowDebugLogsPref bool `json:"show_debug_logs_pref,omitempty"`

	// timelineData caches the results from Timeline().
	timelineData string

	// steps caches the result of Steps().
	steps []*Step

	// BlamelistError holds errors related to the blamelist.
	// This determines the behavior of clicking the "blamelist" tab.
	BlamelistError error `json:"blamelist_error,omitempty"`

	// ForcedBlamelist indicates that the user forced a blamelist load.
	ForcedBlamelist bool `json:"forced_blamelist,omitempty"`

	// Whether the user is able to perform certain actions on this build
	CanCancel bool `json:"can_cancel,omitempty"`
	CanRetry  bool `json:"can_retry,omitempty"`
}

// RelatedBuildsTable represents a related builds table on Milo.
type RelatedBuildsTable struct {
	// Build is the underlying build proto for the build page.
	Build `json:"build,omitempty"`

	// RelatedBuilds are build summaries with the same buildset.
	RelatedBuilds []*Build `json:"related_builds,omitempty"`
}

func NewBuildPage(c context.Context, b *buildbucketpb.Build) *BuildPage {
	now := timestamppb.New(clock.Now(c))
	return &BuildPage{
		Build: Build{Build: b, Now: now},
	}
}

// ChangeLinks returns a slice of links to build input gerrit changes.
func (b *Build) ChangeLinks() []*Link {
	changes := b.GetInput().GetGerritChanges()
	ret := make([]*Link, len(changes))
	for i, c := range changes {
		ret[i] = NewPatchLink(c)
	}
	return ret
}

func (b *Build) RecipeLink() *Link {
	projectName := b.GetBuilder().GetProject()
	cipdPackage := b.GetExe().GetCipdPackage()
	recipeName := b.GetInput().GetProperties().GetFields()["recipe"].GetStringValue()
	// We don't know location of recipes within the repo and getting that
	// information is not trivial, so use code search, which is precise enough.
	csHost := "source.chromium.org"
	if strings.Contains(cipdPackage, "internal") {
		csHost = "source.corp.google.com"
	}
	// TODO(crbug.com/1149540): remove this conditional once the long-term
	// solution for recipe links has been implemented.
	if projectName == "flutter" {
		csHost = "cs.opensource.google"
	}
	u := url.URL{
		Scheme: "https",
		Host:   csHost,
		Path:   "/search/",
		RawQuery: url.Values{
			"q": []string{fmt.Sprintf(`file:recipes/%s.py`, recipeName)},
		}.Encode(),
	}
	return NewLink(recipeName, u.String(), fmt.Sprintf("recipe %s", recipeName))
}

// BuildbucketLink returns a link to the buildbucket version of the page.
func (bp *BuildPage) BuildbucketLink() *Link {
	if bp.BuildbucketHost == "" {
		return nil
	}
	u := url.URL{
		Scheme: "https",
		Host:   bp.BuildbucketHost,
		Path:   "/rpcexplorer/services/buildbucket.v2.Builds/GetBuild",
		RawQuery: url.Values{
			"request": []string{fmt.Sprintf(`{"id":"%d"}`, bp.Id)},
		}.Encode(),
	}
	return NewLink(
		fmt.Sprintf("%d", bp.Id),
		u.String(),
		"Buildbucket RPC explorer for build")
}

func (b *Build) BuildSets() []string {
	return protoutil.BuildSets(b.Build)
}

func (b *Build) BuildSetLinks() []template.HTML {
	buildSets := b.BuildSets()
	links := make([]template.HTML, 0, len(buildSets))
	for _, buildSet := range buildSets {
		result := crosMainRE.FindStringSubmatch(buildSet)
		if result == nil {
			// Don't know how to link, just return the text.
			links = append(links, template.HTML(template.HTMLEscapeString(buildSet)))
		} else {
			// This linking is for legacy ChromeOS builders to show a link to their
			// main builder, it can be removed when these have been transitioned
			// to parallel CQ.
			buildbucketId := result[1]
			builderURL := fmt.Sprintf("https://ci.chromium.org/b/%s", buildbucketId)
			ariaLabel := fmt.Sprintf("Main builder %s", buildbucketId)
			link := NewLink(buildSet, builderURL, ariaLabel)
			links = append(links, link.HTML())
		}
	}
	return links
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
			Interval:  utils.ToInterval(step.GetStartTime(), step.GetEndTime(), bp.Now),
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

// HumanStatus returns a human friendly string for the status.
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
		return "Canceled"
	default:
		return "Unknown status"
	}
}

// ShouldShowCanaryWarning returns true for failed canary builds.
func (b *Build) ShouldShowCanaryWarning() bool {
	return b.Canary && (b.Status == buildbucketpb.Status_FAILURE || b.Status == buildbucketpb.Status_INFRA_FAILURE)
}

type property struct {
	// Name is the name of the property relative to a build.
	// Note: We call this a "Name" not a "Key", since this was the term used in BuildBot.
	Name string `json:"name,omitempty"`
	// Value is a JSON string of the value.
	Value string `json:"value,omitempty"`
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
	// Prefer build number below, but if using buildbucket ID
	// a b prefix is needed on the buildbucket ID for it to work.
	numStr := fmt.Sprintf("b%d", num)
	if b.Number != 0 {
		num = int64(b.Number)
		numStr = strconv.FormatInt(num, 10)
	}
	builder := b.Builder
	return NewLink(
		fmt.Sprintf("%d", num),
		fmt.Sprintf("/p/%s/builders/%s/%s/%s", builder.Project, builder.Bucket, builder.Builder, numStr),
		fmt.Sprintf("Build %d", num))
}

// Banners returns names of icons to display next to the build number.
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
	AuthorName string `json:"author_name,omitempty"`
	// Email of the committer.
	AuthorEmail string `json:"author_email,omitempty"`
	// Time of the commit.
	CommitTime time.Time `json:"commit_time,omitempty"`
	// Full URL of the main source repository.
	Repo string `json:"repo,omitempty"`
	// Branch of the repo.
	Branch string `json:"branch,omitempty"`
	// Requested revision of the commit or base commit.
	RequestRevision *Link `json:"request_revision,omitempty"`
	// Revision of the commit or base commit.
	Revision *Link `json:"revision,omitempty"`
	// The commit message.
	Description string `json:"description,omitempty"`
	// Rietveld or Gerrit URL if the commit is a patch.
	Changelist *Link `json:"changelist,omitempty"`
	// Browsable URL of the commit.
	CommitURL string `json:"commit_url,omitempty"`
	// List of changed filenames.
	File []string `json:"file,omitempty"`
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

	now := bp.Now.AsTime()

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
			Duration:        utils.Duration(step.StartTime, step.EndTime, bp.Now),
			LogURL:          logURL,
			StatusClassName: statusClassName,
		}
		groups[i] = group{groupID, data}
		start := step.StartTime.AsTime()
		end := step.EndTime.AsTime()
		if end.IsZero() || end.Before(start) {
			end = now
		}
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

	timeline, err := json.Marshal(map[string]any{
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
					`{{else}}{{.Label}}{{end}}` +
					`</a>`))

	linkifySetTemplate = template.Must(
		template.New("linkifySet").
			Parse(
				`{{ range $i, $link := . }}` +
					`{{ if gt $i 0 }} {{ end }}` +
					`{{ $link.HTML }}` +
					`{{ end }}`))
	newBuildPageOptInTemplate = template.Must(
		template.New("buildOptIn").
			Parse(`
				<div id="opt-in-banner">
					<div id="opt-in-link">
						Switch to
						<a
							id="new-build-page-link"
							{{if .Number}}
								href="/ui/p/{{.Builder.Project}}/builders/{{.Builder.Bucket}}/{{.Builder.Builder}}/{{.Number}}"
							{{else}}
								href="/ui/p/{{.Builder.Project}}/builders/{{.Builder.Bucket}}/{{.Builder.Builder}}/b{{.Id}}"
							{{end}}
						>the new build page!</a>
					</div>
					<div id="feedback-bar">
						Or <a id="feedback-link" href="/">tell us what's missing</a>.
						[<a id="dismiss-feedback-bar" href="/">dismiss</a>]
					</div>
				</div>`))
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

// NewBuildPageOptInHTML returns a link to the new build page of the build.
func (b *Build) NewBuildPageOptInHTML() template.HTML {
	buf := bytes.Buffer{}
	if err := newBuildPageOptInTemplate.Execute(&buf, b); err != nil {
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
	AriaLabel string `json:"aria_label,omitempty"`

	// Img is an icon for the link.  Not compatible with label.  Rendered as <img>
	Img string `json:"img,omitempty"`

	// Alt text for the image, or title text with text link.
	Alt string `json:"alt,omitempty"`
}

// NewLink does just about what you'd expect.
func NewLink(label, url, ariaLabel string) *Link {
	return &Link{Link: model.Link{Label: label, URL: url}, AriaLabel: ariaLabel}
}

// NewPatchLink generates a URL to a Gerrit CL.
func NewPatchLink(cl *buildbucketpb.GerritChange) *Link {
	return NewLink(
		fmt.Sprintf("CL %d (ps#%d)", cl.Change, cl.Patchset),
		protoutil.GerritChangeURL(cl),
		fmt.Sprintf("gerrit changelist number %d patchset %d", cl.Change, cl.Patchset))
}

// NewEmptyLink creates a Link struct acting as a pure text label.
func NewEmptyLink(label string) *Link {
	return &Link{Link: model.Link{Label: label}}
}

// BuildPageData represents a build page on Milo.
// Comparing to BuildPage, it caches a lot of the computed properties so they
// be serialised to JSON.
type BuildPageData struct {
	*BuildPage
	CommitLinkHTML          template.HTML   `json:"commit_link_html,omitempty"`
	Summary                 []string        `json:"summary,omitempty"`
	RecipeLink              *Link           `json:"recipe_link,omitempty"`
	BuildbucketLink         *Link           `json:"buildbucket_link,omitempty"`
	BuildSets               []string        `json:"build_sets,omitempty"`
	BuildSetLinks           []template.HTML `json:"build_set_links,omitempty"`
	Steps                   []*Step         `json:"steps,omitempty"`
	HumanStatus             string          `json:"human_status,omitempty"`
	ShouldShowCanaryWarning bool            `json:"should_show_canary_warning,omitempty"`
	InputProperties         []property      `json:"input_properties,omitempty"`
	OutputProperties        []property      `json:"output_properties,omitempty"`
	BuilderLink             *Link           `json:"builder_link,omitempty"`
	Link                    *Link           `json:"link,omitempty"`
	Banners                 []Logo          `json:"banners,omitempty"`
	Timeline                string          `json:"timeline,omitempty"`
}
