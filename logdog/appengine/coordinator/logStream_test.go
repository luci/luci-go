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
	"fmt"
	"testing"
	"time"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func shouldHaveLogStreams(actual interface{}, expected ...interface{}) string {
	var names []string
	switch t := actual.(type) {
	case error:
		return t.Error()

	case []*LogStream:
		for _, ls := range t {
			names = append(names, ls.Name)
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

// sps constructs a ds.PropertySlice from a set of interfaces.
func sps(values ...interface{}) ds.PropertySlice {
	ps := make(ds.PropertySlice, len(values))
	for i, v := range values {
		ps[i] = ds.MkProperty(v)
	}
	return ps
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
			ID:          LogStreamID("testing/+/log/stream"),
			Prefix:      "testing",
			Name:        "log/stream",
			Created:     now.UTC(),
			ContentType: string(types.ContentTypeText),
		}

		desc := logpb.LogStreamDescriptor{
			Prefix:      "testing",
			Name:        "log/stream",
			StreamType:  logpb.StreamType_TEXT,
			ContentType: "application/text",
			Timestamp:   google.NewTimestamp(now),
			Tags: map[string]string{
				"foo":  "bar",
				"baz":  "qux",
				"quux": "",
			},
		}

		Convey(`Will skip invalid tags when loading.`, func() {
			pmap, err := ls.Save(false)
			So(err, ShouldBeNil)
			pmap["_Tags"] = sps(encodeKey("!!!invalid key!!!"))

			So(ls.Load(pmap), ShouldBeNil)
			So(ls.Tags, ShouldResemble, TagMap(nil))
		})

		Convey(`With invalid tags will fail to encode.`, func() {
			ls.Tags = TagMap{
				"!!!invalid key!!!": "value",
			}

			ls.SetDSValidate(false)
			_, err := ls.Save(true)
			So(err, ShouldErrLike, "failed to encode tags")
		})

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
				Convey(`Without a valid content type`, func() {
					ls.ContentType = ""
					So(ls.Validate(), ShouldErrLike, "empty content type")
				})
				Convey(`Without a valid created time`, func() {
					ls.Created = time.Time{}
					So(ls.Validate(), ShouldErrLike, "created time is not set")
				})
				Convey(`Without a valid stream type`, func() {
					ls.StreamType = -1
					So(ls.Validate(), ShouldErrLike, "unsupported stream type")
				})
				Convey(`Without an invalid tag: empty key`, func() {
					ls.Tags[""] = "empty"
					So(ls.Validate(), ShouldErrLike, "invalid tag")
				})
				Convey(`Without an invalid tag: bad key`, func() {
					ls.Tags["!"] = "bad-value"
					So(ls.Validate(), ShouldErrLike, "invalid tag")
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
			times := map[string]time.Time{}
			streamNames := []string{
				"foo/bar",
				"foo/bar/baz",
				"baz/qux",
				"cat/dog",
				"cat/bird/dog",
				"bird/plane",
			}
			for i, name := range streamNames {
				lsCopy := ls
				lsCopy.Name = name
				lsCopy.Created = ds.RoundTime(now.Add(time.Duration(i) * time.Second))
				updateLogStreamID(&lsCopy)

				descCopy := desc
				descCopy.Name = name

				if err := lsCopy.LoadDescriptor(&descCopy); err != nil {
					panic(err)
				}
				So(ds.Put(c, &lsCopy), ShouldBeNil)

				times[name] = lsCopy.Created
			}

			Convey(`When querying LogStream by -Created`, func() {
				q := ds.NewQuery("LogStream").Order("-Created")

				Convey(`LogStream path queries`, func() {
					Convey(`A query for "foo/bar" should return "foo/bar".`, func() {
						q, err := AddLogStreamPathFilter(q, "**/+/foo/bar")
						So(err, ShouldBeNil)

						var streams []*LogStream
						So(ds.GetAll(c, q, &streams), ShouldBeNil)
						So(streams, shouldHaveLogStreams, "foo/bar")
					})

					Convey(`A query for "foo/bar/*" should return "foo/bar/baz".`, func() {
						q, err := AddLogStreamPathFilter(q, "**/+/foo/bar/*")
						So(err, ShouldBeNil)

						var streams []*LogStream
						So(ds.GetAll(c, q, &streams), ShouldBeNil)
						So(streams, shouldHaveLogStreams, "foo/bar/baz")
					})

					Convey(`A query for "foo/**" should return "foo/bar/baz" and "foo/bar".`, func() {
						q, err := AddLogStreamPathFilter(q, "**/+/foo/**")
						So(err, ShouldBeNil)

						var streams []*LogStream
						So(ds.GetAll(c, q, &streams), ShouldBeNil)
						So(streams, shouldHaveLogStreams, "foo/bar/baz", "foo/bar")
					})

					Convey(`A query for "cat/**/dog" should return "cat/dog" and "cat/bird/dog".`, func() {
						q, err := AddLogStreamPathFilter(q, "**/+/cat/**/dog")
						So(err, ShouldBeNil)

						var streams []*LogStream
						So(ds.GetAll(c, q, &streams), ShouldBeNil)
						So(streams, shouldHaveLogStreams, "cat/bird/dog", "cat/dog")
					})
				})

				Convey(`A timestamp inequality query for all records returns them in reverse order.`, func() {
					// Reverse "streamNames".
					si := make([]interface{}, len(streamNames))
					for i := 0; i < len(streamNames); i++ {
						si[i] = interface{}(streamNames[len(streamNames)-i-1])
					}

					var streams []*LogStream
					So(ds.GetAll(c, q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, si...)
				})

				Convey(`A query for "cat/**/dog" should return "cat/bird/dog" and "cat/dog".`, func() {
					q, err := AddLogStreamPathFilter(q, "**/+/cat/**/dog")
					So(err, ShouldBeNil)

					var streams []*LogStream
					So(ds.GetAll(c, q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "cat/bird/dog", "cat/dog")
				})

				Convey(`A query for streams older than "baz/qux" returns {"foo/bar/baz", and "foo/bar"}.`, func() {
					q = AddOlderFilter(q, times["baz/qux"])

					var streams []*LogStream
					So(ds.GetAll(c, q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "foo/bar/baz", "foo/bar")
				})

				Convey(`A query for streams newer than "cat/dog" returns {"bird/plane", "cat/bird/dog"}.`, func() {
					q = AddNewerFilter(q, times["cat/dog"])

					var streams []*LogStream
					So(ds.GetAll(c, q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "bird/plane", "cat/bird/dog")
				})

				Convey(`A query for "cat/**/dog" newer than "cat/dog" returns {"cat/bird/dog"}.`, func() {
					q = AddNewerFilter(q, times["cat/dog"])

					q, err := AddLogStreamPathFilter(q, "**/+/cat/**/dog")
					So(err, ShouldBeNil)

					var streams []*LogStream
					So(ds.GetAll(c, q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "cat/bird/dog")
				})
			})
		})
	})
}

func TestLogStreamPathFilter(t *testing.T) {
	t.Parallel()

	Convey(`A testing query`, t, func() {
		q := ds.NewQuery("LogStream")

		Convey(`Will construct a non-globbing query as Prefix/Name equality.`, func() {
			q, err := AddLogStreamPathFilter(q, "foo/bar/+/baz/qux")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResemble, map[string]ds.PropertySlice{
				"Prefix": sps("foo/bar"),
				"Name":   sps("baz/qux"),
			})
		})

		Convey(`Will refuse to query an invalid Prefix/Name.`, func() {
			_, err := AddLogStreamPathFilter(q, "////+/baz/qux")
			So(err, ShouldErrLike, "invalid Prefix component")

			_, err = AddLogStreamPathFilter(q, "foo/bar/+//////")
			So(err, ShouldErrLike, "invalid Name component")
		})

		Convey(`Will not impose any filters on an empty Prefix.`, func() {
			q, err := AddLogStreamPathFilter(q, "/+/baz/qux")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResemble, map[string]ds.PropertySlice{
				"Name": sps("baz/qux"),
			})
		})

		Convey(`Will not impose any filters on an empty Name.`, func() {
			q, err := AddLogStreamPathFilter(q, "baz/qux")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResemble, map[string]ds.PropertySlice{
				"Prefix": sps("baz/qux"),
			})
		})

		Convey(`Will glob out single Prefix components.`, func() {
			q, err := AddLogStreamPathFilter(q, "foo/*/*/bar/*/baz/qux/*")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResemble, map[string]ds.PropertySlice{
				"_C": sps("PC:8", "PF:0:foo", "PF:3:bar", "PF:5:baz", "PF:6:qux"),
			})
		})

		Convey(`Will handle end-of-query globbing.`, func() {
			q, err := AddLogStreamPathFilter(q, "foo/*/bar/**")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResemble, map[string]ds.PropertySlice{
				"_C": sps("PF:0:foo", "PF:2:bar"),
			})
		})

		Convey(`Will handle beginning-of-query globbing.`, func() {
			q, err := AddLogStreamPathFilter(q, "**/foo/*/bar")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResemble, map[string]ds.PropertySlice{
				"_C": sps("PR:0:bar", "PR:2:foo"),
			})
		})

		Convey(`Can handle middle-of-query globbing.`, func() {
			q, err := AddLogStreamPathFilter(q, "*/foo/*/**/bar/*/baz/*")
			So(err, ShouldBeNil)

			fq, err := q.Finalize()
			So(err, ShouldBeNil)
			So(fq.EqFilters(), ShouldResemble, map[string]ds.PropertySlice{
				"_C": sps("PF:1:foo", "PR:1:baz", "PR:3:bar"),
			})
		})

		Convey(`Will error if more than one greedy glob is present.`, func() {
			_, err := AddLogStreamPathFilter(q, "*/foo/**/bar/**")
			So(err, ShouldErrLike, "cannot have more than one greedy glob")
		})
	})
}
