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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func shouldHaveLogPaths(actual any, expected ...any) string {
	var names []string
	switch t := actual.(type) {
	case error:
		return t.Error()

	case []*LogStream:
		for _, ls := range t {
			names = append(names, string(ls.Path()))
		}

	default:
		return fmt.Sprintf("unknown 'actual' type: %T", t)
	}

	exp := make([]string, len(expected))
	for i, v := range expected {
		s, ok := v.(string)
		if !ok {
			panic("non-string stream name specified")
		}
		exp[i] = s
	}
	return ShouldResemble(names, exp)
}

func updateLogStreamID(ls *LogStream) {
	ls.ID = LogStreamID(ls.Path())
}

func TestLogStream(t *testing.T) {
	t.Parallel()

	Convey(`A testing log stream`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		ds.GetTestable(c).AutoIndex(true)
		ds.GetTestable(c).Consistent(true)

		now := ds.RoundTime(tc.Now().UTC())

		ls := LogStream{
			ID:      LogStreamID("testing/+/log/stream"),
			Prefix:  "testing",
			Name:    "log/stream",
			Created: now.UTC(),
		}

		desc := logpb.LogStreamDescriptor{
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

		Convey(`Can populate the LogStream with descriptor state.`, func() {
			So(ls.LoadDescriptor(&desc), ShouldBeNil)
			So(ls.Validate(), ShouldBeNil)

			Convey(`Will not validate`, func() {
				Convey(`Without a valid Prefix`, func() {
					ls.Prefix = "!!!not a valid prefix!!!"
					updateLogStreamID(&ls)

					So(ls.Validate(), ShouldErrLike, "invalid prefix")
				})
				Convey(`Without a valid Name`, func() {
					ls.Name = "!!!not a valid name!!!"
					updateLogStreamID(&ls)

					So(ls.Validate(), ShouldErrLike, "invalid name")
				})
				Convey(`Without a valid created time`, func() {
					ls.Created = time.Time{}
					So(ls.Validate(), ShouldErrLike, "created time is not set")
				})
				Convey(`With an invalid descriptor protobuf`, func() {
					ls.Descriptor = []byte{0x00} // Invalid tag, "0".
					So(ls.Validate(), ShouldErrLike, "could not unmarshal descriptor")
				})
			})

			Convey(`Can write the LogStream to the Datastore.`, func() {
				So(ds.Put(c, &ls), ShouldBeNil)

				Convey(`Can read the LogStream back from the Datastore.`, func() {
					ls2 := LogStream{ID: ls.ID}
					So(ds.Get(c, &ls2), ShouldBeNil)
					So(ls2, ShouldResemble, ls)
				})
			})
		})

		Convey(`Will refuse to populate from an invalid descriptor.`, func() {
			desc.StreamType = -1
			So(ls.LoadDescriptor(&desc), ShouldErrLike, "invalid descriptor")
		})

		Convey(`Writing multiple LogStream entries`, func() {
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
				updateLogStreamID(&lsCopy)

				descCopy := desc
				descCopy.Name = name

				if err := lsCopy.LoadDescriptor(&descCopy); err != nil {
					panic(fmt.Errorf("in %#v: %s", descCopy, err))
				}
				So(ds.Put(c, &lsCopy), ShouldBeNil)

				times[name] = timestamppb.New(lsCopy.Created)
			}

			getAll := func(q *LogStreamQuery) []*LogStream {
				var streams []*LogStream
				err := q.Run(c, func(ls *LogStream, _ ds.CursorCB) error {
					streams = append(streams, ls)
					return nil
				})
				So(err, ShouldBeNil)
				return streams
			}

			Convey(`When querying LogStream`, func() {
				Convey(`LogStream path queries`, func() {
					Convey(`A query for "foo/bar" should return "foo/bar".`, func() {
						q, err := NewLogStreamQuery("testing/+/foo/bar")
						So(err, ShouldBeNil)

						So(getAll(q), shouldHaveLogPaths, "testing/+/foo/bar")
					})

					Convey(`A query for "foo/bar/*" should return "foo/bar/baz".`, func() {
						q, err := NewLogStreamQuery("testing/+/foo/bar/*")
						So(err, ShouldBeNil)

						So(getAll(q), shouldHaveLogPaths, "testing/+/foo/bar/baz")
					})

					Convey(`A query for "foo/**" should return "foo/bar/baz" and "foo/bar".`, func() {
						q, err := NewLogStreamQuery("testing/+/foo/**")
						So(err, ShouldBeNil)

						So(getAll(q), shouldHaveLogPaths,
							"testing/+/foo/bar/baz", "testing/+/foo/bar")
					})

					Convey(`A query for "cat/**/dog" should return "cat/dog" and "cat/bird/dog".`, func() {
						q, err := NewLogStreamQuery("testing/+/cat/**/dog")
						So(err, ShouldBeNil)

						So(getAll(q), shouldHaveLogPaths,
							"testing/+/cat/bird/dog",
							"testing/+/cat/dog",
						)
					})
				})

				Convey(`A timestamp inequality query for all records returns them in reverse order.`, func() {
					// Reverse "streamPaths".
					si := make([]any, len(streamPaths))
					for i := 0; i < len(streamPaths); i++ {
						si[i] = any(streamPaths[len(streamPaths)-i-1])
					}

					q, err := NewLogStreamQuery("testing")
					So(err, ShouldBeNil)
					So(getAll(q), shouldHaveLogPaths, si...)
				})

				Convey(`A query for "cat/**/dog" should return "cat/bird/dog" and "cat/dog".`, func() {
					q, err := NewLogStreamQuery("testing/+/cat/**/dog")
					So(err, ShouldBeNil)

					So(getAll(q), shouldHaveLogPaths,
						"testing/+/cat/bird/dog", "testing/+/cat/dog")
				})

				Convey(`A query for streams older than "baz/qux" returns {"foo/bar/baz", and "foo/bar"}.`, func() {
					q, err := NewLogStreamQuery("testing")
					So(err, ShouldBeNil)
					q.TimeBound(nil, times["baz/qux"])

					So(getAll(q), shouldHaveLogPaths,
						"testing/+/foo/bar/baz", "testing/+/foo/bar")
				})

				Convey(`A query for streams newer than "cat/dog" returns {"bird/plane", "cat/bird/dog"}.`, func() {
					q, err := NewLogStreamQuery("testing")
					So(err, ShouldBeNil)
					q.TimeBound(times["cat/dog"], nil)

					So(getAll(q), shouldHaveLogPaths,
						"testing/+/bird/plane",
						"testing/+/cat/bird/dog",
					)
				})

				Convey(`A query for "cat/**/dog" newer than "cat/dog" returns {"cat/bird/dog"}.`, func() {
					q, err := NewLogStreamQuery("testing/+/cat/**/dog")
					So(err, ShouldBeNil)
					q.TimeBound(times["cat/dog"], nil)

					So(getAll(q), shouldHaveLogPaths, "testing/+/cat/bird/dog")
				})
			})
		})
	})
}

func TestNewLogStreamGlob(t *testing.T) {
	t.Parallel()

	mkLS := func(path string, now time.Time) *LogStream {
		prefix, name := types.StreamPath(path).Split()
		ret := &LogStream{Created: now}
		So(ret.LoadDescriptor(&logpb.LogStreamDescriptor{
			Prefix:      string(prefix),
			Name:        string(name),
			ContentType: string(types.ContentTypeText),
			Timestamp:   timestamppb.New(now),
		}), ShouldBeNil)
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
		So(ds.Put(ctx, logStreams), ShouldBeNil)

		var streams []*LogStream
		err := q.Run(ctx, func(ls *LogStream, _ ds.CursorCB) error {
			streams = append(streams, ls)
			return nil
		})
		So(err, ShouldBeNil)
		return streams
	}

	Convey(`A testing query`, t, func() {
		Convey(`Will construct a non-globbing query as Prefix/Name equality.`, func() {
			q, err := NewLogStreamQuery("foo/bar/+/baz/qux")
			So(err, ShouldBeNil)

			So(getAllMatches(q,
				"foo/bar/+/baz/qux",

				"foo/bar/+/baz/qux/other",
				"foo/bar/+/baz",
				"other/prefix/+/baz/qux",
			), shouldHaveLogPaths,
				"foo/bar/+/baz/qux",
			)
		})

		Convey(`Will refuse to query an invalid Prefix/Name.`, func() {
			_, err := NewLogStreamQuery("////+/baz/qux")
			So(err, ShouldErrLike, "prefix invalid")

			_, err = NewLogStreamQuery("foo/bar/+//////")
			So(err, ShouldErrLike, "name invalid")
		})

		Convey(`Returns error on empty prefix.`, func() {
			_, err := NewLogStreamQuery("/+/baz/qux")
			So(err, ShouldErrLike, "prefix invalid: empty")
		})

		Convey(`Treats empty name like **.`, func() {
			q, err := NewLogStreamQuery("baz/qux")
			So(err, ShouldBeNil)

			So(getAllMatches(q,
				"baz/qux/+/narp",
				"baz/qux/+/blats/stuff",
				"baz/qux/+/nerds/cool_pants",

				"other/prefix/+/baz/qux",
			), shouldHaveLogPaths,
				"baz/qux/+/nerds/cool_pants",
				"baz/qux/+/blats/stuff",
				"baz/qux/+/narp",
			)
		})

		Convey(`Properly escapes non-* metachars.`, func() {
			q, err := NewLogStreamQuery("baz/qux/+/hi..../**")
			So(err, ShouldBeNil)

			So(getAllMatches(q,
				"baz/qux/+/hi....",
				"baz/qux/+/hi..../some_stuff",

				"baz/qux/+/hiblat",
				"baz/qux/+/hiblat/some_stuff",
			), shouldHaveLogPaths,
				"baz/qux/+/hi..../some_stuff",
				"baz/qux/+/hi....",
			)
		})

		Convey(`Will glob out single Name components.`, func() {
			q, err := NewLogStreamQuery("pfx/+/foo/*/*/bar/*/baz/qux/*")
			So(err, ShouldBeNil)

			So(getAllMatches(q,
				"pfx/+/foo/a/b/bar/c/baz/qux/d",

				"pfx/+/foo/bar/baz/qux",
				"pfx/+/foo/a/extra/b/bar/c/baz/qux/d",
			), shouldHaveLogPaths,
				"pfx/+/foo/a/b/bar/c/baz/qux/d",
			)
		})

		Convey(`Will handle end-of-query globbing.`, func() {
			q, err := NewLogStreamQuery("pfx/+/foo/*/bar/**")
			So(err, ShouldBeNil)

			So(getAllMatches(q,
				"pfx/+/foo/a/bar",
				"pfx/+/foo/a/bar/stuff",
				"pfx/+/foo/a/bar/even/more/stuff",

				"pfx/+/foo/a/extra/bar",
				"pfx/+/nope/a/bar",
			), shouldHaveLogPaths,
				"pfx/+/foo/a/bar/even/more/stuff",
				"pfx/+/foo/a/bar/stuff",
				"pfx/+/foo/a/bar",
			)
		})

		Convey(`Will handle beginning-of-query globbing.`, func() {
			q, err := NewLogStreamQuery("pfx/+/**/foo/*/bar")
			So(err, ShouldBeNil)

			So(getAllMatches(q,
				"pfx/+/extra/foo/a/bar",
				"pfx/+/even/more/extra/foo/a/bar",
				"pfx/+/foo/a/bar",

				"pfx/+/foo/a/bar/extra",
				"pfx/+/foo/bar",
			), shouldHaveLogPaths,
				"pfx/+/foo/a/bar",
				"pfx/+/even/more/extra/foo/a/bar",
				"pfx/+/extra/foo/a/bar",
			)
		})

		Convey(`Can handle middle-of-query globbing.`, func() {
			q, err := NewLogStreamQuery("pfx/+/*/foo/*/**/bar/*/baz/*")
			So(err, ShouldBeNil)

			So(getAllMatches(q,
				"pfx/+/a/foo/b/stuff/bar/c/baz/d",
				"pfx/+/a/foo/b/lots/of/stuff/bar/c/baz/d",
				"pfx/+/a/foo/b/bar/c/baz/d",

				"pfx/+/foo/a/bar/b/baz/c",
			), shouldHaveLogPaths,
				"pfx/+/a/foo/b/bar/c/baz/d",
				"pfx/+/a/foo/b/lots/of/stuff/bar/c/baz/d",
				"pfx/+/a/foo/b/stuff/bar/c/baz/d",
			)
		})
	})
}
