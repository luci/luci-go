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
	"os"
	"testing"

	"go.chromium.org/luci/auth"
	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/flag/stringmapflag"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

func TestTriggerParse_NoArgs(t *testing.T) {
	Convey(`Make sure that Parse works with no arguments.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.Parse([]string(nil))
		So(err, ShouldErrLike, "must provide -server")
	})
}

func TestTriggerParse_NoDimension(t *testing.T) {
	Convey(`Make sure that Parse fails with no dimensions.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})

		err = c.Parse([]string(nil))
		So(err, ShouldErrLike, "dimension")
	})
}

func TestTriggerParse_NoIsolated(t *testing.T) {
	Convey(`Make sure that Parse handles a missing isolated flag.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
		})

		err = c.Parse([]string(nil))
		So(err, ShouldErrLike, "please use -isolated to specify hash or -raw-cmd")
	})
}

func TestTriggerParse_RawNoArgs(t *testing.T) {
	Convey(`Make sure that Parse handles missing raw-cmd arguments.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
			"-isolated", "0123456789012345678901234567890123456789",
			"-raw-cmd",
		})

		err = c.Parse([]string(nil))
		So(err, ShouldErrLike, "arguments with -raw-cmd should be passed after -- as command delimiter")
	})
}

func TestTriggerParse_RawArgs(t *testing.T) {
	Convey(`Make sure that Parse allows both -raw-cmd and -isolated`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
			"-isolated", "0123456789012345678901234567890123456789",
			"-raw-cmd",
		})

		err = c.Parse([]string{"arg1", "arg2"})
		So(err, ShouldBeNil)
	})
}

func TestProcessTriggerOptions_WithRawArgs(t *testing.T) {
	Convey(`Make sure that processing trigger options handles raw-args.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})
		c.commonFlags.serverURL = "http://localhost:9050"
		c.isolateServer = "http://localhost:10050"
		c.rawCmd = true

		result := c.processTriggerOptions([]string{"arg1", "arg2"}, nil)
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

		result := c.processTriggerOptions([]string{"arg1", "arg2"}, nil)
		So(result.Properties.Command, ShouldBeNil)
		So(result.Properties.ExtraArgs, ShouldResemble, []string{"arg1", "arg2"})
		So(result.Properties.InputsRef, ShouldResemble, &swarming.SwarmingRpcsFilesRef{
			Isolated:       "1234567890123456789012345678901234567890",
			Isolatedserver: "http://localhost:10050",
			Namespace:      "default-gzip",
		})
	})
}

func TestProcessTriggerOptions_CipdPackages(t *testing.T) {
	Convey(`Make sure that processing trigger options handles cipd packages.`, t, func() {
		c := triggerRun{}
		c.Init(auth.Options{})
		c.cipdPackage = map[string]string{
			"path:name": "version",
		}
		result := c.processTriggerOptions([]string(nil), nil)
		So(result.Properties.CipdInput, ShouldResemble, &swarming.SwarmingRpcsCipdInput{
			Packages: []*swarming.SwarmingRpcsCipdPackage{{
				PackageName: "name",
				Path:        "path",
				Version:     "version",
			}},
		})
	})
}
