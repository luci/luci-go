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

package swarmingimpl

import (
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	// So that this test works on swarming!
	os.Unsetenv(ServerEnvVar)
	os.Unsetenv(TaskIDEnvVar)
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

func TestOptionalDimension(t *testing.T) {
	Convey(`Make sure that stringmapflag.Value are returned as sorted arrays.`, t, func() {
		type item struct {
			s string
			d *optionalDimension
		}

		data := []item{
			{
				s: "foo",
			},
			{
				s: "foo=123",
			},
			{
				s: "foo=123:abc",
			},
			{
				s: "foo=123=abc",
			},
			{
				s: "foo=123:321",
				d: &optionalDimension{
					kv: &swarming.SwarmingRpcsStringPair{
						Key:   "foo",
						Value: "123",
					},
					expiration: 321,
				},
			},
			{
				s: "foo=123:abc:321",
				d: &optionalDimension{
					kv: &swarming.SwarmingRpcsStringPair{
						Key:   "foo",
						Value: "123:abc",
					},
					expiration: 321,
				},
			},
		}

		for _, item := range data {
			f := optionalDimension{}
			err := f.Set(item.s)
			if item.d == nil {
				So(err, ShouldNotBeNil)
			} else {
				So(err, ShouldBeNil)
				So(f, ShouldResemble, *item.d)
			}
		}
	})
}

func TestListToStringListPairArray(t *testing.T) {
	Convey(`TestListToStringListPairArray`, t, func() {
		input := stringlistflag.Flag{
			"x=a",
			"y=c",
			"x=b",
		}
		expected := []*swarming.SwarmingRpcsStringListPair{
			{Key: "x", Value: []string{"a", "b"}},
			{Key: "y", Value: []string{"c"}},
		}

		So(listToStringListPairArray(input), ShouldResemble, expected)
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
		c.Init(&testAuthFlags{})

		err := c.Parse([]string(nil))
		So(err, ShouldErrLike, "must provide -server")
	})
}

func TestTriggerParse_NoDimension(t *testing.T) {
	Convey(`Make sure that Parse fails with no dimensions.`, t, func() {
		c := triggerRun{}
		c.Init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{"-server", "http://localhost:9050"})
		So(err, ShouldBeNil)

		err = c.Parse([]string(nil))
		So(err, ShouldErrLike, "dimension")
	})
}

func TestTriggerParse_NoIsolated(t *testing.T) {
	Convey(`Make sure that Parse handles a missing isolated flag.`, t, func() {
		c := triggerRun{}
		c.Init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
		})
		So(err, ShouldBeNil)

		err = c.Parse([]string(nil))
		So(err, ShouldErrLike, "please specify command after '--'")
	})
}

func TestTriggerParse_RawNoArgs(t *testing.T) {
	Convey(`Make sure that Parse handles missing raw-cmd arguments.`, t, func() {
		c := triggerRun{}
		c.Init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
			"-isolated", "0123456789012345678901234567890123456789",
		})
		So(err, ShouldBeNil)

		err = c.Parse([]string(nil))
		So(err, ShouldErrLike, "please specify command after '--'")
	})
}

func TestTriggerParse_RawArgs(t *testing.T) {
	Convey(`Make sure that Parse allows both raw-cmd and -isolated`, t, func() {
		c := triggerRun{}
		c.Init(&testAuthFlags{})

		err := c.GetFlags().Parse([]string{
			"-server", "http://localhost:9050",
			"-dimension", "os=Ubuntu",
			"-isolated", "0123456789012345678901234567890123456789",
		})
		So(err, ShouldBeNil)

		err = c.Parse([]string{"arg1", "arg2"})
		So(err, ShouldBeNil)
	})
}

func TestProcessTriggerOptions_WithRawArgs(t *testing.T) {
	Convey(`Make sure that processing trigger options handles raw-args.`, t, func() {
		c := triggerRun{}
		c.Init(&testAuthFlags{})
		c.commonFlags.serverURL = "http://localhost:9050"
		c.isolateServer = "http://localhost:10050"

		result, err := c.processTriggerOptions([]string{"arg1", "arg2"}, nil)
		So(err, ShouldBeNil)
		// Setting properties directly on the task is deprecated.
		So(result.Properties, ShouldBeNil)
		So(result.TaskSlices, ShouldHaveLength, 1)
		properties := result.TaskSlices[0].Properties
		So(properties.Command, ShouldResemble, []string{"arg1", "arg2"})
		So(properties.ExtraArgs, ShouldResemble, ([]string)(nil))
		So(properties.InputsRef, ShouldBeNil)
	})
}

func TestProcessTriggerOptions_CipdPackages(t *testing.T) {
	Convey(`Make sure that processing trigger options handles cipd packages.`, t, func() {
		c := triggerRun{}
		c.Init(&testAuthFlags{})
		c.cipdPackage = map[string]string{
			"path:name": "version",
		}
		result, err := c.processTriggerOptions([]string(nil), nil)
		So(err, ShouldBeNil)
		// Setting properties directly on the task is deprecated.
		So(result.Properties, ShouldBeNil)
		So(result.TaskSlices, ShouldHaveLength, 1)
		properties := result.TaskSlices[0].Properties
		So(properties.CipdInput, ShouldResemble, &swarming.SwarmingRpcsCipdInput{
			Packages: []*swarming.SwarmingRpcsCipdPackage{{
				PackageName: "name",
				Path:        "path",
				Version:     "version",
			}},
		})
	})
}

func TestProcessTriggerOptions_CAS(t *testing.T) {
	t.Parallel()
	Convey(`Make sure that processing trigger options handles cas digest.`, t, func() {
		c := triggerRun{}
		c.digest = "1d1e14a2d0da6348f3f37312ef524a2cea1db4ead9ebc6c335f9948ad634cbfd/10430"
		c.commonFlags.serverURL = "https://cas.appspot.com"
		result, err := c.processTriggerOptions([]string(nil), nil)
		So(err, ShouldBeNil)
		// Setting properties directly on the task is deprecated.
		So(result.Properties, ShouldBeNil)
		So(result.TaskSlices, ShouldHaveLength, 1)
		properties := result.TaskSlices[0].Properties
		So(properties.CasInputRoot, ShouldResemble,
			&swarming.SwarmingRpcsCASReference{
				CasInstance: "projects/cas/instances/default_instance",
				Digest: &swarming.SwarmingRpcsDigest{
					Hash:      "1d1e14a2d0da6348f3f37312ef524a2cea1db4ead9ebc6c335f9948ad634cbfd",
					SizeBytes: 10430,

					ForceSendFields: []string{"SizeBytes"},
				},
			})
	})
}

func TestProcessTriggerOptions_OptionalDimension(t *testing.T) {
	t.Parallel()
	Convey(`Basic`, t, func() {
		c := triggerRun{}
		c.Init(&testAuthFlags{})
		So(c.dimensions.Set("foo=abc"), ShouldBeNil)
		So(c.optionalDimension.Set("bar=def:60"), ShouldBeNil)

		optDimExp := int64(60)
		totalExp := int64(660)
		c.expiration = totalExp

		result, err := c.processTriggerOptions([]string(nil), nil)
		So(err, ShouldBeNil)
		So(result.Properties, ShouldBeNil)
		So(result.TaskSlices, ShouldHaveLength, 2)

		slice := result.TaskSlices[0]
		So(slice.Properties.Dimensions, ShouldResemble,
			[]*swarming.SwarmingRpcsStringPair{
				{
					Key:   "foo",
					Value: "abc",
				},
				{
					Key:   "bar",
					Value: "def",
				},
			})
		So(slice.ExpirationSecs, ShouldResemble, optDimExp)

		slice = result.TaskSlices[1]
		So(slice.Properties.Dimensions, ShouldResemble,
			[]*swarming.SwarmingRpcsStringPair{
				{
					Key:   "foo",
					Value: "abc",
				},
			})
		So(slice.ExpirationSecs, ShouldResemble, totalExp-optDimExp)
	})
}

func TestProcessTriggerOptions_SecretBytesPath(t *testing.T) {
	Convey(`Read secret bytes from the file, and set the base64 encoded string.`, t, func() {
		// prepare secret bytes file.
		dir := t.TempDir()
		secretBytes := []byte("this is secret!")
		secretBytesPath := filepath.Join(dir, "secret_bytes")
		err := ioutil.WriteFile(secretBytesPath, secretBytes, 0600)
		So(err, ShouldBeEmpty)

		c := triggerRun{}
		c.Init(&testAuthFlags{})
		c.secretBytesPath = secretBytesPath
		result, err := c.processTriggerOptions(nil, nil)
		So(err, ShouldBeNil)
		So(result.Properties, ShouldBeNil)
		So(result.TaskSlices, ShouldHaveLength, 1)
		slice := result.TaskSlices[0]
		So(slice.Properties.SecretBytes, ShouldEqual, base64.StdEncoding.EncodeToString(secretBytes))
	})
}
