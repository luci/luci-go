// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
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

	case []*TimestampIndexedQueryFields:
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
	return ShouldResembleV(names, exp)
}

func TestLogStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		tds := ds.Get(c).Testable()

		desc := protocol.LogStreamDescriptor{
			Prefix:      "testing",
			Name:        "log/stream",
			StreamType:  protocol.LogStreamDescriptor_TEXT,
			ContentType: "application/text",
			Timestamp:   google.NewTimestamp(clock.Now(c)),
			Tags: []*protocol.LogStreamDescriptor_Tag{
				{"foo", "bar"},
				{"baz", "qux"},
				{"quux", ""},
			},
		}

		Convey(`Can create a LogStream keyed on hash.`, func() {
			ls, err := NewLogStream("0123456789abcdef0123456789ABCDEF0123456789abcdef0123456789ABCDEF")
			So(err, ShouldBeNil)
			So(ls, ShouldNotBeNil)
		})

		Convey(`Will fail to create a LogStream keyed on an invalid hash-length string.`, func() {
			_, err := NewLogStream("0123456789abcdef!@#$%^&*()ABCDEF0123456789abcdef0123456789ABCDEF")
			So(err, ShouldErrLike, "invalid path")
		})

		Convey(`Can create a LogStream keyed on path.`, func() {
			ls, err := NewLogStream("a/b/+/c/d")
			So(err, ShouldBeNil)
			So(ls, ShouldNotBeNil)
		})

		Convey(`Will fail to create a LogStream keyed on neither a path nor hash.`, func() {
			_, err := NewLogStream("")
			So(err, ShouldNotBeNil)
		})

		Convey(`Can create a new LogStream`, func() {
			ls, err := NewLogStream(string(desc.Path()))
			So(err, ShouldBeNil)

			Convey(`Can populate the LogStream with descriptor state.`, func() {
				ls.Created = NormalizeTime(clock.Now(c).UTC())
				ls.Updated = NormalizeTime(clock.Now(c).UTC())
				ls.Secret = bytes.Repeat([]byte{0x6F}, types.StreamSecretLength)
				So(ls.LoadDescriptor(&desc), ShouldBeNil)
				So(ls.Validate(), ShouldBeNil)

				Convey(`Will not validate`, func() {
					Convey(`Without a valid Prefix`, func() {
						ls.Prefix = ""
						So(ls.Validate(), ShouldErrLike, "invalid prefix")
					})
					Convey(`Without a valid prefix`, func() {
						ls.Name = ""
						So(ls.Validate(), ShouldErrLike, "invalid name")
					})
					Convey(`Without a valid stream secret`, func() {
						ls.Secret = nil
						So(ls.Validate(), ShouldErrLike, "invalid secret length")
					})
					Convey(`Without a valid content type`, func() {
						ls.ContentType = ""
						So(ls.Validate(), ShouldErrLike, "empty content type")
					})
					Convey(`Without a valid created time`, func() {
						ls.Created = time.Time{}
						So(ls.Validate(), ShouldErrLike, "created time is not set")
					})
					Convey(`Without a valid updated time`, func() {
						ls.Updated = time.Time{}
						So(ls.Validate(), ShouldErrLike, "updated time is not set")
					})
					Convey(`With an updated time before the created time`, func() {
						ls.Updated = ls.Created.Add(-time.Second)
						So(ls.Validate(), ShouldErrLike, "updated time must be >= created time")
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
						So(ls.Validate(), ShouldErrLike, "invalid descriptor protobuf")
					})
				})

				Convey(`Can write the LogStream to the Datastore.`, func() {
					So(ls.Put(ds.Get(c)), ShouldBeNil)
					tds.CatchupIndexes()

					Convey(`Can read the LogStream back from the Datastore.`, func() {
						ls2 := LogStreamFromID(ls.HashID())
						So(ds.Get(c).Get(ls2), ShouldBeNil)
						So(ls, ShouldResembleV, ls2)
					})

					Convey(`Will also write a timestamp entry.`, func() {
						q := ds.NewQuery("TimestampIndexedQueryFields")

						qfs := []*TimestampIndexedQueryFields(nil)
						So(ds.Get(c).Run(q, func(qf *TimestampIndexedQueryFields) error {
							qfs = append(qfs, qf)
							return nil
						}), ShouldBeNil)

						So(len(qfs), ShouldEqual, 1)
						So(qfs[0].QueryBase, ShouldResembleV, ls.QueryBase)
						So(qfs[0].LogStream.Equal(ds.Get(c).KeyForObj(ls)), ShouldBeTrue)
					})
				})

				Convey(`Will refuse to write an invalid LogStream.`, func() {
					ls.Secret = nil
					So(ls.Put(ds.Get(c)), ShouldErrLike, "invalid secret length")
				})
			})

			Convey(`Will refuse to populate from an invalid descriptor.`, func() {
				desc.Name = ""
				So(ls.LoadDescriptor(&desc), ShouldErrLike, "invalid descriptor")
			})
		})

		Convey(`Writing multiple LogStream entries`, func() {
			di := ds.Get(c)

			times := map[string]time.Time{}
			streamNames := []string{
				"foo/bar",
				"foo/bar/baz",
				"baz/qux",
				"cat/dog",
				"cat/bird/dog",
				"bird/plane",
			}
			for _, name := range streamNames {
				desc := desc
				desc.Name = name

				ls, err := NewLogStream(string(desc.Path()))
				So(err, ShouldBeNil)

				ls.Secret = bytes.Repeat([]byte{0x55}, types.StreamSecretLength)
				So(ls.LoadDescriptor(&desc), ShouldBeNil)
				ls.Created = clock.Now(c).UTC()
				ls.Updated = ls.Created
				So(ls.Put(di), ShouldBeNil)

				times[name] = clock.Now(c)
				tc.Add(time.Second)
			}
			di.Testable().CatchupIndexes()

			Convey(`LogStream path queries`, func() {
				Convey(`A query for "foo/bar" should return "foo/bar".`, func() {
					q := ds.NewQuery("LogStream")
					q, err := AddLogStreamPathQuery(q, "**/+/foo/bar")
					So(err, ShouldBeNil)

					var streams []*LogStream
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "foo/bar")
				})

				Convey(`A query for "foo/bar/*" should return "foo/bar/baz".`, func() {
					q := ds.NewQuery("LogStream")
					q, err := AddLogStreamPathQuery(q, "**/+/foo/bar/*")
					So(err, ShouldBeNil)

					var streams []*LogStream
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "foo/bar/baz")
				})

				Convey(`A query for "foo/**" should return "foo/bar/baz" and "foo/bar".`, func() {
					q := ds.NewQuery("LogStream")
					q, err := AddLogStreamPathQuery(q, "**/+/foo/**")
					So(err, ShouldBeNil)

					var streams []*LogStream
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "foo/bar/baz", "foo/bar")
				})

				Convey(`A query for "cat/**/dog" should return "cat/dog" and "cat/bird/dog".`, func() {
					q := ds.NewQuery("LogStream")
					q, err := AddLogStreamPathQuery(q, "**/+/cat/**/dog")
					So(err, ShouldBeNil)

					var streams []*LogStream
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "cat/bird/dog", "cat/dog")
				})
			})

			Convey(`TimestampIndexedQueryFields queries (keyed newest-to-oldest`, func() {
				Convey(`A query for all records returns them in reverse order.`, func() {
					q := ds.NewQuery("TimestampIndexedQueryFields")

					// Reverse "streamNames".
					si := make([]interface{}, len(streamNames))
					for i := 0; i < len(streamNames); i++ {
						si[i] = interface{}(streamNames[len(streamNames)-i-1])
					}

					var streams []*TimestampIndexedQueryFields
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, si...)
				})

				Convey(`A query for "cat/**/dog" should return "cat/bird/dog" and "cat/dog".`, func() {
					q := ds.NewQuery("TimestampIndexedQueryFields")
					q, err := AddLogStreamPathQuery(q, "**/+/cat/**/dog")
					So(err, ShouldBeNil)

					var streams []*TimestampIndexedQueryFields
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "cat/bird/dog", "cat/dog")
				})

				Convey(`A query for streams older than "baz/qux" returns {"baz/qux", "foo/bar/baz", and "foo/bar"}.`, func() {
					q := ds.NewQuery("TimestampIndexedQueryFields")
					q = AddOlderFilter(q, di, times["baz/qux"])

					var streams []*TimestampIndexedQueryFields
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "baz/qux", "foo/bar/baz", "foo/bar")
				})

				Convey(`A query for streams newer than "cat/dog" returns {"bird/plane", "cat/bird/dog", "cat/dog"}.`, func() {
					q := ds.NewQuery("TimestampIndexedQueryFields")
					q = AddNewerFilter(q, di, times["cat/dog"])

					var streams []*TimestampIndexedQueryFields
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "bird/plane", "cat/bird/dog", "cat/dog")
				})

				Convey(`A query for "cat/**/dog" newer than "cat/dog" returns {"cat/bird/dog", "cat/dog"}.`, func() {
					q := ds.NewQuery("TimestampIndexedQueryFields")
					q = AddNewerFilter(q, di, times["cat/dog"])

					q, err := AddLogStreamPathQuery(q, "**/+/cat/**/dog")
					So(err, ShouldBeNil)

					var streams []*TimestampIndexedQueryFields
					So(di.GetAll(q, &streams), ShouldBeNil)
					So(streams, shouldHaveLogStreams, "cat/bird/dog", "cat/dog")
				})
			})
		})
	})
}
