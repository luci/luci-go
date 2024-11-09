// Copyright 2015 The LUCI Authors.
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

package bootstrap

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/common/types"
)

func TestBootstrap(t *testing.T) {
	ftt.Run(`A test Environment`, t, func(t *ftt.Test) {
		env := environ.New([]string{
			"IRRELEVANT=VALUE",
		})

		t.Run(`With no Butler values will return ErrNotBootstrapped.`, func(t *ftt.Test) {
			_, err := GetFromEnv(env)
			assert.Loosely(t, err, should.Equal(ErrNotBootstrapped))
		})

		t.Run(`With a Butler stream server path`, func(t *ftt.Test) {
			env.Set(EnvStreamServerPath, "null")

			t.Run(`Yields a Bootstrap with a Client, but nothing else`, func(t *ftt.Test) {
				bs, err := GetFromEnv(env)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, bs.Client, should.NotBeNil)
				bs.Client = nil
				assert.Loosely(t, bs, should.Resemble(&Bootstrap{}))
			})

			t.Run(`And the remaining environment parameters`, func(t *ftt.Test) {
				env.Set(EnvStreamPrefix, "butler/prefix")
				env.Set(EnvStreamProject, "test-project")
				env.Set(EnvCoordinatorHost, "example.appspot.com")
				env.Set(EnvNamespace, "some/namespace")

				t.Run(`Yields a fully-populated Bootstrap.`, func(t *ftt.Test) {
					bs, err := GetFromEnv(env)
					assert.Loosely(t, err, should.BeNil)

					// Check that the client is populated, so we can test the remaining
					// fields without reconstructing it.
					assert.Loosely(t, bs.Client, should.NotBeNil)
					bs.Client = nil

					assert.Loosely(t, bs, should.Resemble(&Bootstrap{
						CoordinatorHost: "example.appspot.com",
						Project:         "test-project",
						Prefix:          "butler/prefix",
						Namespace:       "some/namespace",
					}))
				})
			})

			t.Run(`With an invalid Butler prefix, will fail.`, func(t *ftt.Test) {
				env.Set(EnvStreamPrefix, "_notavaildprefix")
				_, err := GetFromEnv(env)
				assert.Loosely(t, err, should.ErrLike("failed to validate prefix"))
			})

			t.Run(`With an invalid Namespace, will fail.`, func(t *ftt.Test) {
				env.Set(EnvNamespace, "!!! invalid")
				_, err := GetFromEnv(env)
				assert.Loosely(t, err, should.ErrLike("failed to validate namespace"))
			})

			t.Run(`With an invalid Butler project, will fail.`, func(t *ftt.Test) {
				env.Set(EnvStreamProject, "_notavaildproject")
				_, err := GetFromEnv(env)
				assert.Loosely(t, err, should.ErrLike("failed to validate project"))
			})
		})
	})
}

func TestBootstrapURLGeneration(t *testing.T) {
	t.Parallel()

	ftt.Run(`A bootstrap instance`, t, func(t *ftt.Test) {
		bs := &Bootstrap{
			Project:         "test",
			Prefix:          "foo",
			CoordinatorHost: "example.appspot.com",
		}

		t.Run(`Can generate viewer URLs`, func(t *ftt.Test) {
			for _, tc := range []struct {
				paths []types.StreamPath
				url   string
			}{
				{[]types.StreamPath{"foo/bar/+/baz"}, "https://example.appspot.com/logs/test/foo/bar/+/baz"},
				{[]types.StreamPath{
					"foo/bar/+/baz",
					"foo/bar/+/qux",
				}, "https://example.appspot.com/v/?s=test%2Ffoo%2Fbar%2F%2B%2Fbaz&s=test%2Ffoo%2Fbar%2F%2B%2Fqux"},
			} {
				t.Run(fmt.Sprintf(`Will generate [%s] from %q`, tc.url, tc.paths), func(t *ftt.Test) {
					url, err := bs.GetViewerURL(tc.paths...)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, url, should.Equal(tc.url))
				})
			}
		})

		t.Run(`With no project, will not generate URLs.`, func(t *ftt.Test) {
			bs.Project = ""

			_, err := bs.GetViewerURL("bar")
			assert.Loosely(t, err, should.ErrLike("no project is configured"))
		})

		t.Run(`With no coordinator host, will not generate URLs.`, func(t *ftt.Test) {
			bs.CoordinatorHost = ""

			_, err := bs.GetViewerURL("bar")
			assert.Loosely(t, err, should.ErrLike("no coordinator host is configured"))
		})
	})
}
