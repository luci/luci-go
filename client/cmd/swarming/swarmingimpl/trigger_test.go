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
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

// TODO(vadimsh): Add a test for actually triggering stuff (calling NewTask).

func TestMapToArray(t *testing.T) {
	t.Parallel()

	ftt.Run(`Make sure that stringmapflag.Value are returned as sorted arrays.`, t, func(t *ftt.Test) {
		type item struct {
			m stringmapflag.Value
			a []*swarmingpb.StringPair
		}

		data := []item{
			{
				m: stringmapflag.Value{},
				a: []*swarmingpb.StringPair{},
			},
			{
				m: stringmapflag.Value{
					"foo": "bar",
				},
				a: []*swarmingpb.StringPair{
					{Key: "foo", Value: "bar"},
				},
			},
			{
				m: stringmapflag.Value{
					"foo":  "bar",
					"toto": "fifi",
				},
				a: []*swarmingpb.StringPair{
					{Key: "foo", Value: "bar"},
					{Key: "toto", Value: "fifi"},
				},
			},
			{
				m: stringmapflag.Value{
					"toto": "fifi",
					"foo":  "bar",
				},
				a: []*swarmingpb.StringPair{
					{Key: "foo", Value: "bar"},
					{Key: "toto", Value: "fifi"},
				},
			},
		}

		for _, item := range data {
			a := mapToArray(item.m)
			assert.Loosely(t, len(a), should.Resemble(len(item.m)))
			assert.Loosely(t, a, should.Resemble(item.a))
		}
	})
}

func TestOptionalDimension(t *testing.T) {
	t.Parallel()

	ftt.Run(`Make sure that stringmapflag.Value are returned as sorted arrays.`, t, func(t *ftt.Test) {
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
					kv: &swarmingpb.StringPair{
						Key:   "foo",
						Value: "123",
					},
					expiration: 321,
				},
			},
			{
				s: "foo=123:abc:321",
				d: &optionalDimension{
					kv: &swarmingpb.StringPair{
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
				assert.Loosely(t, err, should.NotBeNil)
			} else {
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, f, should.Resemble(*item.d))
			}
		}
	})
}

func TestListToStringListPairArray(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestListToStringListPairArray`, t, func(t *ftt.Test) {
		input := stringlistflag.Flag{
			"x=a",
			"y=c",
			"x=b",
		}
		expected := []*swarmingpb.StringListPair{
			{Key: "x", Value: []string{"a", "b"}},
			{Key: "y", Value: []string{"c"}},
		}

		assert.Loosely(t, listToStringListPairArray(input), should.Resemble(expected))
	})
}

func TestNamePartFromDimensions(t *testing.T) {
	t.Parallel()

	ftt.Run(`Make sure that a string name can be constructed from dimensions.`, t, func(t *ftt.Test) {
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
			assert.Loosely(t, part, should.Resemble(item.part))
		}
	})
}

func TestTriggerParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdTrigger,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		assert.Loosely(t, code, should.Equal(1))
		assert.Loosely(t, stderr, should.ContainSubstring(errLike))
	}

	ftt.Run("Wants dimensions", t, func(t *ftt.Test) {
		expectErr(nil, "please specify at least one dimension")
	})

	ftt.Run("Wants a command", t, func(t *ftt.Test) {
		expectErr([]string{"-d", "k=v"}, "please specify command after '--'")
	})
}

func TestProcessTriggerOptions_WithRawArgs(t *testing.T) {
	t.Parallel()

	ftt.Run(`Make sure that processing trigger options handles raw-args.`, t, func(t *ftt.Test) {
		c := triggerImpl{}

		result, err := c.processTriggerOptions([]string{"arg1", "arg2"}, &url.URL{
			Scheme: "http",
			Host:   "localhost:9050",
		})
		assert.Loosely(t, err, should.BeNil)
		// Setting properties directly on the task is deprecated.
		assert.Loosely(t, result.Properties, should.BeNil)
		assert.Loosely(t, result.TaskSlices, should.HaveLength(1))
		properties := result.TaskSlices[0].Properties
		assert.Loosely(t, properties.Command, should.Resemble([]string{"arg1", "arg2"}))
	})
}

func TestProcessTriggerOptions_CipdPackages(t *testing.T) {
	t.Parallel()

	ftt.Run(`Make sure that processing trigger options handles cipd packages.`, t, func(t *ftt.Test) {
		c := triggerImpl{}
		c.cipdPackage = map[string]string{
			"path:name": "version",
		}
		result, err := c.processTriggerOptions([]string(nil), nil)
		assert.Loosely(t, err, should.BeNil)
		// Setting properties directly on the task is deprecated.
		assert.Loosely(t, result.Properties, should.BeNil)
		assert.Loosely(t, result.TaskSlices, should.HaveLength(1))
		properties := result.TaskSlices[0].Properties
		assert.Loosely(t, properties.CipdInput, should.Resemble(&swarmingpb.CipdInput{
			Packages: []*swarmingpb.CipdPackage{{
				PackageName: "name",
				Path:        "path",
				Version:     "version",
			}},
		}))
	})
}

func TestProcessTriggerOptions_CAS(t *testing.T) {
	t.Parallel()

	ftt.Run(`Make sure that processing trigger options handles cas digest.`, t, func(t *ftt.Test) {
		c := triggerImpl{}
		c.digest = "1d1e14a2d0da6348f3f37312ef524a2cea1db4ead9ebc6c335f9948ad634cbfd/10430"
		result, err := c.processTriggerOptions([]string(nil), &url.URL{
			Scheme: "https",
			Host:   "cas.appspot.com",
		})
		assert.Loosely(t, err, should.BeNil)
		// Setting properties directly on the task is deprecated.
		assert.Loosely(t, result.Properties, should.BeNil)
		assert.Loosely(t, result.TaskSlices, should.HaveLength(1))
		properties := result.TaskSlices[0].Properties
		assert.Loosely(t, properties.CasInputRoot, should.Resemble(
			&swarmingpb.CASReference{
				CasInstance: "projects/cas/instances/default_instance",
				Digest: &swarmingpb.Digest{
					Hash:      "1d1e14a2d0da6348f3f37312ef524a2cea1db4ead9ebc6c335f9948ad634cbfd",
					SizeBytes: 10430,
				},
			}))
	})
}

func TestProcessTriggerOptions_OptionalDimension(t *testing.T) {
	t.Parallel()

	ftt.Run(`Basic`, t, func(t *ftt.Test) {
		c := triggerImpl{}
		assert.Loosely(t, c.dimensions.Set("foo=abc"), should.BeNil)
		assert.Loosely(t, c.optionalDimension.Set("bar=def:60"), should.BeNil)

		const optDimExp = 60
		const totalExp = 660
		c.expiration = totalExp

		result, err := c.processTriggerOptions([]string(nil), nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result.Properties, should.BeNil)
		assert.Loosely(t, result.TaskSlices, should.HaveLength(2))

		slice1 := result.TaskSlices[0]
		expectedDims := []*swarmingpb.StringPair{
			{
				Key:   "foo",
				Value: "abc",
			},
			{
				Key:   "bar",
				Value: "def",
			},
		}
		assert.Loosely(t, slice1.Properties.Dimensions, should.Resemble(expectedDims))
		assert.Loosely(t, slice1.ExpirationSecs, should.Equal(optDimExp))
		slice2 := result.TaskSlices[1]
		assert.Loosely(t, slice2.Properties.Dimensions, should.Resemble(expectedDims[0:1]))
		assert.Loosely(t, slice2.ExpirationSecs, should.Equal(totalExp-optDimExp))
	})
}

func TestProcessTriggerOptions_SecretBytesPath(t *testing.T) {
	t.Parallel()

	ftt.Run(`Read secret bytes from the file, and set the base64 encoded string.`, t, func(t *ftt.Test) {
		// prepare secret bytes file.
		dir := t.TempDir()
		secretBytes := []byte("this is secret!")
		secretBytesPath := filepath.Join(dir, "secret_bytes")
		err := os.WriteFile(secretBytesPath, secretBytes, 0600)
		assert.Loosely(t, err, should.ErrLike(nil))

		c := triggerImpl{}
		c.secretBytesPath = secretBytesPath
		result, err := c.processTriggerOptions(nil, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result.Properties, should.BeNil)
		assert.Loosely(t, result.TaskSlices, should.HaveLength(1))
		slice := result.TaskSlices[0]
		assert.Loosely(t, slice.Properties.SecretBytes, should.Match(secretBytes))
	})
}
