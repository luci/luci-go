// Copyright 2020 The LUCI Authors.
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

package gitiles

import (
	"context"
	"strconv"
	"testing"

	"go.chromium.org/luci/common/proto/git"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Implements GitilesClient", t, func() {
		var _ GitilesClient = &Fake{}
		So(nil, ShouldBeNil)
	})

	Convey("Edge cases", t, func() {
		Convey("Empty", func() {
			fake := Fake{}
			Convey("Projects", func() {
				resp, err := fake.Projects(ctx, &ProjectsRequest{})
				So(err, ShouldBeNil)
				So(resp.GetProjects(), ShouldBeEmpty)
			})
			Convey("Revs", func() {
				in := &RefsRequest{
					Project: "foo",
				}
				// Repository not found
				_, err := fake.Refs(ctx, in)
				So(err, ShouldBeError)
			})
		})

		Convey("Empty project", func() {
			fake := Fake{}
			fake.SetRepository("foo", nil, nil)
			Convey("Projects", func() {
				resp, err := fake.Projects(ctx, &ProjectsRequest{})
				So(err, ShouldBeNil)
				So(resp.GetProjects(), ShouldResemble, []string{"foo"})
			})

			Convey("Revs", func() {
				in := &RefsRequest{
					Project: "foo",
				}
				// Repository not found
				out, err := fake.Refs(ctx, in)
				So(err, ShouldBeNil)
				So(out.GetRevisions(), ShouldBeEmpty)
			})
		})

		Convey("Empty repository", func() {
			fake := Fake{}
			refs := map[string]string{"refs/heads/main": ""}
			fake.SetRepository("foo", refs, nil)
			Convey("Projects", func() {
				resp, err := fake.Projects(ctx, &ProjectsRequest{})
				So(err, ShouldBeNil)
				So(resp.GetProjects(), ShouldResemble, []string{"foo"})
			})

			Convey("Revs", func() {
				in := &RefsRequest{
					Project: "foo",
				}
				// Repository not found
				out, err := fake.Refs(ctx, in)
				So(err, ShouldBeNil)
				So(out.GetRevisions(), ShouldResemble, refs)
			})
		})

		Convey("Reference points to invalid revision", func() {
			fake := Fake{}
			So(func() {
				fake.SetRepository("foo", map[string]string{"foo": "bar"}, nil)
			}, ShouldPanic)
		})

		Convey("Duplicate commit", func() {
			fake := Fake{}
			So(func() {
				commits := []*git.Commit{
					{
						Id: "bar",
					},
					{
						Id: "bar",
					},
				}
				fake.SetRepository("foo", map[string]string{"foo": "bar"}, commits)
			}, ShouldPanic)
		})

		Convey("Broken commit chain", func() {
			fake := Fake{}
			commits := []*git.Commit{
				{
					Id:      "bar",
					Parents: []string{"baz"},
				},
			}
			fake.SetRepository("foo", map[string]string{"refs/heads/main": "bar"}, commits)
			So(func() {
				in := &LogRequest{
					Project:    "foo",
					Committish: "refs/heads/main",
				}
				fake.Log(ctx, in)
			}, ShouldPanic)
		})
	})

	Convey("Linear commits", t, func() {
		// Commit structure: 9 -> 8 -> 7 -> 6 -> 5 -> 4 -> 3 -> 2 -> 1 -> 0
		commits := make([]*git.Commit, 10)
		for i := 0; i < 10; i++ {
			commits[i] = &git.Commit{
				Id: strconv.Itoa(i),
			}
			if i > 0 {
				commits[i].Parents = []string{commits[i-1].GetId()}
			}
		}
		refs := map[string]string{
			"refs/heads/main":   "9",
			"refs/heads/stable": "1",
		}
		fake := Fake{}
		fake.SetRepository("foo", refs, commits)

		Convey("Revs", func() {
			in := &RefsRequest{
				Project: "foo",
			}
			out, err := fake.Refs(ctx, in)
			So(err, ShouldBeNil)
			So(out.GetRevisions(), ShouldResemble, refs)
		})

		Convey("Log pagination main", func() {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/main",
				PageSize:   5,
			}
			out, err := fake.Log(ctx, in)
			So(err, ShouldBeNil)
			So(len(out.GetLog()), ShouldEqual, 5)
			// Last element returned is 5, which is the token
			So(out.GetNextPageToken(), ShouldEqual, "5")

			in.PageToken = out.GetNextPageToken()
			out, err = fake.Log(ctx, in)
			So(err, ShouldBeNil)
			So(len(out.GetLog()), ShouldEqual, 5)
			// Last element returned is 0, which is the token
			So(out.GetNextPageToken(), ShouldEqual, "0")

			// We reached the end
			in.PageToken = out.GetNextPageToken()
			out, err = fake.Log(ctx, in)
			So(err, ShouldBeNil)
			So(len(out.GetLog()), ShouldEqual, 0)
			So(out.GetNextPageToken(), ShouldEqual, "")
		})

		Convey("Log stable branch", func() {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/stable",
				PageSize:   5,
			}
			out, err := fake.Log(ctx, in)
			So(err, ShouldBeNil)
			So(len(out.GetLog()), ShouldEqual, 2)
			So(out.GetNextPageToken(), ShouldEqual, "")
		})

		Convey("Log Non existing branch", func() {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/foo",
				PageSize:   5,
			}
			_, err := fake.Log(ctx, in)
			So(err, ShouldBeError)
		})
	})

	Convey("Merge logs", t, func() {
		// Commit structure: 9 -> 8 -> 7 -> 6 -> 5 -> 4 -> 3 -> 2 -> 1 -> 0
		//                          \_ b2      ->     b1  /
		commits := make([]*git.Commit, 12)
		for i := 0; i < 10; i++ {
			commits[i] = &git.Commit{
				Id: strconv.Itoa(i),
			}
			if i > 0 {
				commits[i].Parents = []string{commits[i-1].GetId()}
			}
		}
		commits[10] = &git.Commit{
			Id:      "b1",
			Parents: []string{"3"},
		}
		commits[11] = &git.Commit{
			Id:      "b2",
			Parents: []string{"b1"},
		}
		// Modify commit with ID 8 to merge commit include b2 as parent.
		commits[8].Parents = []string{"7", "b2"}

		refs := map[string]string{
			"refs/heads/main":    "9",
			"refs/heads/feature": "b2",
			"refs/heads/stable":  "7",
		}
		fake := Fake{}
		fake.SetRepository("foo", refs, commits)

		Convey("Entire log", func() {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/main",
				PageSize:   100,
			}
			out, err := fake.Log(ctx, in)
			So(err, ShouldBeNil)
			So(out.GetNextPageToken(), ShouldBeEmpty)
			So(len(out.GetLog()), ShouldEqual, 12)
		})

		Convey("Partial log, before merge", func() {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/stable",
				PageSize:   100,
			}
			out, err := fake.Log(ctx, in)
			So(err, ShouldBeNil)
			So(out.GetNextPageToken(), ShouldBeEmpty)
			So(len(out.GetLog()), ShouldEqual, 8)
		})

		Convey("Partial log, before merge (feature branch)", func() {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/feature",
				PageSize:   100,
			}
			out, err := fake.Log(ctx, in)
			So(err, ShouldBeNil)
			So(out.GetNextPageToken(), ShouldBeEmpty)
			So(len(out.GetLog()), ShouldEqual, 6)
		})

		Convey("Partial log, specific commit", func() {
			in := &LogRequest{
				Project:    "foo",
				Committish: "8",
				PageSize:   100,
			}
			out, err := fake.Log(ctx, in)
			So(err, ShouldBeNil)
			So(out.GetNextPageToken(), ShouldBeEmpty)
			So(len(out.GetLog()), ShouldEqual, 11)
		})
	})
}
