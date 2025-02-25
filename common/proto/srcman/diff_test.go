// Copyright 2017 The LUCI Authors.
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

package srcman

import (
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDiff(t *testing.T) {
	t.Parallel()

	ftt.Run(`test Diff`, t, func(t *ftt.Test) {

		t.Run(`git checkouts`, func(t *ftt.Test) {
			a := &Manifest{
				Directories: map[string]*Manifest_Directory{
					"foo": {
						GitCheckout: &Manifest_GitCheckout{
							RepoUrl:  "https://example.com",
							Revision: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
						},
					},
				},
			}
			b := proto.Clone(a).(*Manifest)

			t.Run(`equal`, func(t *ftt.Test) {
				d := a.Diff(b)
				assert.Loosely(t, d.Overall, should.Equal(ManifestDiff_EQUAL))
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {GitCheckout: &ManifestDiff_GitCheckout{
						RepoUrl: "https://example.com",
					}},
				}))
			})

			t.Run(`directory add`, func(t *ftt.Test) {
				b.Directories["bar"] = &Manifest_Directory{
					GitCheckout: &Manifest_GitCheckout{
						RepoUrl:  "https://other.example.com",
						Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
					},
				}
				d := a.Diff(b)
				assert.Loosely(t, d.Overall, should.Equal(ManifestDiff_MODIFIED))
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {GitCheckout: &ManifestDiff_GitCheckout{
						RepoUrl: "https://example.com",
					}},
					"bar": {Overall: ManifestDiff_ADDED},
				}))
			})

			t.Run(`directory mv`, func(t *ftt.Test) {
				b.Directories["bar"] = &Manifest_Directory{
					GitCheckout: &Manifest_GitCheckout{
						RepoUrl:  "https://other.example.com",
						Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
					},
				}
				delete(b.Directories, "foo")
				d := a.Diff(b)
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {Overall: ManifestDiff_REMOVED},
					"bar": {Overall: ManifestDiff_ADDED},
				}))
			})

			t.Run(`directory mod`, func(t *ftt.Test) {
				b.Directories["foo"] = &Manifest_Directory{
					GitCheckout: &Manifest_GitCheckout{
						RepoUrl:  "https://other.example.com",
						Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
					},
				}
				d := a.Diff(b)
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						GitCheckout: &ManifestDiff_GitCheckout{
							Overall: ManifestDiff_MODIFIED,
						},
					},
				}))
			})

			t.Run(`diffable change`, func(t *ftt.Test) {
				b.Directories["foo"].GitCheckout.Revision = "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc"
				d := a.Diff(b)
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						GitCheckout: &ManifestDiff_GitCheckout{
							Overall:  ManifestDiff_MODIFIED,
							Revision: ManifestDiff_DIFF,
							RepoUrl:  "https://example.com",
						},
					},
				}))
			})

			t.Run(`patch diffable change`, func(t *ftt.Test) {
				a.Directories["foo"].GitCheckout.PatchFetchRef = "refs/changes/12/12345612/2"
				a.Directories["foo"].GitCheckout.PatchRevision = "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc"

				b.Directories["foo"].GitCheckout.PatchFetchRef = "refs/changes/12/12345612/5"
				b.Directories["foo"].GitCheckout.PatchRevision = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

				d := a.Diff(b)
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						GitCheckout: &ManifestDiff_GitCheckout{
							Overall:       ManifestDiff_MODIFIED,
							PatchRevision: ManifestDiff_DIFF,
							RepoUrl:       "https://example.com",
						},
					},
				}))
			})

			t.Run(`added patch`, func(t *ftt.Test) {
				b.Directories["foo"].GitCheckout.PatchFetchRef = "refs/changes/12/12345612/5"
				b.Directories["foo"].GitCheckout.PatchRevision = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

				d := a.Diff(b)
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						GitCheckout: &ManifestDiff_GitCheckout{
							Overall:       ManifestDiff_MODIFIED,
							PatchRevision: ManifestDiff_ADDED,
							RepoUrl:       "https://example.com",
						},
					},
				}))
			})

		})

		t.Run(`cipd packages`, func(t *ftt.Test) {
			a := &Manifest{
				Directories: map[string]*Manifest_Directory{
					"foo": {
						CipdServerHost: "some.server.example.com",
						CipdPackage: map[string]*Manifest_CIPDPackage{
							"some/pattern/resolved": {
								PackagePattern: "some/pattern/${platform}",
								Version:        "latest",
								InstanceId:     "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
							},
						},
					},
				},
			}
			b := proto.Clone(a).(*Manifest)

			t.Run(`equal`, func(t *ftt.Test) {
				d := a.Diff(b)
				assert.Loosely(t, d.Overall, should.Equal(ManifestDiff_EQUAL))
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {
						CipdPackage: map[string]ManifestDiff_Stat{
							"some/pattern/resolved": ManifestDiff_EQUAL,
						},
					},
				}))
			})

			t.Run(`url diff`, func(t *ftt.Test) {
				b.Directories["foo"].CipdServerHost = "nope@nope.example.com/nope.nope"
				d := a.Diff(b)
				assert.Loosely(t, d.Overall, should.Equal(ManifestDiff_MODIFIED))
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {
						Overall:        ManifestDiff_MODIFIED,
						CipdServerHost: ManifestDiff_MODIFIED,
					},
				}))
			})

			t.Run(`add pkg`, func(t *ftt.Test) {
				b.Directories["foo"].CipdPackage["other/thing"] = &Manifest_CIPDPackage{
					InstanceId: "f00df00df00df00df00df00df00df00df00df00d",
				}
				d := a.Diff(b)
				assert.Loosely(t, d.Overall, should.Equal(ManifestDiff_MODIFIED))
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						CipdPackage: map[string]ManifestDiff_Stat{
							"some/pattern/resolved": ManifestDiff_EQUAL,
							"other/thing":           ManifestDiff_ADDED,
						},
					},
				}))
			})

			t.Run(`pkg mod`, func(t *ftt.Test) {
				b.Directories["foo"].CipdPackage["some/pattern/resolved"] = &Manifest_CIPDPackage{
					InstanceId: "f00df00df00df00df00df00df00df00df00df00d",
				}
				d := a.Diff(b)
				assert.Loosely(t, d.Overall, should.Equal(ManifestDiff_MODIFIED))
				assert.Loosely(t, d.Directories, should.Match(map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						CipdPackage: map[string]ManifestDiff_Stat{
							"some/pattern/resolved": ManifestDiff_MODIFIED,
						},
					},
				}))
			})

		})
	})
}
