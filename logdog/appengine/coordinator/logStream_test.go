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

package coordinator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

func shouldHaveLogPaths(expected ...string) comparison.Func[any] {
	const cmpName = "shouldHaveLogPaths"

	return func(actual any) *failure.Summary {
		ret := comparison.NewSummaryBuilder(cmpName, actual)
		names := stringset.New(len(expected))

		switch t := actual.(type) {
		case error:
			return ret.
				AddFindingf("Error", t.Error()).
				Because("Encountered Error").
				Summary

		case []*LogStream:
			for _, ls := range t {
				names.Add(string(ls.Path()))
			}

		default:
			return ret.
				Because("Unsupported type").
				Summary
		}

		exp := stringset.NewFromSlice(expected...)
		diff := names.Difference(exp)
		if len(diff) == 0 {
			return nil
		}
		return ret.
			Actual(names.ToSortedSlice()).
			Expected(exp.ToSortedSlice()).
			AddFindingf("Diff", "%#v", diff.ToSortedSlice()).
			Because("Actual was missing some Expected paths").
			Summary
	}
}

func updateLogStreamID(ls *LogStream) {
	ls.ID = LogStreamID(ls.Path())
}

func TestLogStream(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing log stream`, t, func(t *ftt.Test) {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		ds.GetTestable(c).AutoIndex(true)
		ds.GetTestable(c).Consistent(true)

		now := ds.RoundTime(tc.Now().UTC())

		ls := LogStream{
			ID:       LogStreamID("testing/+/log/stream"),
			Prefix:   "testing",
			Name:     "log/stream",
			Created:  now.UTC(),
			ExpireAt: now.Add(LogStreamExpiry).UTC(),
		}

		desc := &logpb.LogStreamDescriptor{
			Prefix:      "testing",
			Name:        "log/stream",
			StreamType:  logpb.StreamType_TEXT,
			ContentType: string(types.ContentTypeText),
			Timestamp:   timestamppb.New(now),
			Tags: map[string]string{
				"foo":  "bar",
				"baz":  "qux",
				"quux": "",
			},
		}

		t.Run(`Can populate the LogStream with descriptor state.`, func(t *ftt.Test) {
			assert.Loosely(t, ls.LoadDescriptor(desc), should.BeNil)
			assert.Loosely(t, ls.Validate(), should.BeNil)

			t.Run(`Will not validate`, func(t *ftt.Test) {
				t.Run(`Without a valid Prefix`, func(t *ftt.Test) {
					ls.Prefix = "!!!not a valid prefix!!!"
					updateLogStreamID(&ls)

					assert.Loosely(t, ls.Validate(), should.ErrLike("invalid prefix"))
				})
				t.Run(`Without a valid Name`, func(t *ftt.Test) {
					ls.Name = "!!!not a valid name!!!"
					updateLogStreamID(&ls)

					assert.Loosely(t, ls.Validate(), should.ErrLike("invalid name"))
				})
				t.Run(`Without a valid created time`, func(t *ftt.Test) {
					ls.Created = time.Time{}
					assert.Loosely(t, ls.Validate(), should.ErrLike("created time is not set"))
				})
				t.Run(`With an invalid descriptor protobuf`, func(t *ftt.Test) {
					ls.Descriptor = []byte{0x00} // Invalid tag, "0".
					assert.Loosely(t, ls.Validate(), should.ErrLike("could not unmarshal descriptor"))
				})
			})

			t.Run(`Can write the LogStream to the Datastore.`, func(t *ftt.Test) {
				assert.Loosely(t, ds.Put(c, &ls), should.BeNil)

				t.Run(`Can read the LogStream back from the Datastore.`, func(t *ftt.Test) {
					ls2 := LogStream{ID: ls.ID}
					assert.Loosely(t, ds.Get(c, &ls2), should.BeNil)
					assert.Loosely(t, ls2, should.Resemble(ls))
				})
			})
		})

		t.Run(`Will refuse to populate from an invalid descriptor.`, func(t *ftt.Test) {
			desc.StreamType = -1
			assert.Loosely(t, ls.LoadDescriptor(desc), should.ErrLike("invalid descriptor"))
		})

		t.Run(`Writing multiple LogStream entries`, func(t *ftt.Test) {
			times := map[string]*timestamppb.Timestamp{}
			streamPaths := []string{
				"testing/+/foo/bar",
				"testing/+/foo/bar/baz",
				"testing/+/baz/qux",
				"testing/+/cat/dog",
				"testing/+/cat/bird/dog",
				"testing/+/bird/plane",
			}
			for i, path := range streamPaths {
				_, splitName := types.StreamPath(path).Split()
				name := string(splitName)

				lsCopy := ls
				lsCopy.Name = name
				lsCopy.Created = ds.RoundTime(now.Add(time.Duration(i) * time.Second))
				lsCopy.ExpireAt = lsCopy.Created.Add(LogStreamExpiry)
				updateLogStreamID(&lsCopy)

				descCopy := proto.Clone(desc).(*logpb.LogStreamDescriptor)
				descCopy.Name = name

				if err := lsCopy.LoadDescriptor(descCopy); err != nil {
					panic(fmt.Errorf("in %#v: %s", descCopy, err))
				}
				assert.Loosely(t, ds.Put(c, &lsCopy), should.BeNil)

				times[name] = timestamppb.New(lsCopy.Created)
			}

			getAll := func(q *LogStreamQuery) []*LogStream {
				var streams []*LogStream
				err := q.Run(c, func(ls *LogStream, _ ds.CursorCB) error {
					streams = append(streams, ls)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				return streams
			}

			t.Run(`When querying LogStream`, func(t *ftt.Test) {
				t.Run(`LogStream path queries`, func(t *ftt.Test) {
					t.Run(`A query for "foo/bar" should return "foo/bar".`, func(t *ftt.Test) {
						q, err := NewLogStreamQuery("testing/+/foo/bar")
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, getAll(q), shouldHaveLogPaths("testing/+/foo/bar"))
					})

					t.Run(`A query for "foo/bar/*" should return "foo/bar/baz".`, func(t *ftt.Test) {
						q, err := NewLogStreamQuery("testing/+/foo/bar/*")
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, getAll(q), shouldHaveLogPaths("testing/+/foo/bar/baz"))
					})

					t.Run(`A query for "foo/**" should return "foo/bar/baz" and "foo/bar".`, func(t *ftt.Test) {
						q, err := NewLogStreamQuery("testing/+/foo/**")
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, getAll(q), shouldHaveLogPaths(
							"testing/+/foo/bar/baz", "testing/+/foo/bar"))
					})

					t.Run(`A query for "cat/**/dog" should return "cat/dog" and "cat/bird/dog".`, func(t *ftt.Test) {
						q, err := NewLogStreamQuery("testing/+/cat/**/dog")
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, getAll(q), shouldHaveLogPaths(
							"testing/+/cat/bird/dog",
							"testing/+/cat/dog",
						))
					})
				})

				t.Run(`A timestamp inequality query for all records returns them in reverse order.`, func(t *ftt.Test) {
					// Reverse "streamPaths".
					si := make([]string, len(streamPaths))
					for i := 0; i < len(streamPaths); i++ {
						si[i] = streamPaths[len(streamPaths)-i-1]
					}

					q, err := NewLogStreamQuery("testing")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, getAll(q), shouldHaveLogPaths(si...))
				})

				t.Run(`A query for "cat/**/dog" should return "cat/bird/dog" and "cat/dog".`, func(t *ftt.Test) {
					q, err := NewLogStreamQuery("testing/+/cat/**/dog")
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, getAll(q), shouldHaveLogPaths(
						"testing/+/cat/bird/dog", "testing/+/cat/dog"))
				})

			})
		})
	})
}

func TestNewLogStreamGlob(t *testing.T) {
	t.Parallel()

	mkLS := func(path string, now time.Time) *LogStream {
		prefix, name := types.StreamPath(path).Split()
		ret := &LogStream{Created: now, ExpireAt: now.Add(LogStreamExpiry)}
		assert.Loosely(t, ret.LoadDescriptor(&logpb.LogStreamDescriptor{
			Prefix:      string(prefix),
			Name:        string(name),
			ContentType: string(types.ContentTypeText),
			Timestamp:   timestamppb.New(now),
		}), should.BeNil)
		updateLogStreamID(ret)
		return ret
	}

	getAllMatches := func(q *LogStreamQuery, logPaths ...string) []*LogStream {
		ctx := memory.Use(context.Background())
		ds.GetTestable(ctx).AutoIndex(true)
		ds.GetTestable(ctx).Consistent(true)

		logStreams := make([]*LogStream, len(logPaths))
		now := testclock.TestTimeUTC
		for i, path := range logPaths {
			logStreams[i] = mkLS(path, now)
			now = now.Add(time.Second)
		}
		assert.Loosely(t, ds.Put(ctx, logStreams), should.BeNil)

		var streams []*LogStream
		err := q.Run(ctx, func(ls *LogStream, _ ds.CursorCB) error {
			streams = append(streams, ls)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)
		return streams
	}

	ftt.Run(`A testing query`, t, func(t *ftt.Test) {
		t.Run(`Will construct a non-globbing query as Prefix/Name equality.`, func(t *ftt.Test) {
			q, err := NewLogStreamQuery("foo/bar/+/baz/qux")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, getAllMatches(q,
				"foo/bar/+/baz/qux",

				"foo/bar/+/baz/qux/other",
				"foo/bar/+/baz",
				"other/prefix/+/baz/qux",
			), shouldHaveLogPaths(
				"foo/bar/+/baz/qux",
			))
		})

		t.Run(`Will refuse to query an invalid Prefix/Name.`, func(t *ftt.Test) {
			_, err := NewLogStreamQuery("////+/baz/qux")
			assert.Loosely(t, err, should.ErrLike("prefix invalid"))

			_, err = NewLogStreamQuery("foo/bar/+//////")
			assert.Loosely(t, err, should.ErrLike("name invalid"))
		})

		t.Run(`Returns error on empty prefix.`, func(t *ftt.Test) {
			_, err := NewLogStreamQuery("/+/baz/qux")
			assert.Loosely(t, err, should.ErrLike("prefix invalid: empty"))
		})

		t.Run(`Treats empty name like **.`, func(t *ftt.Test) {
			q, err := NewLogStreamQuery("baz/qux")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, getAllMatches(q,
				"baz/qux/+/narp",
				"baz/qux/+/blats/stuff",
				"baz/qux/+/nerds/cool_pants",

				"other/prefix/+/baz/qux",
			), shouldHaveLogPaths(
				"baz/qux/+/nerds/cool_pants",
				"baz/qux/+/blats/stuff",
				"baz/qux/+/narp",
			))
		})

		t.Run(`Properly escapes non-* metachars.`, func(t *ftt.Test) {
			q, err := NewLogStreamQuery("baz/qux/+/hi..../**")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, getAllMatches(q,
				"baz/qux/+/hi....",
				"baz/qux/+/hi..../some_stuff",

				"baz/qux/+/hiblat",
				"baz/qux/+/hiblat/some_stuff",
			), shouldHaveLogPaths(
				"baz/qux/+/hi..../some_stuff",
				"baz/qux/+/hi....",
			))
		})

		t.Run(`Will glob out single Name components.`, func(t *ftt.Test) {
			q, err := NewLogStreamQuery("pfx/+/foo/*/*/bar/*/baz/qux/*")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, getAllMatches(q,
				"pfx/+/foo/a/b/bar/c/baz/qux/d",

				"pfx/+/foo/bar/baz/qux",
				"pfx/+/foo/a/extra/b/bar/c/baz/qux/d",
			), shouldHaveLogPaths(
				"pfx/+/foo/a/b/bar/c/baz/qux/d",
			))
		})

		t.Run(`Will handle end-of-query globbing.`, func(t *ftt.Test) {
			q, err := NewLogStreamQuery("pfx/+/foo/*/bar/**")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, getAllMatches(q,
				"pfx/+/foo/a/bar",
				"pfx/+/foo/a/bar/stuff",
				"pfx/+/foo/a/bar/even/more/stuff",

				"pfx/+/foo/a/extra/bar",
				"pfx/+/nope/a/bar",
			), shouldHaveLogPaths(
				"pfx/+/foo/a/bar/even/more/stuff",
				"pfx/+/foo/a/bar/stuff",
				"pfx/+/foo/a/bar",
			))
		})

		t.Run(`Will handle beginning-of-query globbing.`, func(t *ftt.Test) {
			q, err := NewLogStreamQuery("pfx/+/**/foo/*/bar")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, getAllMatches(q,
				"pfx/+/extra/foo/a/bar",
				"pfx/+/even/more/extra/foo/a/bar",
				"pfx/+/foo/a/bar",

				"pfx/+/foo/a/bar/extra",
				"pfx/+/foo/bar",
			), shouldHaveLogPaths(
				"pfx/+/foo/a/bar",
				"pfx/+/even/more/extra/foo/a/bar",
				"pfx/+/extra/foo/a/bar",
			))
		})

		t.Run(`Can handle middle-of-query globbing.`, func(t *ftt.Test) {
			q, err := NewLogStreamQuery("pfx/+/*/foo/*/**/bar/*/baz/*")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, getAllMatches(q,
				"pfx/+/a/foo/b/stuff/bar/c/baz/d",
				"pfx/+/a/foo/b/lots/of/stuff/bar/c/baz/d",
				"pfx/+/a/foo/b/bar/c/baz/d",

				"pfx/+/foo/a/bar/b/baz/c",
			), shouldHaveLogPaths(
				"pfx/+/a/foo/b/bar/c/baz/d",
				"pfx/+/a/foo/b/lots/of/stuff/bar/c/baz/d",
				"pfx/+/a/foo/b/stuff/bar/c/baz/d",
			))
		})
	})
}
