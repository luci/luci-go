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
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/common/model"
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
	_kind       string   `gae:"$kind,buildbotBuild"`
	Master      string   `gae:"$master"`
	Blame       []string `json:"blame" gae:"-"`
	Buildername string   `json:"builderName"`
	// This needs to be reflected.  This can be either a String or a Step.
	Currentstep interface{} `json:"currentStep" gae:"-"`
	// We don't care about this one.
	Eta    interface{} `json:"eta" gae:"-"`
	Logs   []Log       `json:"logs" gae:"-"`
	Number int         `json:"number"`
	// This is a slice of tri-tuples of [property name, value, source].
	// property name is always a string
	// value can be a string or float
	// source is optional, but is always a string if present
	Properties  []*Property  `json:"properties" gae:"-"`
	Reason      string       `json:"reason"`
	Results     Result       `json:"results" gae:"-"`
	Slave       string       `json:"slave"`
	Sourcestamp *SourceStamp `json:"sourceStamp" gae:"-"`
	Steps       []Step       `json:"steps" gae:"-"`
	Text        []string     `json:"text" gae:"-"`
	Times       TimeRange    `json:"times" gae:"-"`
	// This one is injected by Milo.  Does not exist in a normal json query.
	TimeStamp Time `json:"timeStamp" gae:"-"`
	// This one is marked by Milo, denotes whether or not the build is internal.
	Internal bool `json:"internal" gae:"-"`
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

var _ datastore.PropertyLoadSaver = (*Build)(nil)
var _ datastore.MetaGetterSetter = (*Build)(nil)

// getID is a helper function that returns the datastore key for a given
// build.
func (b *Build) getID() string {
	s := []string{b.Master, b.Buildername, strconv.Itoa(b.Number)}
	id, err := json.Marshal(s)
	if err != nil {
		panic(err) // This really shouldn't fail.
	}
	return string(id)
}

// setKeys is the inverse of getID().
func (b *Build) setKeys(id string) error {
	s := []string{}
	err := json.Unmarshal([]byte(id), &s)
	if err != nil {
		return err
	}
	if len(s) != 3 {
		return fmt.Errorf("%s does not have 3 items", id)
	}
	b.Master = s[0]
	b.Buildername = s[1]
	b.Number, err = strconv.Atoi(s[2])
	return err // or nil.
}

// GetMeta is overridden so that a query for "id" calls getID() instead of
// the superclass method.
func (b *Build) GetMeta(key string) (interface{}, bool) {
	if key == "id" {
		if b.Master == "" || b.Buildername == "" {
			panic(fmt.Errorf("No Master or Builder found"))
		}
		return b.getID(), true
	}
	return datastore.GetPLS(b).GetMeta(key)
}

// GetAllMeta is overridden for the same reason GetMeta() is.
func (b *Build) GetAllMeta() datastore.PropertyMap {
	p := datastore.GetPLS(b).GetAllMeta()
	p.SetMeta("id", b.getID())
	return p
}

// SetMeta is the inverse of GetMeta().
func (b *Build) SetMeta(key string, val interface{}) bool {
	if key == "id" {
		err := b.setKeys(val.(string))
		if err != nil {
			panic(err)
		}
	}
	return datastore.GetPLS(b).SetMeta(key, val)
}

// Load translates a propertymap into the struct and loads the data into
// the struct.
func (b *Build) Load(p datastore.PropertyMap) error {
	if _, ok := p["data"]; !ok {
		// This is probably from a keys-only query.  No need to load the rest.
		return datastore.GetPLS(b).Load(p)
	}
	gz, err := p.Slice("data")[0].Project(datastore.PTBytes)
	if err != nil {
		return err
	}
	reader, err := gzip.NewReader(bytes.NewReader(gz.([]byte)))
	if err != nil {
		return err
	}
	bs, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, b)
}

func (b *Build) PropertyValue(name string) interface{} {
	for _, prop := range b.Properties {
		if prop.Name == name {
			return prop.Value
		}
	}
	return ""
}

type ErrTooBig struct {
	error
}

// Save returns a property map of the struct to save in the datastore.  It
// contains two fields, the ID which is the key, and a data field which is a
// serialized and gzipped representation of the entire struct.
func (b *Build) Save(withMeta bool) (datastore.PropertyMap, error) {
	bs, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	gzbs := bytes.Buffer{}
	gsw := gzip.NewWriter(&gzbs)
	_, err = gsw.Write(bs)
	if err != nil {
		return nil, err
	}
	err = gsw.Close()
	if err != nil {
		return nil, err
	}
	blob := gzbs.Bytes()
	// Datastore has a max size of 1MB.  If the blob is over 9.5MB, it probably
	// won't fit after accounting for overhead.
	if len(blob) > 950000 {
		return nil, ErrTooBig{
			fmt.Errorf("Build: Build too big to store (%d bytes)", len(blob))}
	}
	p := datastore.PropertyMap{
		"data": datastore.MkPropertyNI(blob),
	}
	if withMeta {
		p["id"] = datastore.MkPropertyNI(b.getID())
		p["master"] = datastore.MkProperty(b.Master)
		p["builder"] = datastore.MkProperty(b.Buildername)
		p["number"] = datastore.MkProperty(b.Number)
		p["finished"] = datastore.MkProperty(b.Finished)
	}
	return p, nil
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
	Who        string          `json:"who"`
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
