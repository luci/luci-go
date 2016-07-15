// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/transport"
	"golang.org/x/net/context"
)

func getURL(c context.Context, URL string) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "Fetching %s\n", URL)
	client := transport.GetClient(c)
	resp, err := client.Get(URL)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Failed to fetch %s, status code %d", URL, resp.StatusCode)
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

// getBuild fetches a build from BuildBot.
func getBuild(c context.Context, master, builder, buildNum string) (*buildbotBuild, error) {
	result := &buildbotBuild{
		Master:      master,
		Buildername: builder,
	}
	if num, err := strconv.Atoi(buildNum); err != nil {
		panic(fmt.Errorf("%s does not look like a number", buildNum))
	} else {
		result.Number = num
	}
	// Check memcache first.
	mc := memcache.Get(c)
	if item, err := mc.Get(result.getID()); err == nil {
		log.Debugf(c, "Found in Memcache!")
		json.Unmarshal(item.Value(), result)
		return result, nil
	}
	// Then check local datastore.
	ds := datastore.Get(c)
	err := ds.Get(result)
	if err == nil {
		log.Debugf(c, "Found build in local cache!")
		if result.Times[1] != nil {
			bs, ierr := json.Marshal(result)
			if ierr == nil {
				item := mc.NewItem(result.getID()).SetValue(bs)
				mc.Set(item)
			}
		}
		return result, nil
	}
	log.Debugf(c, "Could not find log in local cache: %s", err.Error())
	// Now check CBE.
	cbeURL := fmt.Sprintf(
		"https://chrome-build-extract.appspot.com/p/%s/builders/%s/builds/%s?json=1",
		master, builder, buildNum)
	if buf, err := getURL(c, cbeURL); err == nil {
		return result, json.Unmarshal(buf, result)
	}
	// TODO(hinoka): Cache finished builds.
	url := fmt.Sprintf(
		"https://build.chromium.org/p/%s/json/builders/%s/builds/%s", master, builder, buildNum)
	buf, err := getURL(c, url)
	if err != nil {
		return nil, err
	}

	return result, json.Unmarshal(buf, result)
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
	started = time.Unix(int64(*times[0]), int64(*times[0]*1e9)%1e9).UTC()
	if times[1] != nil {
		ended = time.Unix(int64(*times[1]), int64(*times[1]*1e9)%1e9).UTC()
		duration = ended.Sub(started)
	} else {
		duration = time.Since(started)
	}
	return
}

// summary Extract the top level summary from a buildbot build as a
// BuildComponent
func summary(b *buildbotBuild) resp.BuildComponent {
	// Timing info
	started, ended, duration := parseTimes(b.Times)

	// Link to bot and original build.
	bot := &resp.Link{
		Label: b.Slave,
		// TODO(hinoka): Internal builds.
		URL: fmt.Sprintf("https://build.chromium.org/p/%s/buildslaves/%s", b.Master, b.Slave),
	}
	source := &resp.Link{
		Label: fmt.Sprintf("%s/%s/%d", b.Master, b.Buildername, b.Number),
		// TODO(hinoka): Internal builds.
		URL: fmt.Sprintf(
			"https://build.chromium.org/p/%s/builders/%s/builds/%d",
			b.Master, b.Buildername, b.Number),
	}

	sum := resp.BuildComponent{
		Label:      fmt.Sprintf("Builder %s Build #%d", b.Buildername, b.Number),
		Status:     b.toStatus(),
		Started:    started,
		Finished:   ended,
		Bot:        bot,
		Source:     source,
		Duration:   duration,
		Type:       resp.Summary, // This is more or less ignored.
		LevelsDeep: 1,
		Text:       []string{}, // Status messages.  Eg "This build failed on..xyz"
	}

	return sum
}

// components takes a full buildbot build struct and extract step info from all
// of the steps and returns it as a list of milo Build Components.
func components(b *buildbotBuild) (result []*resp.BuildComponent) {
	for _, step := range b.Steps {
		if step.Hidden == true {
			continue
		}
		bc := &resp.BuildComponent{
			Label: step.Name,
			Text:  step.Text,
		}

		// Figure out the status.
		if !step.Isstarted {
			bc.Status = resp.NotRun
		} else if !step.Isfinished {
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
		for _, log := range step.Logs {
			link := &resp.Link{
				Label: log[0],
				URL:   log[1],
			}
			if link.Label == "stdio" {
				bc.MainLink = link
			} else {
				bc.SubLink = append(bc.SubLink, link)
			}
		}

		// Figure out the times
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
			return fmt.Sprintf("%s/%d", rietveld, int(v.(float64))), true
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
		allProps[prop[0].(string)] = Prop{
			Value: prop[1],
			Group: prop[2].(string),
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
		key := prop[0].(string)
		value := prop[1]
		switch key {
		case "rietveld":
			if v, ok := value.(string); ok {
				rietveld = v
			} else {
				log.Warningf(c, "Field rietveld is not a string: %#v", value)
			}
		case "issue":
			if v, ok := value.(float64); ok {
				issue = int64(v)
			} else {
				log.Warningf(c, "Field issue is not a float: %#v", value)
			}

		case "got_revision":
			if v, ok := value.(string); ok {
				ss.Revision = v
			} else {
				log.Warningf(c, "Field got_revision is not a string: %#v", value)
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
			log.Warningf(c, "Found issue but not rietveld property.")
		}
	}
	return ss
}

func getDebugBuild(c context.Context, builder, buildNum string) (*buildbotBuild, error) {
	fname := fmt.Sprintf("%s.%s.json", builder, buildNum)
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
func build(c context.Context, master, builder, buildNum string) (*resp.MiloBuild, error) {
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
		Summary:       summary(b),
		Components:    components(b),
		PropertyGroup: properties(b),
		Blame:         blame(b),
	}, nil
}
