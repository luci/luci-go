// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/api/resp"
)

var errBuildNotFound = errors.New("Build not found")

// getBuild fetches a buildbot build from the datastore and checks ACLs.
// The return code matches the master responses.
func getBuild(c context.Context, master, builder string, buildNum int) (*buildbotBuild, error) {
	result := &buildbotBuild{
		Master:      master,
		Buildername: builder,
		Number:      buildNum,
	}

	err := datastore.Get(c, result)
	err = checkAccess(c, err, result.Internal)
	if err == errMasterNotFound {
		err = errBuildNotFound
	}
	return result, err
}

// result2Status translates a buildbot result integer into a resp.Status.
func result2Status(s *int) (status resp.Status) {
	if s == nil {
		return resp.Running
	}
	switch *s {
	case 0:
		status = resp.Success
	case 1:
		status = resp.Warning
	case 2:
		status = resp.Failure
	case 3:
		status = resp.NotRun // Skipped
	case 4:
		status = resp.InfraFailure // Exception
	case 5:
		status = resp.WaitingDependency // Retry
	default:
		panic(fmt.Errorf("Unknown status %d", s))
	}
	return
}

// parseTimes translates a buildbot time tuple (start/end) into a triplet
// of (Started time, Ending time, duration).
func parseTimes(times []*float64) (started, ended time.Time, duration time.Duration) {
	if len(times) != 2 {
		panic(fmt.Errorf("Expected 2 floats for times, got %v", times))
	}
	if times[0] == nil {
		// Some steps don't have timing info.  In that case, just return nils.
		return
	}
	started = time.Unix(int64(*times[0]), int64(*times[0]*1e9)%1e9).UTC()
	if times[1] != nil {
		ended = time.Unix(int64(*times[1]), int64(*times[1]*1e9)%1e9).UTC()
		duration = ended.Sub(started)
	} else {
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

// summary Extract the top level summary from a buildbot build as a
// BuildComponent
func summary(c context.Context, b *buildbotBuild) resp.BuildComponent {
	// TODO(hinoka): use b.toStatus()
	// Status
	var status resp.Status
	if b.Currentstep != nil {
		status = resp.Running
	} else {
		status = result2Status(b.Results)
	}

	// Timing info
	started, ended, duration := parseTimes(b.Times)

	// Link to bot and original build.
	server := "build.chromium.org/p"
	if b.Internal {
		server = "uberchromegw.corp.google.com/i"
	}
	bot := &resp.Link{
		Label: b.Slave,
		URL:   fmt.Sprintf("https://%s/%s/buildslaves/%s", server, b.Master, b.Slave),
	}
	host := "build.chromium.org/p"
	if b.Internal {
		host = "uberchromegw.corp.google.com/i"
	}
	source := &resp.Link{
		Label: fmt.Sprintf("%s/%s/%d", b.Master, b.Buildername, b.Number),
		// TODO(hinoka): Internal builds.
		URL: fmt.Sprintf(
			"https://%s/%s/builders/%s/builds/%d",
			host, b.Master, b.Buildername, b.Number),
	}

	// The link to the builder page.
	parent := &resp.Link{
		Label: b.Buildername,
		URL:   ".",
	}

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
	for i, step := range b.Steps {
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
			bc.Status = resp.NotRun
		} else if !step.IsFinished {
			bc.Status = resp.Running
		} else {
			if len(step.Results) > 0 {
				status := int(step.Results[0].(float64))
				bc.Status = result2Status(&status)
			} else {
				bc.Status = resp.Success
			}
		}

		// Now, link to the logs.
		//
		// Any given log may have an alias link. There may also be alises that are
		// not attached to logs (unusual, but allowed), so we need to track which
		// aliases are unreferenced and add them as immediate links.
		//
		// Additionally, if the build is LogDog-only ("log_location" is "logdog://")
		// then we want to render *only* the alias for steps with aliases.
		isLogDogOnly := false
		if loc, ok := b.getPropertyValue("log_location").(string); ok && strings.HasPrefix(loc, "logdog://") {
			isLogDogOnly = true
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

			var links resp.LinkSet
			if isLogDogOnly && len(aliases) > 0 && !(isLog && i == 0 && logLink.Label == "stdio") {
				// LogDog only and there are viable aliases. Feature them instead of the
				// log link.
				//
				// We exclude the top-level "stdio" link here because there isn't a
				// LogDog equivalent of the raw step output.
				//
				// Any link named "logdog" (Annotee cosmetic implementation detail) will
				// inherit the name of the build link.
				for _, a := range aliases {
					if a.Label == "logdog" {
						a.Label = logLink.Label
					}
				}
				return aliases
			}

			// Step log link takes primary, with aliases as secondary.
			links = make(resp.LinkSet, 1, 1+len(aliases))
			links[0] = logLink

			for _, a := range aliases {
				a.Alias = true
			}
			return append(links, aliases...)
		}

		for _, l := range step.Logs {
			logLink := resp.Link{
				Label: l[0],
				URL:   l[1],
			}

			links := getLinksWithAliases(&logLink, true)
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
			logLink := resp.Link{
				Label: name,
				URL:   step.Urls[name],
			}

			bc.SubLink = append(bc.SubLink, getLinksWithAliases(&logLink, false))
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
		bc.Started, bc.Finished, bc.Duration = parseTimes(step.Times)

		result = append(result, bc)
	}
	return
}

// parseProp returns a representation of v based off k, and a boolean to
// specify whether or not to hide it altogether.
func parseProp(prop map[string]Prop, k string, v interface{}) (string, bool) {
	switch k {
	case "requestedAt":
		if vf, ok := v.(float64); ok {
			return time.Unix(int64(vf), 0).UTC().Format(time.RFC3339), true
		}
	case "buildbucket":
		var b map[string]interface{}
		json.Unmarshal([]byte(v.(string)), &b)
		if b["build"] == nil {
			return "", false
		}
		build := b["build"].(map[string]interface{})
		id := build["id"]
		if id == nil {
			return "", false
		}
		return fmt.Sprintf("https://cr-buildbucket.appspot.com/b/%s", id.(string)), true
	case "issue":
		if rv, ok := prop["rietveld"]; ok {
			rietveld := rv.Value.(string)
			// Issue could be a float, int, or string.
			switch v := v.(type) {
			case float64:
				return fmt.Sprintf("%s/%d", rietveld, int(v)), true
			default: // Probably int or string
				return fmt.Sprintf("%s/%v", rietveld, v), true
			}
		}
		return fmt.Sprintf("%d", int(v.(float64))), true
	case "rietveld":
		return "", false
	default:
		if vs, ok := v.(string); ok && vs == "" {
			return "", false // Value is empty, don't show it.
		}
		if v == nil {
			return "", false
		}
	}
	return fmt.Sprintf("%v", v), true
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
		vs, ok := parseProp(allProps, key, value)
		if !ok {
			continue
		}
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
	for _, c := range b.Sourcestamp.Changes {
		files := make([]string, len(c.Files))
		for i, f := range c.Files {
			// Buildbot stores files both as a string, or as a dict with a single entry
			// named "name".  It doesn't matter to us what the type is, but we need
			// to reflect on the type anyways.
			if fn, ok := f.(string); ok {
				files[i] = fn
			} else if fn, ok := f.(struct{ Name string }); ok {
				files[i] = fn.Name
			}
		}
		result = append(result, &resp.Commit{
			AuthorEmail: c.Who,
			Repo:        c.Repository,
			Revision:    c.Revision,
			Description: c.Comments,
			Title:       strings.Split(c.Comments, "\n")[0],
			File:        files,
		})
	}
	return
}

// sourcestamp extracts the source stamp from various parts of a buildbot build,
// including the properties.
func sourcestamp(c context.Context, b *buildbotBuild) *resp.SourceStamp {
	ss := &resp.SourceStamp{}
	rietveld := ""
	issue := int64(-1)
	// TODO(hinoka): Gerrit URLs.
	for _, prop := range b.Properties {
		switch prop.Name {
		case "rietveld":
			if v, ok := prop.Value.(string); ok {
				rietveld = v
			} else {
				logging.Warningf(c, "Field rietveld is not a string: %#v", prop.Value)
			}
		case "issue":
			if v, ok := prop.Value.(float64); ok {
				issue = int64(v)
			} else {
				logging.Warningf(c, "Field issue is not a float: %#v", prop.Value)
			}

		case "got_revision":
			if v, ok := prop.Value.(string); ok {
				ss.Revision = v
			} else {
				logging.Warningf(c, "Field got_revision is not a string: %#v", prop.Value)
			}

		}
	}
	if issue != -1 {
		if rietveld != "" {
			rietveld = strings.TrimRight(rietveld, "/")
			ss.Changelist = &resp.Link{
				Label: fmt.Sprintf("Issue %d", issue),
				URL:   fmt.Sprintf("%s/%d", rietveld, issue),
			}
		} else {
			logging.Warningf(c, "Found issue but not rietveld property.")
		}
	}
	return ss
}

func getDebugBuild(c context.Context, builder string, buildNum int) (*buildbotBuild, error) {
	fname := fmt.Sprintf("%s.%d.json", builder, buildNum)
	// ../buildbot below assumes that
	// - this code is not executed by tests outside of this dir
	// - this dir is a sibling of frontend dir
	path := filepath.Join("..", "buildbot", "testdata", fname)
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b := &buildbotBuild{}
	return b, json.Unmarshal(raw, b)
}

// build fetches a buildbot build and translates it into a miloBuild.
func build(c context.Context, master, builder string, buildNum int) (*resp.MiloBuild, error) {
	var b *buildbotBuild
	var err error
	if master == "debug" {
		b, err = getDebugBuild(c, builder, buildNum)
	} else {
		b, err = getBuild(c, master, builder, buildNum)
	}
	if err != nil {
		return nil, err
	}

	// TODO(hinoka): Do all fields concurrently.
	return &resp.MiloBuild{
		SourceStamp:   sourcestamp(c, b),
		Summary:       summary(c, b),
		Components:    components(b),
		PropertyGroup: properties(b),
		Blame:         blame(b),
	}, nil
}
