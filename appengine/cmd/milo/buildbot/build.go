// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/common/transport"
	"golang.org/x/net/context"
)

var (
	buildbotBuildURLTemplate = "https://build.chromium.org/p/%s/json/builders/%s/builds/%s"
)

// Built using mostly https://mholt.github.io/json-to-go/, which is awesome.
type bbBuild struct {
	Blame       []string        `json:"blame"`
	Buildername string          `json:"builderName"`
	Currentstep *string         `json:"currentStep"`
	Eta         *string         `json:"eta"`
	Logs        [][]string      `json:"logs"`
	Number      int             `json:"number"`
	Properties  [][]interface{} `json:"properties"`
	Reason      string          `json:"reason"`
	Results     int             `json:"results"`
	Slave       string          `json:"slave"`
	Sourcestamp struct {
		Branch  *string `json:"branch"`
		Changes []struct {
			At         string          `json:"at"`
			Branch     *string         `json:"branch"`
			Category   string          `json:"category"`
			Comments   string          `json:"comments"`
			Files      []string        `json:"files"`
			Number     int             `json:"number"`
			Project    string          `json:"project"`
			Properties [][]interface{} `json:"properties"`
			Repository string          `json:"repository"`
			Rev        string          `json:"rev"`
			Revision   string          `json:"revision"`
			Revlink    string          `json:"revlink"`
			When       int             `json:"when"`
			Who        string          `json:"who"`
		} `json:"changes"`
		Haspatch   bool   `json:"hasPatch"`
		Project    string `json:"project"`
		Repository string `json:"repository"`
		Revision   string `json:"revision"`
	} `json:"sourceStamp"`
	Steps []struct {
		Eta          *string         `json:"eta"`
		Expectations [][]interface{} `json:"expectations"`
		Hidden       bool            `json:"hidden"`
		Isfinished   bool            `json:"isFinished"`
		Isstarted    bool            `json:"isStarted"`
		Logs         [][]string      `json:"logs"`
		Name         string          `json:"name"`
		Results      []interface{}   `json:"results"`
		Statistics   struct {
		} `json:"statistics"`
		StepNumber int        `json:"step_number"`
		Text       []string   `json:"text"`
		Times      []*float64 `json:"times"`
		Urls       struct {
		} `json:"urls"`
	} `json:"steps"`
	Text  []string   `json:"text"`
	Times []*float64 `json:"times"`
}

// getBuild fetches a build from BuildBot.
func getBuild(c context.Context, master, builder, buildNum string) (*bbBuild, error) {
	result := &bbBuild{}
	// TODO(hinoka): Check CBE first.
	// TODO(hinoka): Cache finished builds.
	url := fmt.Sprintf(buildbotBuildURLTemplate, master, builder, buildNum)
	client := transport.GetClient(c)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Failed to fetch %s, status code %d", url, resp.StatusCode)
	}
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(buf, result)
	return result, err
}

// result2Status translates a buildbot result integer into a resp.Status.
func result2Status(s int) (status resp.Status) {
	switch s {
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
// of (Started time, Ending time, duration), where the times are strings
// in RFC3339.
func parseTimes(times []*float64) (string, string, uint64) {
	if len(times) != 2 {
		panic(fmt.Errorf("Expected 2 floats for times, got %v", times))
	}
	endedStr := ""
	var duration time.Duration
	started := time.Unix(int64(*times[0]), 0)
	startedStr := started.Format(time.RFC3339)
	if times[1] != nil {
		ended := time.Unix(int64(*times[1]), 0)
		endedStr = ended.Format(time.RFC3339)
		duration = ended.Sub(started)
	} else {
		duration = time.Since(started)
	}
	return startedStr, endedStr, uint64(duration.Seconds())
}

// summary Extract the top level summary from a buildbot build as a
// BuildComponent
func summary(b *bbBuild) resp.BuildComponent {
	// Status
	var status resp.Status
	if b.Currentstep != nil {
		status = resp.Running
	} else {
		status = result2Status(b.Results)
	}

	// Timing info
	started, ended, duration := parseTimes(b.Times)

	sum := resp.BuildComponent{
		Label:      fmt.Sprintf("Builder %s Build #%d", b.Buildername, b.Number),
		Status:     status,
		Started:    started,
		Finished:   ended,
		Duration:   duration,
		Type:       resp.Summary, // This is more or less ignored.
		LevelsDeep: 1,
		Text:       []string{}, // Status messages.  Eg "This build failed on..xyz"
	}

	return sum
}

// components takes a full buildbot build struct and extract step info from all
// of the steps and returns it as a list of milo Build Components.
func components(b *bbBuild) (result []*resp.BuildComponent) {
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
				bc.Status = result2Status(int(step.Results[0].(float64)))
			} else {
				bc.Status = resp.Success
			}
		}

		// Now, link to the logs.
		for _, log := range step.Logs {
			fmt.Fprintf(os.Stderr, "Log %s/%s", log[0], log[1])
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
// specify whether or not to hide it alltogether.
func parseProp(prop map[string]Prop, k string, v interface{}) (string, bool) {
	switch k {
	case "requestedAt":
		if vf, ok := v.(float64); ok {
			return time.Unix(int64(vf), 0).Format(time.RFC3339), true
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
func properties(b *bbBuild) (result []*resp.PropertyGroup) {
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
func blame(b *bbBuild) (result []*resp.Commit) {
	for _, c := range b.Sourcestamp.Changes {
		result = append(result, &resp.Commit{
			AuthorEmail: c.Who,
			Repo:        c.Repository,
			Revision:    c.Revision,
			Description: c.Comments,
			Title:       strings.Split(c.Comments, "\n")[0],
			File:        c.Files,
		})
	}
	return
}

// build fetches a buildbot build and translates it into a miloBuild.
func build(c context.Context, master, builder, buildNum string) (*resp.MiloBuild, error) {
	b, err := getBuild(c, master, builder, buildNum)
	if err != nil {
		return nil, err
	}
	return &resp.MiloBuild{
		Summary:       summary(b),
		Components:    components(b),
		PropertyGroup: properties(b),
		Blame:         blame(b),
	}, nil
}
