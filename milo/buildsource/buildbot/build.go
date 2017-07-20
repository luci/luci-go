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

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/milo/common/model"
)

// getBuild fetches a buildbot build from the datastore and checks ACLs.
// The return code matches the master responses.
func getBuild(c context.Context, master, builder string, buildNum int) (*buildbotBuild, error) {
	if err := canAccessMaster(c, master); err != nil {
		return nil, err
	}

	result := &buildbotBuild{
		Master:      master,
		Buildername: builder,
		Number:      buildNum,
	}

	err := datastore.Get(c, result)
	if err == datastore.ErrNoSuchEntity {
		err = errors.New("build not found", common.CodeNotFound)
	}

	return result, err
}

// result2Status translates a buildbot result integer into a model.Status.
func result2Status(s *int) (status model.Status) {
	if s == nil {
		return model.Running
	}
	switch *s {
	case 0:
		status = model.Success
	case 1:
		status = model.Warning
	case 2:
		status = model.Failure
	case 3:
		status = model.NotRun // Skipped
	case 4:
		status = model.Exception
	case 5:
		status = model.WaitingDependency // Retry
	default:
		panic(fmt.Errorf("Unknown status %d", s))
	}
	return
}

// buildbotTimeToTime converts a buildbot time representation (pointer to float
// of seconds since epoch) to a native time.Time object.
func buildbotTimeToTime(t *float64) (result time.Time) {
	if t != nil {
		result = time.Unix(int64(*t), int64(*t*1e9)%1e9).UTC()
	}
	return
}

// parseTimes translates a buildbot time tuple (start, end) into a triplet
// of (Started time, Ending time, duration).
// If times[1] is nil and buildFinished is not, ended will be set to buildFinished
// time.
func parseTimes(buildFinished *float64, times []*float64) (started, ended time.Time, duration time.Duration) {
	if len(times) != 2 {
		panic(fmt.Errorf("Expected 2 floats for times, got %v", times))
	}
	if times[0] == nil {
		// Some steps don't have timing info.  In that case, just return nils.
		return
	}
	started = buildbotTimeToTime(times[0])
	switch {
	case times[1] != nil:
		ended = buildbotTimeToTime(times[1])
		duration = ended.Sub(started)
	case buildFinished != nil:
		ended = buildbotTimeToTime(buildFinished)
		duration = ended.Sub(started)
	default:
		duration = time.Since(started)
	}
	return
}

// getBanner parses the OS information from the build and maybe returns a banner.
func getBanner(c context.Context, b *buildbotBuild) *resp.LogoBanner {
	logging.Infof(c, "OS: %s/%s", b.OSFamily, b.OSVersion)
	osLogo := func() *resp.Logo {
		result := &resp.Logo{}
		switch b.OSFamily {
		case "windows":
			result.LogoBase = resp.Windows
		case "Darwin":
			result.LogoBase = resp.OSX
		case "Debian":
			result.LogoBase = resp.Ubuntu
		default:
			return nil
		}
		result.Subtitle = b.OSVersion
		return result
	}()
	if osLogo != nil {
		return &resp.LogoBanner{
			OS: []resp.Logo{*osLogo},
		}
	}
	logging.Warningf(c, "No OS info found.")
	return nil
}

// summary extracts the top level summary from a buildbot build as a
// BuildComponent
func summary(c context.Context, b *buildbotBuild) resp.BuildComponent {
	// TODO(hinoka): use b.toStatus()
	// Status
	var status model.Status
	if b.Currentstep != nil {
		status = model.Running
	} else {
		status = result2Status(b.Results)
	}

	// Timing info
	started, ended, duration := parseTimes(nil, b.Times)

	// Link to bot and original build.
	host := "build.chromium.org/p"
	if b.Internal {
		host = "uberchromegw.corp.google.com/i"
	}
	bot := resp.NewLink(
		b.Slave,
		fmt.Sprintf("https://%s/%s/buildslaves/%s", host, b.Master, b.Slave),
	)
	source := resp.NewLink(
		fmt.Sprintf("%s/%s/%d", b.Master, b.Buildername, b.Number),
		fmt.Sprintf("https://%s/%s/builders/%s/builds/%d",
			host, b.Master, b.Buildername, b.Number),
	)

	// The link to the builder page.
	parent := resp.NewLink(b.Buildername, ".")

	// Do a best effort lookup for the bot information to fill in OS/Platform info.
	banner := getBanner(c, b)

	sum := resp.BuildComponent{
		ParentLabel: parent,
		Label:       fmt.Sprintf("#%d", b.Number),
		Banner:      banner,
		Status:      status,
		Started:     started,
		Finished:    ended,
		Bot:         bot,
		Source:      source,
		Duration:    duration,
		Type:        resp.Summary, // This is more or less ignored.
		LevelsDeep:  1,
		Text:        []string{}, // Status messages.  Eg "This build failed on..xyz"
	}

	return sum
}

var rLineBreak = regexp.MustCompile("<br */?>")

// components takes a full buildbot build struct and extract step info from all
// of the steps and returns it as a list of milo Build Components.
func components(b *buildbotBuild) (result []*resp.BuildComponent) {
	endingTime := b.Times[1]
	for _, step := range b.Steps {
		if step.Hidden == true {
			continue
		}
		bc := &resp.BuildComponent{
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
		} else if !step.IsFinished {
			bc.Status = model.Running
		} else {
			if len(step.Results) > 0 {
				status := int(step.Results[0].(float64))
				bc.Status = result2Status(&status)
			} else {
				bc.Status = model.Success
			}
		}

		// Raise the interesting-ness if the step is not "Success".
		if bc.Status != model.Success {
			bc.Verbosity = resp.Interesting
		}

		remainingAliases := stringset.New(len(step.Aliases))
		for linkAnchor := range step.Aliases {
			remainingAliases.Add(linkAnchor)
		}

		getLinksWithAliases := func(logLink *resp.Link, isLog bool) resp.LinkSet {
			// Generate alias links.
			var aliases resp.LinkSet
			if remainingAliases.Del(logLink.Label) {
				stepAliases := step.Aliases[logLink.Label]
				aliases = make(resp.LinkSet, len(stepAliases))
				for i, alias := range stepAliases {
					aliases[i] = alias.toLink()
				}
			}

			// Step log link takes primary, with aliases as secondary.
			links := make(resp.LinkSet, 1, 1+len(aliases))
			links[0] = logLink

			for _, a := range aliases {
				a.Alias = true
			}
			return append(links, aliases...)
		}

		for _, l := range step.Logs {
			logLink := resp.NewLink(l[0], l[1])

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
			logLink := resp.NewLink(name, step.Urls[name])

			bc.SubLink = append(bc.SubLink, getLinksWithAliases(logLink, false))
		}

		// Add any unused aliases directly.
		if remainingAliases.Len() > 0 {
			unusedAliases := remainingAliases.ToSlice()
			sort.Strings(unusedAliases)

			for _, label := range unusedAliases {
				var baseLink resp.LinkSet
				for _, alias := range step.Aliases[label] {
					aliasLink := alias.toLink()
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

		// Figure out the times.
		bc.Started, bc.Finished, bc.Duration = parseTimes(endingTime, step.Times)

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
func properties(b *buildbotBuild) (result []*resp.PropertyGroup) {
	groups := map[string]*resp.PropertyGroup{}
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
			groups[groupName] = &resp.PropertyGroup{GroupName: groupName}
		}
		vs := parseProp(value)
		groups[groupName].Property = append(groups[groupName].Property, &resp.Property{
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
func blame(b *buildbotBuild) (result []*resp.Commit) {
	if b.Sourcestamp != nil {
		for _, c := range b.Sourcestamp.Changes {
			files := c.GetFiles()
			result = append(result, &resp.Commit{
				AuthorEmail: c.Who,
				Repo:        c.Repository,
				CommitTime:  time.Unix(int64(c.When), 0).UTC(),
				Revision:    resp.NewLink(c.Revision, c.Revlink),
				Description: c.Comments,
				File:        files,
			})
		}
	}
	return
}

// sourcestamp extracts the source stamp from various parts of a buildbot build,
// including the properties.
func sourcestamp(c context.Context, b *buildbotBuild) *resp.SourceStamp {
	ss := &resp.SourceStamp{}
	rietveld := ""
	gerrit := ""
	got_revision := ""
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
		case "issue":
			// Sometime this is a number (float), sometime it is a string.
			if v, ok := prop.Value.(float64); ok {
				issue = int64(v)
			} else if v, ok := prop.Value.(string); ok {
				if vi, err := strconv.ParseInt(v, 10, 64); err == nil {
					issue = int64(vi)
				} else {
					logging.Warningf(c, "Could not decode field issue: %q - %s", prop.Value, err)
				}
			} else {
				logging.Warningf(c, "Field issue is not a string or float: %#v", prop.Value)
			}

		case "got_revision":
			if v, ok := prop.Value.(string); ok {
				got_revision = v
			} else {
				logging.Warningf(c, "Field got_revision is not a string: %#v", prop.Value)
			}

		case "patch_issue":
			if v, ok := prop.Value.(float64); ok {
				issue = int64(v)
			} else {
				logging.Warningf(c, "Field patch_issue is not a float: %#v", prop.Value)
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
			ss.Changelist = resp.NewLink(
				fmt.Sprintf("Rietveld CL %d", issue),
				fmt.Sprintf("%s/%d", rietveld, issue))
		case gerrit != "":
			gerrit = strings.TrimRight(gerrit, "/")
			ss.Changelist = resp.NewLink(
				fmt.Sprintf("Gerrit CL %d", issue),
				fmt.Sprintf("%s/c/%d", gerrit, issue))
		}
	}

	if got_revision != "" {
		ss.Revision = resp.NewLink(got_revision, "")
		if repository != "" {
			ss.Revision.URL = repository + "/+/" + got_revision
		}
	}
	return ss
}

func renderBuild(c context.Context, b *buildbotBuild) *resp.MiloBuild {
	// Modify the build for rendering.
	updatePostProcessBuild(b)

	// TODO(hinoka): Do all fields concurrently.
	return &resp.MiloBuild{
		SourceStamp:   sourcestamp(c, b),
		Summary:       summary(c, b),
		Components:    components(b),
		PropertyGroup: properties(b),
		Blame:         blame(b),
	}
}

// DebugBuild fetches a debugging build for testing.
func DebugBuild(c context.Context, relBuildbotDir string, builder string, buildNum int) (*resp.MiloBuild, error) {
	fname := fmt.Sprintf("%s.%d.json", builder, buildNum)
	// ../buildbot below assumes that
	// - this code is not executed by tests outside of this dir
	// - this dir is a sibling of frontend dir
	path := filepath.Join(relBuildbotDir, "testdata", fname)
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b := &buildbotBuild{}
	if err := json.Unmarshal(raw, b); err != nil {
		return nil, err
	}
	return renderBuild(c, b), nil
}

// Build fetches a buildbot build and translates it into a miloBuild.
func Build(c context.Context, master, builder string, buildNum int) (*resp.MiloBuild, error) {
	b, err := getBuild(c, master, builder, buildNum)
	if err != nil {
		return nil, err
	}
	return renderBuild(c, b), nil
}

// updatePostProcessBuild transforms a build from its raw JSON format into the
// format that should be presented to users.
//
// Post-processing includes:
//	- If the build is LogDog-only, promotes aliases (LogDog links) to
//	  first-class links in the build.
func updatePostProcessBuild(b *buildbotBuild) {
	// If this is a LogDog-only build, we want to promote the LogDog links.
	if loc, ok := b.getPropertyValue("log_location").(string); ok && strings.HasPrefix(loc, "logdog://") {
		linkMap := map[string]string{}
		for sidx := range b.Steps {
			promoteLogDogLinks(&b.Steps[sidx], sidx == 0, linkMap)
		}

		// Update "Logs". This field is part of BuildBot, and is the amalgamation
		// of all logs in the build's steps. Since each log is out of context of its
		// original step, we can't apply the promotion logic; instead, we will use
		// the link map to map any old URLs that were matched in "promoteLogDogLnks"
		// to their new URLs.
		for _, link := range b.Logs {
			// "link" is in the form: [NAME, URL]
			if len(link) != 2 {
				continue
			}

			if newURL, ok := linkMap[link[1]]; ok {
				link[1] = newURL
			}
		}
	}
}

// promoteLogDogLinks updates the links in a BuildBot step to
// promote LogDog links.
//
// A build's links come in one of three forms:
//	- Log Links, which link directly to BuildBot build logs.
//	- URL Links, which are named links to arbitrary URLs.
//	- Aliases, which attach to the label in one of the other types of links and
//	  augment it with additional named links.
//
// LogDog uses aliases exclusively to attach LogDog logs to other links. When
// the build is LogDog-only, though, the original links are actually junk. What
// we want to do is remove the original junk links and replace them with their
// alias counterparts, so that the "natural" BuildBot links are actually LogDog
// links.
//
// As URLs are re-mapped, the supplied "linkMap" will be updated to map the old
// URLs to the new ones.
func promoteLogDogLinks(s *buildbotStep, isInitialStep bool, linkMap map[string]string) {
	type stepLog struct {
		label string
		url   string
	}

	remainingAliases := stringset.New(len(s.Aliases))
	for linkAnchor := range s.Aliases {
		remainingAliases.Add(linkAnchor)
	}

	maybePromoteAliases := func(sl *stepLog, isLog bool) []*stepLog {
		// As a special case, if this is the first step ("steps" in BuildBot), we
		// will refrain from promoting aliases for "stdio", since "stdio" represents
		// the raw BuildBot logs.
		if isLog && isInitialStep && sl.label == "stdio" {
			// No aliases, don't modify this log.
			return []*stepLog{sl}
		}

		// If there are no aliases, we should obviously not promote them. This will
		// be the case for pre-LogDog steps such as build setup.
		aliases := s.Aliases[sl.label]
		if len(aliases) == 0 {
			return []*stepLog{sl}
		}

		// We have chosen to promote the aliases. Therefore, we will not include
		// them as aliases in the modified step.
		remainingAliases.Del(sl.label)

		result := make([]*stepLog, len(aliases))
		for i, alias := range aliases {
			aliasStepLog := stepLog{alias.Text, alias.URL}

			// Any link named "logdog" (Annotee cosmetic implementation detail) will
			// inherit the name of the original log.
			if isLog {
				if aliasStepLog.label == "logdog" {
					aliasStepLog.label = sl.label
				}
			}

			result[i] = &aliasStepLog
		}

		// If we performed mapping, add the OLD -> NEW URL mapping to linkMap.
		//
		// Since multpiple aliases can apply to a single log, and we have to pick
		// one, here, we'll arbitrarily pick the last one. This is maybe more
		// consistent than the first one because linkMap, itself, will end up
		// holding the last mapping for any given URL.
		if len(result) > 0 {
			linkMap[sl.url] = result[len(result)-1].url
		}

		return result
	}

	// Update step logs.
	newLogs := make([][]string, 0, len(s.Logs))
	for _, l := range s.Logs {
		for _, res := range maybePromoteAliases(&stepLog{l[0], l[1]}, true) {
			newLogs = append(newLogs, []string{res.label, res.url})
		}
	}
	s.Logs = newLogs

	// Update step URLs.
	newURLs := make(map[string]string, len(s.Urls))
	for label, link := range s.Urls {
		urlLinks := maybePromoteAliases(&stepLog{label, link}, false)
		if len(urlLinks) > 0 {
			// Use the last URL link, since our URL map can only tolerate one link.
			// The expected case here is that len(urlLinks) == 1, though, but it's
			// possible that multiple aliases can be included for a single URL, so
			// we need to handle that.
			newValue := urlLinks[len(urlLinks)-1]
			newURLs[newValue.label] = newValue.url
		} else {
			newURLs[label] = link
		}
	}
	s.Urls = newURLs

	// Preserve any aliases that haven't been promoted.
	var newAliases map[string][]*buildbotLinkAlias
	if l := remainingAliases.Len(); l > 0 {
		newAliases = make(map[string][]*buildbotLinkAlias, l)
		remainingAliases.Iter(func(v string) bool {
			newAliases[v] = s.Aliases[v]
			return true
		})
	}
	s.Aliases = newAliases
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
func (b *BuildID) Get(c context.Context) (*resp.MiloBuild, error) {
	num, err := strconv.ParseInt(b.BuildNumber, 10, 0)
	if err != nil {
		return nil, errors.Annotate(err, "BuildNumber is not a number").
			Tag(common.CodeParameterError).
			Err()
	}
	if num <= 0 {
		return nil, errors.New("BuildNumber must be > 0", common.CodeParameterError)
	}

	if b.Master == "" {
		return nil, errors.New("Master name is required", common.CodeParameterError)
	}
	if b.BuilderName == "" {
		return nil, errors.New("BuilderName name is required", common.CodeParameterError)
	}

	return Build(c, b.Master, b.BuilderName, int(num))
}
