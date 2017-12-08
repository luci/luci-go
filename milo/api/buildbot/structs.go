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

	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// Step represents a single step in a buildbot build.
type Step struct {
	// We actually don't care about ETA.  This is represented as a string if
	// it's fetched from a build json, but a float if it's dug out of the
	// slave portion of a master json.  We'll just set it to interface and
	// ignore it.
	Eta          interface{}     `json:"eta"`
	Expectations [][]interface{} `json:"expectations"`
	Hidden       bool            `json:"hidden"`
	IsFinished   bool            `json:"isFinished"`
	IsStarted    bool            `json:"isStarted"`
	Logs         []Log           `json:"logs"`
	Name         string          `json:"name"`
	Results      StepResults     `json:"results"`
	Statistics   struct {
	} `json:"statistics"`
	StepNumber int               `json:"step_number"`
	Text       []string          `json:"text"`
	Times      TimeRange         `json:"times"`
	Urls       map[string]string `json:"urls"`

	// Log link aliases.  The key is a log name that is being aliases. It should,
	// generally, exist within the Logs. The value is the set of aliases attached
	// to that key.
	Aliases map[string][]*LinkAlias `json:"aliases"`
}

// SourceStamp is a list of changes (commits) tagged with where the changes
// came from, ie. the project/repository.  Also includes a "main" revision."
type SourceStamp struct {
	Branch     *string  `json:"branch"`
	Changes    []Change `json:"changes"`
	Haspatch   bool     `json:"hasPatch"`
	Project    string   `json:"project"`
	Repository string   `json:"repository"`
	Revision   string   `json:"revision"`
}

type LinkAlias struct {
	URL  string `json:"url"`
	Text string `json:"text"`
}

func (a *LinkAlias) Link() *ui.Link {
	return ui.NewLink(a.Text, a.URL, fmt.Sprintf("alias link for %s", a.Text))
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
	Blame       []string `json:"blame"` // email addresses
	Buildername string   `json:"builderName"`
	// This needs to be reflected.  This can be either a String or a Step.
	Currentstep interface{} `json:"currentStep"`
	// We don't care about this one.
	Eta    interface{} `json:"eta"`
	Logs   []Log       `json:"logs"`
	Number int         `json:"number"`
	// This is a slice of tri-tuples of [property name, value, source].
	// property name is always a string
	// value can be a string or float
	// source is optional, but is always a string if present
	Properties  []*Property  `json:"properties"`
	Reason      string       `json:"reason"`
	Results     Result       `json:"results"`
	Slave       string       `json:"slave"`
	Sourcestamp *SourceStamp `json:"sourceStamp"`
	Steps       []Step       `json:"steps"`
	Text        []string     `json:"text"`
	Times       TimeRange    `json:"times"`
	// This one is injected by Milo.  Does not exist in a normal json query.
	TimeStamp Time `json:"timeStamp"`
	// This one is marked by Milo, denotes whether or not the build is internal.
	Internal bool `json:"internal"`
	// This one is computed by Milo for indexing purposes.  It does so by
	// checking to see if times[1] is null or not.
	Finished bool `json:"finished"`
	// OS is a string representation of the OS the build ran on.  This is
	// derived best-effort from the slave information in the master JSON.
	// This information is injected into the buildbot builds via puppet, and
	// comes as Family + Version.  Family is (windows, Darwin, Debian), while
	// Version is the version of the OS, such as (XP, 7, 10) for windows.
	OSFamily  string `json:"osFamily"`
	OSVersion string `json:"osVersion"`

	// Emulated indicates that this Buildbot build was emulated from non-Buildbot
	// e.g. from LUCI.
	Emulated bool `json:"emulated"`
}

// ID returns "<master>/<builder>/<number>" string.
func (b *Build) ID() string {
	return fmt.Sprintf("%s/%s/%d", b.Master, b.Buildername, b.Number)
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

// PropertyNotFound is returned by (*Build).PropertyValue if a property is no
// found.
var PropertyNotFound = fmt.Errorf("property not found")

// PropertyValue returns the named property value.
// If such property does not exist, returns PropertyNotFound.
func (b *Build) PropertyValue(name string) interface{} {
	for _, prop := range b.Properties {
		if prop.Name == name {
			return prop.Value
		}
	}

	// Not returning (nil, false) here because it would complicate all
	// practical usage of this function, which simply try to cast the
	// returned value.
	return PropertyNotFound
}

type Pending struct {
	Source      SourceStamp `json:"source"`
	Reason      string      `json:"reason"`
	SubmittedAt int         `json:"submittedAt"`
	BuilderName string      `json:"builderName"`
}

// Builder is a builder struct from the master json, _not_ the builder json.
type Builder struct {
	Buildername   string `json:"builderName,omitempty"`
	Basedir       string `json:"basedir"`
	CachedBuilds  []int  `json:"cachedBuilds"`
	PendingBuilds int    `json:"pendingBuilds"`
	// This one is specific to the pubsub interface.  This is limited to 75,
	// so it could differ from PendingBuilds
	PendingBuildStates []*Pending `json:"pendingBuildStates"`
	Category           string     `json:"category"`
	CurrentBuilds      []int      `json:"currentBuilds"`
	Slaves             []string   `json:"slaves"`
	State              string     `json:"state"`
}

// ChangeSource is a changesource (ie polling source) usually tied to a master's scheduler.
type ChangeSource struct {
	Description string `json:"description"`
}

// Change describes a commit in a repository as part of a changesource of blamelist.
type Change struct {
	At       string  `json:"at"`
	Branch   *string `json:"branch"`
	Category string  `json:"category"`
	Comments string  `json:"comments"`
	// This could be a list of strings or list of struct { Name string } .
	Files      []interface{}   `json:"files"`
	Number     int             `json:"number"`
	Project    string          `json:"project"`
	Properties [][]interface{} `json:"properties"`
	Repository string          `json:"repository"`
	Rev        string          `json:"rev"`
	Revision   string          `json:"revision"`
	Revlink    string          `json:"revlink"`
	When       int             `json:"when"`
	Who        string          `json:"who"` // email address
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
	RecentBuilds  map[string][]int `json:"builders"`
	Connected     bool             `json:"connected"`
	Host          string           `json:"host"`
	Name          string           `json:"name"`
	Runningbuilds []*Build         `json:"runningBuilds"`
	Version       string           `json:"version"`
	// This is like runningbuilds, but instead of storing the full build,
	// just reference the build by builder: build num.
	RunningbuildsMap map[string][]int `json:"runningBuildsMap"`
}

type Project struct {
	BuildbotURL string `json:"buildbotURL"`
	Title       string `json:"title"`
	Titleurl    string `json:"titleURL"`
}

// Master This is json definition for https://build.chromium.org/p/<master>/json
// endpoints.
type Master struct {
	AcceptingBuilds struct {
		AcceptingBuilds bool `json:"accepting_builds"`
	} `json:"accepting_builds"`

	Builders map[string]*Builder `json:"builders"`

	Buildstate struct {
		AcceptingBuilds bool      `json:"accepting_builds"`
		Builders        []Builder `json:"builders"`
		Project         Project   `json:"project"`
		Timestamp       Time      `json:"timestamp"`
	} `json:"buildstate"`

	ChangeSources map[string]ChangeSource `json:"change_sources"`

	Changes map[string]Change `json:"changes"`

	Clock struct {
		Current struct {
			Local string `json:"local"`
			Utc   string `json:"utc"`
			UtcTs Time   `json:"utc_ts"`
		} `json:"current"`
		ServerStarted struct {
			Local string `json:"local"`
			Utc   string `json:"utc"`
			UtcTs Time   `json:"utc_ts"`
		} `json:"server_started"`
		ServerUptime Time `json:"server_uptime"`
	} `json:"clock"`

	Project Project `json:"project"`

	Slaves map[string]*Slave `json:"slaves"`

	Varz struct {
		AcceptingBuilds bool `json:"accepting_builds"`
		Builders        map[string]struct {
			ConnectedSlaves int    `json:"connected_slaves"`
			CurrentBuilds   int    `json:"current_builds"`
			PendingBuilds   int    `json:"pending_builds"`
			State           string `json:"state"`
			TotalSlaves     int    `json:"total_slaves"`
		} `json:"builders"`
		ServerUptime Time `json:"server_uptime"`
	} `json:"varz"`

	// This is injected by the pubsub publisher on the buildbot side.
	Name string `json:"name"`
}
