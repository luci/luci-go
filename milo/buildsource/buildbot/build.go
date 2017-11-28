// Copyright 2016 The LUCI Authors.
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

package buildbot

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// getBanner parses the OS information from the build and maybe returns a banner.
func getBanner(c context.Context, b *buildbot.Build) *ui.LogoBanner {
	osLogo := func() *ui.Logo {
		result := &ui.Logo{}
		switch b.OSFamily {
		case "windows":
			result.LogoBase = ui.Windows
		case "Darwin":
			result.LogoBase = ui.OSX
		case "Debian":
			result.LogoBase = ui.Ubuntu
		default:
			return nil
		}
		result.Subtitle = b.OSVersion
		return result
	}()
	if osLogo != nil {
		return &ui.LogoBanner{
			OS: []ui.Logo{*osLogo},
		}
	}
	return nil
}

// summary extracts the top level summary from a buildbot build as a
// BuildComponent
func summary(c context.Context, b *buildbot.Build) ui.BuildComponent {
	// TODO(hinoka): use b.toStatus()
	// Status
	var status model.Status
	if b.Currentstep != nil {
		status = model.Running
	} else {
		status = b.Results.Status()
	}

	// Link to bot and original build.
	host := "build.chromium.org/p"
	if b.Internal {
		host = "uberchromegw.corp.google.com/i"
	}
	bot := ui.NewLink(
		b.Slave,
		fmt.Sprintf("https://%s/%s/buildslaves/%s", host, b.Master, b.Slave),
		fmt.Sprintf("Buildbot buildslave %s", b.Slave))

	var source *ui.Link
	if !b.Emulated {
		source = ui.NewLink(
			fmt.Sprintf("%s/%s/%d", b.Master, b.Buildername, b.Number),
			fmt.Sprintf("https://%s/%s/builders/%s/builds/%d",
				host, b.Master, b.Buildername, b.Number),
			fmt.Sprintf("Build number %d on master %s builder %s", b.Number, b.Master, b.Buildername))
	}

	// The link to the builder page.
	parent := ui.NewLink(b.Buildername, ".", fmt.Sprintf("Parent builder %s", b.Buildername))

	// Do a best effort lookup for the bot information to fill in OS/Platform info.
	banner := getBanner(c, b)

	sum := ui.BuildComponent{
		ParentLabel: parent,
		Label:       fmt.Sprintf("#%d", b.Number),
		Banner:      banner,
		Status:      status,
		Started:     b.Times.Start.Time,
		Finished:    b.Times.Finish.Time,
		Bot:         bot,
		Source:      source,
		Duration:    b.Times.Duration(),
		Type:        ui.Summary, // This is more or less ignored.
		LevelsDeep:  1,
		Text:        mergeText(b.Text), // Status messages.  Eg "This build failed on..xyz"
	}

	return sum
}

var rLineBreak = regexp.MustCompile("<br */?>")

// components takes a full buildbot build struct and extract step info from all
// of the steps and returns it as a list of milo Build Components.
func components(b *buildbot.Build) (result []*ui.BuildComponent) {
	for _, step := range b.Steps {
		if step.Hidden == true {
			continue
		}
		bc := &ui.BuildComponent{
			Label: step.Name,
		}
		// Step text sometimes contains <br>, which we want to parse into new lines.
		for _, t := range step.Text {
			for _, line := range rLineBreak.Split(t, -1) {
				bc.Text = append(bc.Text, line)
			}
		}

		// Figure out the status.
		if !step.IsStarted {
			bc.Status = model.NotRun
		} else {
			bc.Status = step.Results.Status()
		}

		// Raise the interesting-ness if the step is not "Success".
		if bc.Status != model.Success {
			bc.Verbosity = ui.Interesting
		}

		remainingAliases := stringset.New(len(step.Aliases))
		for linkAnchor := range step.Aliases {
			remainingAliases.Add(linkAnchor)
		}

		getLinksWithAliases := func(logLink *ui.Link, isLog bool) ui.LinkSet {
			// Generate alias links.
			var aliases ui.LinkSet
			if remainingAliases.Del(logLink.Label) {
				stepAliases := step.Aliases[logLink.Label]
				aliases = make(ui.LinkSet, len(stepAliases))
				for i, alias := range stepAliases {
					aliases[i] = alias.Link()
				}
			}

			// Step log link takes primary, with aliases as secondary.
			links := make(ui.LinkSet, 1, 1+len(aliases))
			links[0] = logLink

			for _, a := range aliases {
				a.Alias = true
			}
			return append(links, aliases...)
		}

		for _, l := range step.Logs {
			ariaName := l.Name
			switch ariaName {
			case "stdio":
				ariaName = "standard i/o"
			case "stdout":
				ariaName = "standard out"
			case "stderr":
				ariaName = "standard error"
			}
			logLink := ui.NewLink(l.Name, l.URL, fmt.Sprintf("log %s for step %s", ariaName, step.Name))

			links := getLinksWithAliases(logLink, true)
			if logLink.Label == "stdio" {
				bc.MainLink = links
			} else {
				bc.SubLink = append(bc.SubLink, links)
			}
		}

		// Step links are stored as maps of name: url
		// Because Go doesn't believe in nice things, we now create another array
		// just so that we can iterate through this map in order.
		names := make([]string, 0, len(step.Urls))
		for name := range step.Urls {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			logLink := ui.NewLink(name, step.Urls[name], fmt.Sprintf("step link %s for step %s", name, step.Name))

			bc.SubLink = append(bc.SubLink, getLinksWithAliases(logLink, false))
		}

		// Add any unused aliases directly.
		if remainingAliases.Len() > 0 {
			unusedAliases := remainingAliases.ToSlice()
			sort.Strings(unusedAliases)

			for _, label := range unusedAliases {
				var baseLink ui.LinkSet
				for _, alias := range step.Aliases[label] {
					aliasLink := alias.Link()
					if len(baseLink) == 0 {
						aliasLink.Label = label
					} else {
						aliasLink.Alias = true
					}
					baseLink = append(baseLink, aliasLink)
				}

				if len(baseLink) > 0 {
					bc.SubLink = append(bc.SubLink, baseLink)
				}
			}
		}

		// Copy times.
		times := step.Times
		if times.Finish.IsZero() {
			times.Finish = b.Times.Finish
		}
		bc.Started = times.Start.Time
		bc.Finished = times.Finish.Time
		bc.Duration = times.Duration()
		result = append(result, bc)
	}
	return
}

// parseProp returns a string representation of v.
func parseProp(v interface{}) string {
	// if v is a whole number, force it into an int.  json.Marshal() would turn
	// it into what looks like a float instead.  We want this to remain and
	// int instead of a number.
	if vf, ok := v.(float64); ok {
		if math.Floor(vf) == vf {
			return fmt.Sprintf("%d", int64(vf))
		}
	}
	// return the json representation of the value.
	b, err := json.Marshal(v)
	if err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

// Prop is a struct used to store a value and group so that we can make a map
// of key:Prop to pass into parseProp() for the purpose of cross referencing
// one prop while working on another.
type Prop struct {
	Value interface{}
	Group string
}

// properties extracts all properties from buildbot builds and groups them into
// property groups.
func properties(b *buildbot.Build) (result []*ui.PropertyGroup) {
	groups := map[string]*ui.PropertyGroup{}
	allProps := map[string]Prop{}
	for _, prop := range b.Properties {
		allProps[prop.Name] = Prop{
			Value: prop.Value,
			Group: prop.Source,
		}
	}
	for key, prop := range allProps {
		value := prop.Value
		groupName := prop.Group
		if _, ok := groups[groupName]; !ok {
			groups[groupName] = &ui.PropertyGroup{GroupName: groupName}
		}
		vs := parseProp(value)
		groups[groupName].Property = append(groups[groupName].Property, &ui.Property{
			Key:   key,
			Value: vs,
		})
	}
	// Insert the groups into a list in alphabetical order.
	// You have to make a separate sorting data structure because Go doesn't like
	// sorting things for you.
	groupNames := []string{}
	for n := range groups {
		groupNames = append(groupNames, n)
	}
	sort.Strings(groupNames)
	for _, k := range groupNames {
		group := groups[k]
		// Also take this oppertunity to sort the properties within the groups.
		sort.Sort(group)
		result = append(result, group)
	}
	return
}

// blame extracts the commit and blame information from a buildbot build and
// returns it as a list of Commits.
func blame(b *buildbot.Build) (result []*ui.Commit) {
	if b.Sourcestamp != nil {
		for _, c := range b.Sourcestamp.Changes {
			files := c.GetFiles()
			result = append(result, &ui.Commit{
				AuthorEmail: c.Who,
				Repo:        c.Repository,
				CommitTime:  time.Unix(int64(c.When), 0).UTC(),
				Revision:    ui.NewLink(c.Revision, c.Revlink, fmt.Sprintf("commit by %s", c.Who)),
				Description: c.Comments,
				File:        files,
			})
		}
	}
	return
}

// sourcestamp extracts the source stamp from various parts of a buildbot build,
// including the properties.
func sourcestamp(c context.Context, b *buildbot.Build) *ui.Trigger {
	ss := &ui.Trigger{}
	rietveld := ""
	gerrit := ""
	gotRevision := ""
	repository := ""
	issue := int64(-1)
	for _, prop := range b.Properties {
		switch prop.Name {
		case "rietveld":
			if v, ok := prop.Value.(string); ok {
				rietveld = v
			} else {
				logging.Warningf(c, "Field rietveld is not a string: %#v", prop.Value)
			}
		case "issue", "patch_issue":
			// Sometime this is a number (float), sometime it is a string.
			switch v := prop.Value.(type) {
			case float64:
				issue = int64(v)
			case string:
				if v != "" {
					if vi, err := strconv.ParseInt(v, 10, 64); err == nil {
						issue = int64(vi)
					} else {
						logging.Warningf(c, "Could not decode field %s: %q - %s", prop.Name, v, err)
					}
				}
			default:
				logging.Warningf(c, "Field %s is not a string or float64: %#v", prop.Name, v)
			}

		case "got_revision":
			if v, ok := prop.Value.(string); ok {
				gotRevision = v
			} else {
				logging.Warningf(c, "Field got_revision is not a string: %#v", prop.Value)
			}

		case "patch_gerrit_url":
			if v, ok := prop.Value.(string); ok {
				gerrit = v
			} else {
				logging.Warningf(c, "Field gerrit is not a string: %#v", prop.Value)
			}

		case "repository":
			if v, ok := prop.Value.(string); ok {
				repository = v
			}
		}
	}
	if issue != -1 {
		switch {
		case rietveld != "":
			rietveld = strings.TrimRight(rietveld, "/")
			ss.Changelist = ui.NewLink(
				fmt.Sprintf("Rietveld CL %d", issue),
				fmt.Sprintf("%s/%d", rietveld, issue), "")
		case gerrit != "":
			gerrit = strings.TrimRight(gerrit, "/")
			ss.Changelist = ui.NewLink(
				fmt.Sprintf("Gerrit CL %d", issue),
				fmt.Sprintf("%s/c/%d", gerrit, issue), "")
		}
	}

	if gotRevision != "" {
		ss.Revision = ui.NewLink(gotRevision, "", fmt.Sprintf("got revision %s", gotRevision))
		if repository != "" {
			ss.Revision.URL = repository + "/+/" + gotRevision
		}
	}
	return ss
}

func renderBuild(c context.Context, b *buildbot.Build) *ui.MiloBuild {
	// TODO(hinoka): Do all fields concurrently.
	return &ui.MiloBuild{
		Trigger:       sourcestamp(c, b),
		Summary:       summary(c, b),
		Components:    components(b),
		PropertyGroup: properties(b),
		Blame:         blame(b),
	}
}

// DebugBuild fetches a debugging build for testing.
func DebugBuild(c context.Context, relBuildbotDir string, builder string, buildNum int) (*ui.MiloBuild, error) {
	fname := fmt.Sprintf("%s.%d.json", builder, buildNum)
	// ../buildbot below assumes that
	// - this code is not executed by tests outside of this dir
	// - this dir is a sibling of frontend dir
	path := filepath.Join(relBuildbotDir, "testdata", fname)
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b := &buildbot.Build{}
	if err := json.Unmarshal(raw, b); err != nil {
		return nil, err
	}
	return renderBuild(c, b), nil
}

// Build fetches a buildbot build and translates it into a miloBuild.
func Build(c context.Context, master, builder string, buildNum int) (*ui.MiloBuild, error) {
	if err := buildstore.CanAccessMaster(c, master); err != nil {
		return nil, err
	}
	b, err := buildstore.GetBuild(c, master, builder, buildNum)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.Reason("build %s/%s/%d not found", master, builder, buildNum).
			Tag(common.CodeNotFound).
			Err()
	}
	return renderBuild(c, b), nil
}

// BuildID is buildbots's notion of a Build. See buildsource.ID.
type BuildID struct {
	Master      string
	BuilderName string
	BuildNumber string
}

// GetLog implements buildsource.ID.
func (b *BuildID) GetLog(context.Context, string) (string, bool, error) { panic("not implemented") }

// Get implements buildsource.ID.
func (b *BuildID) Get(c context.Context) (*ui.MiloBuild, error) {
	num, err := strconv.ParseInt(b.BuildNumber, 10, 0)
	if err != nil {
		return nil, errors.Annotate(err, "BuildNumber is not a number").
			Tag(common.CodeParameterError).
			Err()
	}
	if num < 0 {
		return nil, errors.New("BuildNumber must be >= 0", common.CodeParameterError)
	}

	if b.Master == "" {
		return nil, errors.New("Master is required", common.CodeParameterError)
	}
	if b.BuilderName == "" {
		return nil, errors.New("BuilderName is required", common.CodeParameterError)
	}

	return Build(c, b.Master, b.BuilderName, int(num))
}
