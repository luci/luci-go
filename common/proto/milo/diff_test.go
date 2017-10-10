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

package milo

import (
	"testing"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDiff(t *testing.T) {
	t.Parallel()

	Convey(`test Diff`, t, func() {

		Convey(`git checkouts`, func() {
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

			Convey(`equal`, func() {
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_EQUAL)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {GitCheckout: &ManifestDiff_GitCheckout{
						RepoUrl: "https://example.com",
					}},
				})
			})

			Convey(`directory add`, func() {
				b.Directories["bar"] = &Manifest_Directory{
					GitCheckout: &Manifest_GitCheckout{
						RepoUrl:  "https://other.example.com",
						Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
					},
				}
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {GitCheckout: &ManifestDiff_GitCheckout{
						RepoUrl: "https://example.com",
					}},
					"bar": {Overall: ManifestDiff_ADDED},
				})
			})

			Convey(`directory mv`, func() {
				b.Directories["bar"] = &Manifest_Directory{
					GitCheckout: &Manifest_GitCheckout{
						RepoUrl:  "https://other.example.com",
						Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
					},
				}
				delete(b.Directories, "foo")
				d := a.Diff(b)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {Overall: ManifestDiff_REMOVED},
					"bar": {Overall: ManifestDiff_ADDED},
				})
			})

			Convey(`directory mod`, func() {
				b.Directories["foo"] = &Manifest_Directory{
					GitCheckout: &Manifest_GitCheckout{
						RepoUrl:  "https://other.example.com",
						Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
					},
				}
				d := a.Diff(b)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						GitCheckout: &ManifestDiff_GitCheckout{
							Overall: ManifestDiff_MODIFIED,
						},
					},
				})
			})

			Convey(`diffable change`, func() {
				b.Directories["foo"].GitCheckout.Revision = "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc"
				d := a.Diff(b)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						GitCheckout: &ManifestDiff_GitCheckout{
							Overall:  ManifestDiff_MODIFIED,
							Revision: ManifestDiff_DIFF,
							RepoUrl:  "https://example.com",
						},
					},
				})
			})

			Convey(`patch diffable change`, func() {
				a.Directories["foo"].GitCheckout.PatchFetchRef = "refs/changes/12/12345612/2"
				a.Directories["foo"].GitCheckout.PatchRevision = "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc"

				b.Directories["foo"].GitCheckout.PatchFetchRef = "refs/changes/12/12345612/5"
				b.Directories["foo"].GitCheckout.PatchRevision = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

				d := a.Diff(b)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						GitCheckout: &ManifestDiff_GitCheckout{
							Overall:       ManifestDiff_MODIFIED,
							PatchRevision: ManifestDiff_DIFF,
							RepoUrl:       "https://example.com",
						},
					},
				})
			})

			Convey(`added patch`, func() {
				b.Directories["foo"].GitCheckout.PatchFetchRef = "refs/changes/12/12345612/5"
				b.Directories["foo"].GitCheckout.PatchRevision = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

				d := a.Diff(b)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						GitCheckout: &ManifestDiff_GitCheckout{
							Overall:       ManifestDiff_MODIFIED,
							PatchRevision: ManifestDiff_ADDED,
							RepoUrl:       "https://example.com",
						},
					},
				})
			})

		})

		Convey(`cipd packages`, func() {
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

			Convey(`equal`, func() {
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_EQUAL)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						CipdPackage: map[string]ManifestDiff_Stat{
							"some/pattern/resolved": ManifestDiff_EQUAL,
						},
					},
				})
			})

			Convey(`url diff`, func() {
				b.Directories["foo"].CipdServerHost = "nope@nope.example.com/nope.nope"
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall:        ManifestDiff_MODIFIED,
						CipdServerHost: ManifestDiff_MODIFIED,
					},
				})
			})

			Convey(`add pkg`, func() {
				b.Directories["foo"].CipdPackage["other/thing"] = &Manifest_CIPDPackage{
					InstanceId: "f00df00df00df00df00df00df00df00df00df00d",
				}
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						CipdPackage: map[string]ManifestDiff_Stat{
							"some/pattern/resolved": ManifestDiff_EQUAL,
							"other/thing":           ManifestDiff_ADDED,
						},
					},
				})
			})

			Convey(`pkg mod`, func() {
				b.Directories["foo"].CipdPackage["some/pattern/resolved"] = &Manifest_CIPDPackage{
					InstanceId: "f00df00df00df00df00df00df00df00df00df00d",
				}
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall: ManifestDiff_MODIFIED,
						CipdPackage: map[string]ManifestDiff_Stat{
							"some/pattern/resolved": ManifestDiff_MODIFIED,
						},
					},
				})
			})

		})

		Convey(`isolated items`, func() {
			a := &Manifest{
				Directories: map[string]*Manifest_Directory{
					"foo": {
						IsolatedServerHost: "some.server.example.com",
						Isolated: []*Manifest_Isolated{
							{
								Namespace: "default-gzip",
								Hash:      "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
							},
						},
					},
				},
			}
			b := proto.Clone(a).(*Manifest)

			Convey(`equal`, func() {
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_EQUAL)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {},
				})
			})

			Convey(`server url`, func() {
				b.Directories["foo"].IsolatedServerHost = "nurbs.example.com"
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall:            ManifestDiff_MODIFIED,
						IsolatedServerHost: ManifestDiff_MODIFIED,
					},
				})
			})

			Convey(`add`, func() {
				a.Directories["foo"].Isolated = nil
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall:  ManifestDiff_MODIFIED,
						Isolated: ManifestDiff_ADDED,
					},
				})
			})

			Convey(`rm`, func() {
				b.Directories["foo"].Isolated = nil
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall:  ManifestDiff_MODIFIED,
						Isolated: ManifestDiff_REMOVED,
					},
				})
			})

			Convey(`mod (addition)`, func() {
				b.Directories["foo"].Isolated = append(b.Directories["foo"].Isolated, &Manifest_Isolated{
					Namespace: "default-gzip",
					Hash:      "f00df00df00df00df00df00df00df00df00df00d",
				})
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall:  ManifestDiff_MODIFIED,
						Isolated: ManifestDiff_MODIFIED,
					},
				})
			})

			Convey(`mod (alter hash)`, func() {
				b.Directories["foo"].Isolated[0].Hash = "f00df00df00df00df00df00df00df00df00df00d"
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall:  ManifestDiff_MODIFIED,
						Isolated: ManifestDiff_MODIFIED,
					},
				})
			})

			Convey(`mod (alter namespace)`, func() {
				b.Directories["foo"].Isolated[0].Namespace = "wutwutwut"
				d := a.Diff(b)
				So(d.Overall, ShouldEqual, ManifestDiff_MODIFIED)
				So(d.Directories, ShouldResemble, map[string]*ManifestDiff_Directory{
					"foo": {
						Overall:  ManifestDiff_MODIFIED,
						Isolated: ManifestDiff_MODIFIED,
					},
				})
			})

		})

	})
}
