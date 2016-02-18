// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

// This file contains all of the structs that define buildbot json endpoints.
// This is primarily used for unmarshalling buildbot master and build json.
// json.UnmarshalJSON can directly unmarshal buildbot jsons into these structs.
// Many of the structs were initially built using https://mholt.github.io/json-to-go/

// buildbotStep represents a single step in a buildbot build.
type buildbotStep struct {
	// We actually don't care about ETA.  This is represented as a string if
	// it's fetched from a build json, but a float if it's dug out of the
	// slave portion of a master json.  We'll just set it to interface and
	// ignore it.
	Eta          interface{}     `json:"eta"`
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
}

// buildbotSourceStamp is a list of changes (commits) tagged with where the changes
// came from, ie. the project/repository.  Also includes a "main" revision."
type buildbotSourceStamp struct {
	Branch     *string          `json:"branch"`
	Changes    []buildbotChange `json:"changes"`
	Haspatch   bool             `json:"hasPatch"`
	Project    string           `json:"project"`
	Repository string           `json:"repository"`
	Revision   string           `json:"revision"`
}

// buildbotBuild is a single build json on buildbot.
type buildbotBuild struct {
	Master      string   `gae:"$master"`
	Blame       []string `json:"blame"`
	Buildername string   `json:"builderName"`
	// This needs to be reflected.  This can be either a String or a buildbotStep.
	Currentstep interface{} `json:"currentStep"`
	// We don't care about this one.
	Eta         interface{}          `json:"eta"`
	Logs        [][]string           `json:"logs"`
	Number      int                  `json:"number"`
	Properties  [][]interface{}      `json:"properties"`
	Reason      string               `json:"reason"`
	Results     *int                 `json:"results"`
	Slave       string               `json:"slave"`
	Sourcestamp *buildbotSourceStamp `json:"sourceStamp"`
	Steps       []buildbotStep       `json:"steps"`
	Text        []string             `json:"text"`
	Times       []*float64           `json:"times"`
}

// buildbotBuilder is a builder struct from the master json, _not_ the builder json.
type buildbotBuilder struct {
	Basedir       string   `json:"basedir"`
	Cachedbuilds  []int    `json:"cachedBuilds"`
	Category      string   `json:"category"`
	Currentbuilds []int    `json:"currentBuilds"`
	Slaves        []string `json:"slaves"`
	State         string   `json:"state"`
}

// buildbotChangeSource is a changesource (ie polling source) usually tied to a master's scheduler.
type buildbotChangeSource struct {
	Description string `json:"description"`
}

// buildbotChange describes a commit in a repository as part of a changesource of blamelist.
type buildbotChange struct {
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
	Who        string          `json:"who"`
}

// buildbotSlave describes a slave on a master from a master json, and also includes the
// full builds of any currently running builds.
type buildbotSlave struct {
	// RecentBuilds is a map of builder name to a list of recent finished build
	// numbers on that builder.
	RecentBuilds  map[string][]int `json:"builders"`
	Connected     bool             `json:"connected"`
	Host          string           `json:"host"`
	Name          string           `json:"name"`
	Runningbuilds []buildbotBuild  `json:"runningBuilds"`
	Version       string           `json:"version"`
}

// buildbotMaster This is json definition for https://build.chromium.org/p/<master>/json
// endpoints.
type buildbotMaster struct {
	AcceptingBuilds struct {
		AcceptingBuilds bool `json:"accepting_builds"`
	} `json:"accepting_builds"`

	Builders map[string]buildbotBuilder `json:"builders"`

	Buildstate struct {
		AcceptingBuilds bool `json:"accepting_builds"`
		Builders        []struct {
			Basedir       string   `json:"basedir"`
			Buildername   string   `json:"builderName"`
			Cachedbuilds  []int    `json:"cachedBuilds"`
			Category      string   `json:"category"`
			Currentbuilds []int    `json:"currentBuilds"`
			Slaves        []string `json:"slaves"`
			State         string   `json:"state"`
		} `json:"builders"`
		Project struct {
			Buildboturl string `json:"buildbotURL"`
			Title       string `json:"title"`
			Titleurl    string `json:"titleURL"`
		} `json:"project"`
		Timestamp float64 `json:"timestamp"`
	} `json:"buildstate"`

	ChangeSources map[string]buildbotChangeSource `json:"change_sources"`

	Changes map[string]buildbotChange `json:"changes"`

	Clock struct {
		Current struct {
			Local string  `json:"local"`
			Utc   string  `json:"utc"`
			UtcTs float64 `json:"utc_ts"`
		} `json:"current"`
		ServerStarted struct {
			Local string  `json:"local"`
			Utc   string  `json:"utc"`
			UtcTs float64 `json:"utc_ts"`
		} `json:"server_started"`
		ServerUptime float64 `json:"server_uptime"`
	} `json:"clock"`

	Project struct {
		Buildboturl string `json:"buildbotURL"`
		Title       string `json:"title"`
		Titleurl    string `json:"titleURL"`
	} `json:"project"`

	Slaves map[string]buildbotSlave `json:"slaves"`

	Varz struct {
		AcceptingBuilds bool `json:"accepting_builds"`
		Builders        map[string]struct {
			ConnectedSlaves int    `json:"connected_slaves"`
			CurrentBuilds   int    `json:"current_builds"`
			PendingBuilds   int    `json:"pending_builds"`
			State           string `json:"state"`
			TotalSlaves     int    `json:"total_slaves"`
		} `json:"builders"`
		ServerUptime float64 `json:"server_uptime"`
	} `json:"varz"`
}
