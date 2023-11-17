// Copyright 2023 The LUCI Authors.
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

package triager

import (
	"testing"

	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFindImmediateHardDeps(t *testing.T) {
	t.Parallel()

	Convey("findImmediateHardDeps", t, func() {
		ct := cvtesting.Test{}
		_, cancel := ct.SetUp(t)
		defer cancel()

		cls := make(map[int64]*clInfo)
		nextCLID := int64(1)
		spm := &simplePMState{
			pb: &prjpb.PState{},
		}
		newCL := func() *testCLInfo {
			defer func() { nextCLID++ }()
			tci := &testCLInfo{
				pcl: &prjpb.PCL{
					Clid: nextCLID,
				},
			}
			cls[nextCLID] = (*clInfo)(tci)
			spm.pb.Pcls = append(spm.pb.Pcls, tci.pcl)
			return tci
		}
		rs := &runStage{
			pm:  pmState{spm},
			cls: cls,
		}
		CLIDs := func(pcls ...*prjpb.PCL) []int64 {
			var ret []int64
			for _, pcl := range pcls {
				ret = append(ret, pcl.GetClid())
			}
			return ret
		}

		Convey("without deps", func() {
			cl1 := newCL()
			cl2 := newCL()
			So(rs.findImmediateHardDeps(cl1.pcl), ShouldEqual, CLIDs())
			So(rs.findImmediateHardDeps(cl2.pcl), ShouldEqual, CLIDs())
		})

		Convey("with HARD deps", func() {
			Convey("one immediate parent", func() {
				cl1 := newCL()
				cl2 := newCL().Deps(cl1)
				cl3 := newCL().Deps(cl1, cl2)

				So(rs.findImmediateHardDeps(cl1.pcl), ShouldEqual, CLIDs())
				So(rs.findImmediateHardDeps(cl2.pcl), ShouldEqual, CLIDs(cl1.pcl))
				So(rs.findImmediateHardDeps(cl3.pcl), ShouldEqual, CLIDs(cl2.pcl))
			})

			Convey("more than one immediate parents", func() {
				// stack 1
				s1cl1 := newCL()
				s1cl2 := newCL().Deps(s1cl1)
				So(rs.findImmediateHardDeps(s1cl1.pcl), ShouldEqual, CLIDs())
				So(rs.findImmediateHardDeps(s1cl2.pcl), ShouldEqual, CLIDs(s1cl1.pcl))
				// stack 2
				s2cl1 := newCL()
				s2cl2 := newCL().Deps(s2cl1)
				So(rs.findImmediateHardDeps(s2cl1.pcl), ShouldEqual, CLIDs())
				So(rs.findImmediateHardDeps(s2cl2.pcl), ShouldEqual, CLIDs(s2cl1.pcl))

				// cl3 belongs to both stack 1 and 2.
				// both s1cl2 and s2cl2 should be its immediate parents.
				cl3 := newCL().Deps(s1cl1, s1cl2, s2cl1, s2cl2)
				So(rs.findImmediateHardDeps(cl3.pcl), ShouldEqual, CLIDs(s1cl2.pcl, s2cl2.pcl))
			})
		})

		Convey("with SOFT deps", func() {
			cl1 := newCL()
			cl2 := newCL().SoftDeps(cl1)
			cl3 := newCL().SoftDeps(cl1, cl2)

			So(rs.findImmediateHardDeps(cl1.pcl), ShouldEqual, CLIDs())
			So(rs.findImmediateHardDeps(cl2.pcl), ShouldEqual, CLIDs())
			So(rs.findImmediateHardDeps(cl3.pcl), ShouldEqual, CLIDs())
		})

		Convey("with a mix of HARD and SOFT deps", func() {
			cl1 := newCL()
			clA := newCL()
			cl2 := newCL().Deps(cl1)
			cl3 := newCL().Deps(cl1, cl2).SoftDeps(clA)
			cl4 := newCL().Deps(cl1, cl2, cl3)

			So(rs.findImmediateHardDeps(cl1.pcl), ShouldEqual, CLIDs())
			So(rs.findImmediateHardDeps(cl2.pcl), ShouldEqual, CLIDs(cl1.pcl))
			So(rs.findImmediateHardDeps(cl3.pcl), ShouldEqual, CLIDs(cl2.pcl))
			So(rs.findImmediateHardDeps(cl4.pcl), ShouldEqual, CLIDs(cl3.pcl))
		})
	})
}
