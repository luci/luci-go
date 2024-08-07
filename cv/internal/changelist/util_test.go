// Copyright 2021 The LUCI Authors.
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

package changelist

import (
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/gerritfake"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOwnerIdentity(t *testing.T) {
	t.Parallel()

	Convey("Snapshot.OwnerIdentity works", t, func() {
		s := &Snapshot{}
		_, err := s.OwnerIdentity()
		So(err, ShouldErrLike, "non-Gerrit CL")

		ci := gerritfake.CI(101, gerritfake.Owner("owner-1"))
		s.Kind = &Snapshot_Gerrit{Gerrit: &Gerrit{
			Host: "x-review.example.com",
			Info: ci,
		}}
		i, err := s.OwnerIdentity()
		So(err, ShouldBeNil)
		So(i, ShouldEqual, identity.Identity("user:owner-1@example.com"))

		Convey("no preferred email set", func() {
			// Yes, this happens if no preferred email is set. See crbug/1175771.
			ci.Owner.Email = ""
			_, err = s.OwnerIdentity()
			So(err, ShouldErrLike, "CL x-review.example.com/101 owner email of account 1 is unknown")
		})
	})
}

func TestQueryCLIDsUpdatedBefore(t *testing.T) {
	t.Parallel()

	Convey("QueryCLIDsUpdatedBefore", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		nextChangeNumber := 1
		createNCLs := func(n int) []*CL {
			cls := make([]*CL, n)
			for i := range cls {
				eid := MustGobID("example.com", int64(nextChangeNumber))
				nextChangeNumber++
				cls[i] = eid.MustCreateIfNotExists(ctx)
			}
			return cls
		}

		var allCLs []*CL
		allCLs = append(allCLs, createNCLs(1000)...)
		ct.Clock.Add(1 * time.Minute)
		allCLs = append(allCLs, createNCLs(1000)...)
		ct.Clock.Add(1 * time.Minute)
		allCLs = append(allCLs, createNCLs(1000)...)

		before := ct.Clock.Now().Add(-30 * time.Second)
		var expected common.CLIDs
		for _, cl := range allCLs {
			if cl.UpdateTime.Before(before) {
				expected = append(expected, cl.ID)
			}
		}
		sort.Sort(expected)

		actual, err := QueryCLIDsUpdatedBefore(ctx, before)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, expected)
	})
}
