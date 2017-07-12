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

package main

import (
	"errors"
	"os"
	"testing"

	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/flag/stringmapflag"

	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	// So that this test works on swarming!
	os.Unsetenv("SWARMING_SERVER")
	os.Unsetenv("SWARMING_TASK_ID")
}

// Make sure that stringmapflag.Value are returned as sorted arrays.
func TestMapToArray(t *testing.T) {
	Convey(`Make sure that stringmapflag.Value are returned as sorted arrays.`, t, func() {
		type item struct {
			m stringmapflag.Value
			a []*swarming.SwarmingRpcsStringPair
		}

		data := []item{
			{
				m: stringmapflag.Value{},
				a: []*swarming.SwarmingRpcsStringPair{},
			},
			{
				m: stringmapflag.Value{
					"foo": "bar",
				},
				a: []*swarming.SwarmingRpcsStringPair{
					{Key: "foo", Value: "bar"},
				},
			},
			{
				m: stringmapflag.Value{
					"foo":  "bar",
					"toto": "fifi",
				},
				a: []*swarming.SwarmingRpcsStringPair{
					{Key: "foo", Value: "bar"},
					{Key: "toto", Value: "fifi"},
				},
			},
			{
				m: stringmapflag.Value{
					"toto": "fifi",
					"foo":  "bar",
				},
				a: []*swarming.SwarmingRpcsStringPair{
					{Key: "foo", Value: "bar"},
					{Key: "toto", Value: "fifi"},
				},
			},
		}

		for _, item := range data {
			a := mapToArray(item.m)
			So(len(a), ShouldResemble, len(item.m))
			So(a, ShouldResemble, item.a)
		}
	})
}

func TestNamePartFromDimensions(t *testing.T) {
	Convey(`Make sure that a string name can be constructed from dimensions.`, t, func() {
		type item struct {
			m    stringmapflag.Value
			part string
		}

		data := []item{
			{
				m:    stringmapflag.Value{},
				part: "",
			},
			{
				m: stringmapflag.Value{
					"foo": "bar",
				},
				part: "foo=bar",
			},
			{
				m: stringmapflag.Value{
					"foo":  "bar",
					"toto": "fifi",
				},
				part: "foo=bar_toto=fifi",
			},
			{
				m: stringmapflag.Value{
					"toto": "fifi",
					"foo":  "bar",
				},
				part: "foo=bar_toto=fifi",
			},
		}

		for _, item := range data {
			part := namePartFromDimensions(item.m)
			So(part, ShouldResemble, item.part)
		}
	})
}

func TestParse_NoArgs(t *testing.T) {
	Convey(`Make sure that Parse works with no arguments.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.Parse([]string{})
		So(err, ShouldResemble, errors.New("must provide -server"))
	})
}

func TestParse_NoDimension(t *testing.T) {
	Convey(`Make sure that Parse works with no dimensions.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.Parse([]string{})
		So(err, ShouldResemble, errors.New("please at least specify one dimension"))
	})
}

func TestParse_NoIsolated(t *testing.T) {
	Convey(`Make sure that Parse handles a missing isolated flag.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
		})

		err = c.Parse([]string{})
		So(err, ShouldResemble, errors.New("please use -isolated to specify hash"))
	})
}

func TestParse_BadIsolated(t *testing.T) {
	Convey(`Make sure that Parse handles an invalid isolated flag.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
			"-isolated", "0123456789",
		})

		err = c.Parse([]string{})
		So(err, ShouldResemble, errors.New("invalid hash"))
	})
}

func TestParse_RawNoArgs(t *testing.T) {
	Convey(`Make sure that Parse handles missing raw-cmd arguments.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
			"-isolated", "0123456789012345678901234567890123456789",
			"-raw-cmd",
		})

		err = c.Parse([]string{})
		So(err, ShouldResemble, errors.New("arguments with -raw-cmd should be passed after -- as command delimiter"))
	})
}

func TestParse_RawAndIsolateServer(t *testing.T) {
	Convey(`Make sure that Parse handles raw-cmd and isolate-server arguments.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
			"-isolated", "0123456789012345678901234567890123456789",
			"-raw-cmd",
			"-isolate-server", "http://localhost:10050",
		})

		err = c.Parse([]string{"args1"})
		So(err, ShouldResemble, errors.New("can't use both -raw-cmd and -isolate-server"))
	})
}

func TestProcessTriggerOptions_WithRawArgs(t *testing.T) {
	Convey(`Make sure that processing trigger options handles raw-args.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})
		c.commonFlags.serverURL = "http://localhost:9050"
		c.isolateServer = "http://localhost:10050"
		c.isolated = "1234567890123456789012345678901234567890"
		c.rawCmd = true

		result, err := c.processTriggerOptions([]string{"arg1", "arg2"}, nil)
		So(err, ShouldBeNil)
		So(result.Properties.Command, ShouldResemble, []string{"arg1", "arg2"})
		So(result.Properties.ExtraArgs, ShouldResemble, ([]string)(nil))
		So(result.Properties.InputsRef, ShouldBeNil)
	})
}

func TestProcessTriggerOptions_ExtraArgs(t *testing.T) {
	Convey(`Make sure that processing trigger options handles extra arguments.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})
		c.commonFlags.serverURL = "http://localhost:9050"
		c.isolateServer = "http://localhost:10050"
		c.isolated = "1234567890123456789012345678901234567890"

		result, err := c.processTriggerOptions([]string{"arg1", "arg2"}, nil)
		So(err, ShouldBeNil)
		So(result.Properties.Command, ShouldBeNil)
		So(result.Properties.ExtraArgs, ShouldResemble, []string{"arg1", "arg2"})
		So(result.Properties.InputsRef, ShouldResemble, &swarming.SwarmingRpcsFilesRef{
			Isolated:       "1234567890123456789012345678901234567890",
			Isolatedserver: "http://localhost:10050",
			Namespace:      "default-zip",
		})
	})
}

func TestProcessTriggerOptions_EatDashDash(t *testing.T) {
	Convey(`Make sure that processing trigger options handles extra dash in arguments.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})
		c.commonFlags.serverURL = "http://localhost:9050"
		c.isolateServer = "http://localhost:10050"
		c.isolated = "1234567890123456789012345678901234567890"

		result, err := c.processTriggerOptions([]string{"--", "arg1", "arg2"}, nil)
		So(err, ShouldBeNil)
		So(result.Properties.Command, ShouldBeNil)
		So(result.Properties.ExtraArgs, ShouldResemble, []string{"arg1", "arg2"})
		So(result.Properties.InputsRef, ShouldResemble, &swarming.SwarmingRpcsFilesRef{
			Isolated:       "1234567890123456789012345678901234567890",
			Isolatedserver: "http://localhost:10050",
			Namespace:      "default-zip",
		})
	})
}
