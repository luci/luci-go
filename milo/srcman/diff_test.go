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

	. "github.com/smartystreets/goconvey/convey"

	srcman_pb "go.chromium.org/luci/milo/api/proto/manifest"
)

func TestDiff(t *testing.T) {
	t.Parallel()

	Convey(`test Diff`, t, func() {
		a := &srcman_pb.Manifest{
			Directories: map[string]*srcman_pb.Manifest_Directory{
				"foo": {
					GitCheckout: &srcman_pb.Manifest_GitCheckout{
						RepoUrl:  "https://example.com",
						Revision: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					},
				},
			},
		}
		b := &srcman_pb.Manifest{
			Directories: map[string]*srcman_pb.Manifest_Directory{
				"foo": {
					GitCheckout: &srcman_pb.Manifest_GitCheckout{
						RepoUrl:  "https://example.com",
						Revision: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					},
				},
			},
		}

		Convey(`no change`, func() {
			d := ComputeDiff(a, b)
			So(d.OverallChange, ShouldBeFalse)
			So(d.VersionChange, ShouldBeFalse)
			So(len(d.ChangedDirectories), ShouldEqual, 0)
			So(d.UnchangedDirectories.Len(), ShouldEqual, 1)
			So(proto.Equal(d.A, a), ShouldBeTrue)
			So(proto.Equal(d.B, b), ShouldBeTrue)
		})

		Convey(`version change`, func() {
			a.Version = 1
			b.Version = 2
			d := ComputeDiff(a, b)
			So(d.OverallChange, ShouldBeTrue)
			So(d.VersionChange, ShouldBeTrue)
			So(len(d.ChangedDirectories), ShouldEqual, 0)
			So(d.UnchangedDirectories.Len(), ShouldEqual, 1)
			So(proto.Equal(d.A, a), ShouldBeTrue)
			So(proto.Equal(d.B, b), ShouldBeTrue)
		})

		Convey(`directory add`, func() {
			b.Directories["bar"] = &srcman_pb.Manifest_Directory{
				GitCheckout: &srcman_pb.Manifest_GitCheckout{
					RepoUrl:  "https://other.example.com",
					Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
				},
			}
			d := ComputeDiff(a, b)
			So(d.OverallChange, ShouldBeTrue)
			So(d.VersionChange, ShouldBeFalse)
			So(len(d.ChangedDirectories), ShouldEqual, 1)
			So(d.ChangedDirectories["bar"], ShouldEqual, ADDED)
			So(d.UnchangedDirectories.Len(), ShouldEqual, 1)
			So(proto.Equal(d.A, a), ShouldBeTrue)
			So(proto.Equal(d.B, b), ShouldBeTrue)
		})

		Convey(`directory rm`, func() {
			b.Directories["bar"] = &srcman_pb.Manifest_Directory{
				GitCheckout: &srcman_pb.Manifest_GitCheckout{
					RepoUrl:  "https://other.example.com",
					Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
				},
			}
			delete(b.Directories, "foo")
			d := ComputeDiff(a, b)
			So(d.OverallChange, ShouldBeTrue)
			So(d.VersionChange, ShouldBeFalse)
			So(len(d.ChangedDirectories), ShouldEqual, 2)
			So(d.ChangedDirectories["bar"], ShouldEqual, ADDED)
			So(d.ChangedDirectories["foo"], ShouldEqual, REMOVED)
			So(d.UnchangedDirectories.Len(), ShouldEqual, 0)
			So(proto.Equal(d.A, a), ShouldBeTrue)
			So(proto.Equal(d.B, b), ShouldBeTrue)
		})

		Convey(`directory mod`, func() {
			b.Directories["foo"] = &srcman_pb.Manifest_Directory{
				GitCheckout: &srcman_pb.Manifest_GitCheckout{
					RepoUrl:  "https://other.example.com",
					Revision: "badc0ffeebadc0ffeebadc0ffeebadc0ffeebadc",
				},
			}
			d := ComputeDiff(a, b)
			So(d.OverallChange, ShouldBeTrue)
			So(d.VersionChange, ShouldBeFalse)
			So(len(d.ChangedDirectories), ShouldEqual, 1)
			So(d.ChangedDirectories["foo"], ShouldEqual, MODIFIED)
			So(d.UnchangedDirectories.Len(), ShouldEqual, 0)
			So(proto.Equal(d.A, a), ShouldBeTrue)
			So(proto.Equal(d.B, b), ShouldBeTrue)
		})

	})
}
