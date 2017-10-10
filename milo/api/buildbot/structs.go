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

// Package protocol defines types used in Buildbot build protocol,
// e.g. build, step, log, etc.
// The types can used together with encoding/json package.
package buildbot

import (
	"encoding/json"
	"fmt"

	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/common/model"
)

// Step represents a single step in a buildbot build.
type Step struct {
	// We actually don't care about ETA.  This is represented as a string if
	// it's fetched from a build json, but a float if it's dug out of the
	// slave portion of a master json.  We'll just set it to interface and
	// ignore it.
	Eta          interface{}     `json:"eta,omitempty"`
	Expectations [][]interface{} `json:"expectations,omitempty"`
	Hidden       bool            `json:"hidden,omitempty"`
	IsFinished   bool            `json:"isFinished,omitempty"`
	IsStarted    bool            `json:"isStarted,omitempty"`
	Logs         []Log           `json:"logs,omitempty"`
	Name         string          `json:"name,omitempty"`
	Results      StepResults     `json:"results,omitempty"`
	Statistics   *struct {
	} `json:"statistics,omitempty"`
	StepNumber int               `json:"step_number,omitempty"`
	Text       []string          `json:"text,omitempty"`
	Times      TimeRange         `json:"times"`
	Urls       map[string]string `json:"urls,omitempty"`

	// Log link aliases.  The key is a log name that is being aliases. It should,
	// generally, exist within the Logs. The value is the set of aliases attached
	// to that key.
	Aliases map[string][]*LinkAlias `json:"aliases,omitempty"`
}

// SourceStamp is a list of changes (commits) tagged with where the changes
// came from, ie. the project/repository.  Also includes a "main" revision."
type SourceStamp struct {
	Branch     *string  `json:"branch,omitempty"`
	Changes    []Change `json:"changes,omitempty"`
	Haspatch   bool     `json:"hasPatch,omitempty"`
	Project    string   `json:"project,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Revision   string   `json:"revision,omitempty"`
}

type LinkAlias struct {
	URL  string `json:"url,omitempty"`
	Text string `json:"text,omitempty"`
}

func (a *LinkAlias) Link() *resp.Link {
	return resp.NewLink(a.Text, a.URL)
}

type Property struct {
	Name   string
	Value  interface{}
	Source string
}

func (p *Property) MarshalJSON() ([]byte, error) {
	return json.Marshal([]interface{}{p.Name, p.Value, p.Source})
}

func (p *Property) UnmarshalJSON(d []byte) error {
	// The raw BuildBot representation is a slice of interfaces.
	var raw []interface{}
	if err := json.Unmarshal(d, &raw); err != nil {
		return err
	}

	switch len(raw) {
	case 3:
		if s, ok := raw[2].(string); ok {
			p.Source = s
		}
		fallthrough

	case 2:
		p.Value = raw[1]
		fallthrough

	case 1:
		if s, ok := raw[0].(string); ok {
			p.Name = s
		}
	}
	return nil
}

// Build is a single build json on buildbot.
type Build struct {
	Master      string
	Blame       []string `json:"blame,omitempty"`
	Buildername string   `json:"builderName,omitempty"`
	// This needs to be reflected.  This can be either a String or a Step.
	Currentstep interface{} `json:"currentStep,omitempty"`
	// We don't care about this one.
	Eta    interface{} `json:"eta,omitempty"`
	Logs   []Log       `json:"logs,omitempty"`
	Number int         `json:"number,omitempty"`
	// This is a slice of tri-tuples of [property name, value, source].
	// property name is always a string
	// value can be a string or float
	// source is optional, but is always a string if present
	Properties  []*Property  `json:"properties,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	Results     Result       `json:"results,omitempty"`
	Slave       string       `json:"slave,omitempty"`
	Sourcestamp *SourceStamp `json:"sourceStamp,omitempty"`
	Steps       []Step       `json:"steps,omitempty"`
	Text        []string     `json:"text,omitempty"`
	Times       TimeRange    `json:"times"`
	// This one is injected by Milo.  Does not exist in a normal json query.
	TimeStamp Time `json:"timeStamp,omitempty"`
	// This one is marked by Milo, denotes whether or not the build is internal.
	Internal bool `json:"internal,omitempty"`
	// This one is computed by Milo for indexing purposes.  It does so by
	// checking to see if times[1] is null or not.
	Finished bool `json:"finished,omitempty"`
	// OS is a string representation of the OS the build ran on.  This is
	// derived best-effort from the slave information in the master JSON.
	// This information is injected into the buildbot builds via puppet, and
	// comes as Family + Version.  Family is (windows, Darwin, Debian), while
	// Version is the version of the OS, such as (XP, 7, 10) for windows.
	OSFamily  string `json:"osFamily,omitempty"`
	OSVersion string `json:"osVersion,omitempty"`
}

func (b *Build) Status() model.Status {
	var result model.Status
	if b.Currentstep != nil {
		result = model.Running
	} else {
		result = b.Results.Status()
	}
	return result
}

func (b *Build) PropertyValue(name string) interface{} {
	for _, prop := range b.Properties {
		if prop.Name == name {
			return prop.Value
		}
	}
	return ""
}

type Pending struct {
	Source      SourceStamp `json:"source,omitempty"`
	Reason      string      `json:"reason,omitempty"`
	SubmittedAt int         `json:"submittedAt,omitempty"`
	BuilderName string      `json:"builderName,omitempty"`
}

// Builder is a builder struct from the master json, _not_ the builder json.
type Builder struct {
	Buildername   string `json:"builderName,omitempty"`
	Basedir       string `json:"basedir,omitempty"`
	CachedBuilds  []int  `json:"cachedBuilds,omitempty"`
	PendingBuilds int    `json:"pendingBuilds,omitempty"`
	// This one is specific to the pubsub interface.  This is limited to 75,
	// so it could differ from PendingBuilds
	PendingBuildStates []*Pending `json:"pendingBuildStates,omitempty"`
	Category           string     `json:"category,omitempty"`
	CurrentBuilds      []int      `json:"currentBuilds,omitempty"`
	Slaves             []string   `json:"slaves,omitempty"`
	State              string     `json:"state,omitempty"`
}

// ChangeSource is a changesource (ie polling source) usually tied to a master's scheduler.
type ChangeSource struct {
	Description string `json:"description,omitempty"`
}

// Change describes a commit in a repository as part of a changesource of blamelist.
type Change struct {
	At       string  `json:"at,omitempty"`
	Branch   *string `json:"branch,omitempty"`
	Category string  `json:"category,omitempty"`
	Comments string  `json:"comments,omitempty"`
	// This could be a list of strings or list of struct { Name string } .
	Files      []interface{}   `json:"files,omitempty"`
	Number     int             `json:"number,omitempty"`
	Project    string          `json:"project,omitempty"`
	Properties [][]interface{} `json:"properties,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Rev        string          `json:"rev,omitempty"`
	Revision   string          `json:"revision,omitempty"`
	Revlink    string          `json:"revlink,omitempty"`
	When       int             `json:"when,omitempty"`
	Who        string          `json:"who,omitempty"`
}

func (bc *Change) GetFiles() []string {
	files := make([]string, 0, len(bc.Files))
	for _, f := range bc.Files {
		// Buildbot stores files both as a string, or as a dict with a single entry
		// named "name".  It doesn't matter to us what the type is, but we need
		// to reflect on the type anyways.
		switch fn := f.(type) {
		case string:
			files = append(files, fn)
		case map[string]interface{}:
			if name, ok := fn["name"]; ok {
				files = append(files, fmt.Sprintf("%s", name))
			}
		}
	}
	return files
}

// Slave describes a slave on a master from a master json, and also includes the
// full builds of any currently running builds.
type Slave struct {
	// RecentBuilds is a map of builder name to a list of recent finished build
	// numbers on that builder.
	RecentBuilds  map[string][]int `json:"builders,omitempty"`
	Connected     bool             `json:"connected,omitempty"`
	Host          string           `json:"host,omitempty"`
	Name          string           `json:"name,omitempty"`
	Runningbuilds []*Build         `json:"runningBuilds,omitempty"`
	Version       string           `json:"version,omitempty"`
	// This is like runningbuilds, but instead of storing the full build,
	// just reference the build by builder: build num.
	RunningbuildsMap map[string][]int `json:"runningBuildsMap,omitempty"`
}

type Project struct {
	BuildbotURL string `json:"buildbotURL,omitempty"`
	Title       string `json:"title,omitempty"`
	Titleurl    string `json:"titleURL,omitempty"`
}

// Master This is json definition for https://build.chromium.org/p/<master>/json
// endpoints.
type Master struct {
	AcceptingBuilds struct {
		AcceptingBuilds bool `json:"accepting_builds,omitempty"`
	} `json:"accepting_builds,omitempty"`

	Builders map[string]*Builder `json:"builders,omitempty"`

	Buildstate struct {
		AcceptingBuilds bool      `json:"accepting_builds,omitempty"`
		Builders        []Builder `json:"builders,omitempty"`
		Project         Project   `json:"project,omitempty"`
		Timestamp       Time      `json:"timestamp,omitempty"`
	} `json:"buildstate,omitempty"`

	ChangeSources map[string]ChangeSource `json:"change_sources,omitempty"`

	Changes map[string]Change `json:"changes,omitempty"`

	Clock struct {
		Current struct {
			Local string `json:"local,omitempty"`
			Utc   string `json:"utc,omitempty"`
			UtcTs Time   `json:"utc_ts,omitempty"`
		} `json:"current,omitempty"`
		ServerStarted struct {
			Local string `json:"local,omitempty"`
			Utc   string `json:"utc,omitempty"`
			UtcTs Time   `json:"utc_ts,omitempty"`
		} `json:"server_started,omitempty"`
		ServerUptime Time `json:"server_uptime,omitempty"`
	} `json:"clock,omitempty"`

	Project Project `json:"project,omitempty"`

	Slaves map[string]*Slave `json:"slaves,omitempty"`

	Varz struct {
		AcceptingBuilds bool `json:"accepting_builds,omitempty"`
		Builders        map[string]struct {
			ConnectedSlaves int    `json:"connected_slaves,omitempty"`
			CurrentBuilds   int    `json:"current_builds,omitempty"`
			PendingBuilds   int    `json:"pending_builds,omitempty"`
			State           string `json:"state,omitempty"`
			TotalSlaves     int    `json:"total_slaves,omitempty"`
		} `json:"builders,omitempty"`
		ServerUptime Time `json:"server_uptime,omitempty"`
	} `json:"varz,omitempty"`

	// This is injected by the pubsub publisher on the buildbot side.
	Name string `json:"name,omitempty"`
}
