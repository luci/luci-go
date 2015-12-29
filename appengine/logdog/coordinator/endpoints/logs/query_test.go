// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/services"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/appengine/ephelper/assertions"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

type checker struct {
	order bool
}

func (chk *checker) shouldHaveLogPaths(actual interface{}, expected ...interface{}) string {
	resp := actual.(*QueryResponse)
	var paths []string
	if len(resp.Streams) > 0 {
		paths = make([]string, len(resp.Streams))
		for i, s := range resp.Streams {
			paths[i] = s.Path
		}
	}

	var exp []string
	for _, e := range expected {
		switch t := e.(type) {
		case []string:
			exp = append(exp, t...)

		case string:
			exp = append(exp, t)

		case types.StreamPath:
			exp = append(exp, string(t))

		default:
			panic(fmt.Errorf("unsupported expected type %T: %v", t, t))
		}
	}

	if !chk.order {
		sort.Strings(paths)
		sort.Strings(exp)
	}

	return ShouldResembleV(paths, exp)
}

func TestQuery(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration, a Query request`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		c, fb := featureBreaker.FilterRDS(c, nil)

		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		c = ct.UseConfig(c, &services.Coordinator{
			AdminAuthGroup: "test-administrators",
		})

		s := Logs{
			ServiceBase: ephelper.ServiceBase{
				Middleware: ephelper.TestMode,
			},
		}

		req := QueryRequest{}

		// Install a set of stock log streams to query against.
		var streamPaths []string
		var purgedStreamPaths []string
		streams := map[string]*coordinator.LogStream{}
		descs := map[string]*protocol.LogStreamDescriptor{}
		for i, v := range []types.StreamPath{
			"testing/+/foo",
			"testing/+/foo/bar",
			"other/+/foo/bar",
			"other/+/baz",

			"meta/terminated/+/foo",
			"meta/archived/+/foo",
			"meta/purged/+/foo",
			"meta/terminated/archived/purged/+/foo",
			"meta/datagram/+/foo",
			"meta/binary/+/foo",

			"testing/+/foo/bar/baz",
			"testing/+/baz",
		} {
			prefix, name := v.Split()
			desc := ct.TestLogStreamDescriptor(c, string(name))
			desc.Prefix = string(prefix)
			desc.ContentType = string(prefix)
			desc.Tags = []*protocol.LogStreamDescriptor_Tag{
				{Key: "prefix", Value: string(prefix)},
				{Key: "name", Value: string(name)},
			}

			ls, err := ct.TestLogStream(c, desc)
			if err != nil {
				panic(fmt.Errorf("failed to generate log stream %d: %v", i, err))
			}

			psegs := prefix.Segments()
			if psegs[0] == "meta" {
				for _, p := range psegs[1:] {
					switch p {
					case "purged":
						ls.Purged = true

					case "archived":
						ls.ArchiveStreamURL = "http://example.com"

					case "terminated":
						ls.TerminalIndex = 1337

					case "datagram":
						ls.StreamType = protocol.LogStreamDescriptor_DATAGRAM

					case "binary":
						ls.StreamType = protocol.LogStreamDescriptor_BINARY
					}
				}
			}

			// Set an empty tag for each name segment.
			for _, p := range name.Segments() {
				ls.Tags[p] = ""
			}

			if err := ls.Put(ds.Get(c)); err != nil {
				panic(fmt.Errorf("failed to put log stream %d: %v", i, err))
			}

			descs[string(v)] = desc
			streams[string(v)] = ls
			if !ls.Purged {
				streamPaths = append(streamPaths, string(v))
			} else {
				purgedStreamPaths = append(purgedStreamPaths, string(v))
			}
			tc.Add(time.Second)
		}
		ds.Get(c).Testable().CatchupIndexes()

		chk := checker{}

		Convey(`An empty query will return all log streams.`, func() {
			resp, err := s.Query(c, &req)
			So(err, ShouldBeNil)
			So(resp, chk.shouldHaveLogPaths, streamPaths)
		})

		Convey(`An empty query will include purged streams if admin.`, func() {
			fs.IdentityGroups = []string{"test-administrators"}

			resp, err := s.Query(c, &req)
			So(err, ShouldBeNil)
			So(resp, chk.shouldHaveLogPaths, streamPaths, purgedStreamPaths)
		})

		Convey(`A query with an invalid path will return BadRequest error.`, func() {
			req.Path = "***"

			_, err := s.Query(c, &req)
			So(err, ShouldBeBadRequestError, "invalid query `path`")
		})

		Convey(`A query with an invalid Next cursor will return BadRequest error.`, func() {
			req.Next = "invalid"
			fb.BreakFeatures(errors.New("testing error"), "DecodeCursor")

			_, err := s.Query(c, &req)
			So(err, ShouldBeBadRequestError, "invalid `next` value")
		})

		Convey(`A datastore query error will return InternalServer error.`, func() {
			fb.BreakFeatures(errors.New("testing error"), "Run")

			_, err := s.Query(c, &req)
			So(err, ShouldBeInternalServerError)
		})

		Convey(`When querying for "testing/+/baz"`, func() {
			req.Path = "testing/+/baz"

			stream := streams["testing/+/baz"]
			desc := descs["testing/+/baz"]

			Convey(`State is not returned.`, func() {
				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "testing/+/baz")

				So(resp.Streams, ShouldHaveLength, 1)
				So(resp.Streams[0].State, ShouldBeNil)
				So(resp.Streams[0].Descriptor, ShouldBeNil)
				So(resp.Streams[0].DescriptorProto, ShouldBeNil)
			})

			Convey(`When requesting state`, func() {
				req.State = true

				Convey(`When not requesting protobufs, returns a descriptor structure.`, func() {
					resp, err := s.Query(c, &req)
					So(err, ShouldBeNil)
					So(resp, chk.shouldHaveLogPaths, "testing/+/baz")

					So(resp.Streams, ShouldHaveLength, 1)
					So(resp.Streams[0].State, ShouldResembleV, lep.LoadLogStreamState(stream))
					So(resp.Streams[0].Descriptor, ShouldResembleV, lep.DescriptorFromProto(desc))
					So(resp.Streams[0].DescriptorProto, ShouldBeNil)
				})

				Convey(`When not requesting protobufs, and with a corrupt descriptor, returns InternalServer error.`, func() {
					// We can't use "stream.Put" here because it validates the protobuf!
					stream.Descriptor = []byte{0x00} // Invalid protobuf, zero tag.
					So(ds.Get(c).Put(stream), ShouldBeNil)
					ds.Get(c).Testable().CatchupIndexes()

					_, err := s.Query(c, &req)
					So(err, ShouldBeInternalServerError)
				})

				Convey(`When requesting protobufs, returns the raw protobuf descriptor.`, func() {
					req.Proto = true

					resp, err := s.Query(c, &req)
					So(err, ShouldBeNil)
					So(resp, chk.shouldHaveLogPaths, "testing/+/baz")

					So(resp.Streams, ShouldHaveLength, 1)
					So(resp.Streams[0].State, ShouldResembleV, lep.LoadLogStreamState(stream))
					So(resp.Streams[0].Descriptor, ShouldBeNil)
					So(resp.Streams[0].DescriptorProto, ShouldResembleV, stream.Descriptor)
				})
			})
		})

		Convey(`With a query limit of 3`, func() {
			s.queryResultLimit = 3

			Convey(`Can iteratively query to retrieve all stream paths.`, func() {
				var seen []string

				next := ""
				for {
					req.Next = next

					resp, err := s.Query(c, &req)
					So(err, ShouldBeNil)

					for _, s := range resp.Streams {
						seen = append(seen, s.Path)
					}

					next = resp.Next
					if next == "" {
						break
					}
				}

				sort.Strings(seen)
				sort.Strings(streamPaths)
				So(seen, ShouldResembleV, streamPaths)
			})
		})

		Convey(`When querying against timestamp constraints`, func() {
			chk.order = false
			req.Older = lep.ToRFC3339(tc.Now())

			// Invert our streamPaths, since we're going to receive results
			// newest-to-oldest and this makes it much easier to check/express.
			for i := 0; i < len(streamPaths)/2; i++ {
				eidx := len(streamPaths) - i - 1
				streamPaths[i], streamPaths[eidx] = streamPaths[eidx], streamPaths[i]
			}

			Convey(`Querying for entries newer than 2 seconds ago (latest 2 entries).`, func() {
				req.Newer = lep.ToRFC3339(tc.Now().Add(-2 * time.Second))

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, streamPaths[:2])
			})

			Convey(`With a query limit of 3`, func() {
				s.queryResultLimit = 3

				Convey(`A timestamp-sorted query request will return the newest 3 entries and have a Next cursor.`, func() {
					resp, err := s.Query(c, &req)
					So(err, ShouldBeNil)
					So(resp, chk.shouldHaveLogPaths, streamPaths[:3])
					So(resp.Next, ShouldNotEqual, "")

					Convey(`An iterative query will return the next 3 entries.`, func() {
						req.Older = lep.ToRFC3339(tc.Now())
						req.Next = resp.Next

						resp, err := s.Query(c, &req)
						So(err, ShouldBeNil)
						So(resp, chk.shouldHaveLogPaths, streamPaths[3:6])
					})
				})

				Convey(`A datastore query error will return InternalServer error.`, func() {
					fb.BreakFeatures(errors.New("testing error"), "Run")

					_, err := s.Query(c, &req)
					So(err, ShouldBeInternalServerError)
				})

				Convey(`A datastore GetMulti error will return InternalServer error.`, func() {
					fb.BreakFeatures(errors.New("testing error"), "GetMulti")

					_, err := s.Query(c, &req)
					So(err, ShouldBeInternalServerError)
				})
			})

			Convey(`With 3 max results`, func() {
				req.MaxResults = 2

				Convey(`A timestamp-sorted query request will return the newest 2 entries and have a Next cursor.`, func() {
					req.Older = lep.ToRFC3339(tc.Now())

					resp, err := s.Query(c, &req)
					So(err, ShouldBeNil)
					So(resp, chk.shouldHaveLogPaths, streamPaths[:2])
					So(resp.Next, ShouldNotEqual, "")
				})
			})
		})

		Convey(`An invalid RFC3339 "Older" timestamp will return a BadRequest error.`, func() {
			req.Older = "invalid"

			_, err := s.Query(c, &req)
			So(err, ShouldBeBadRequestError)
		})

		Convey(`An invalid RFC3339 "Newer" timestamp will return a BadRequest error.`, func() {
			req.Newer = "invalid"

			_, err := s.Query(c, &req)
			So(err, ShouldBeBadRequestError)
		})

		Convey(`When querying for meta streams`, func() {
			req.Path = "meta/**/+/**"

			Convey(`When terminated=yes, returns [terminated].`, func() {
				req.Terminated = TrinaryYes

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "meta/terminated/+/foo")
			})

			Convey(`When terminated=no, returns [archived, binary, datagram]`, func() {
				req.Terminated = TrinaryNo

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "meta/archived/+/foo", "meta/binary/+/foo", "meta/datagram/+/foo")
			})

			Convey(`When archived=yes, returns [archived]`, func() {
				req.Archived = TrinaryYes

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "meta/archived/+/foo")
			})

			Convey(`When archived=no, returns [binary, datagram, terminated]`, func() {
				req.Archived = TrinaryNo

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "meta/binary/+/foo", "meta/datagram/+/foo", "meta/terminated/+/foo")
			})

			Convey(`When purged=yes, returns BadRequest error.`, func() {
				req.Purged = TrinaryYes

				_, err := s.Query(c, &req)
				So(err, ShouldBeBadRequestError, "non-admin user cannot request purged log streams")
			})

			Convey(`When the user is an administrator`, func() {
				fs.IdentityGroups = []string{"test-administrators"}

				Convey(`When purged=yes, returns [purged, terminated/archived/purged]`, func() {
					req.Purged = TrinaryYes

					resp, err := s.Query(c, &req)
					So(err, ShouldBeNil)
					So(resp, chk.shouldHaveLogPaths, "meta/purged/+/foo", "meta/terminated/archived/purged/+/foo")
				})

				Convey(`When purged=no, returns [archived, binary, datagram, terminated]`, func() {
					req.Purged = TrinaryNo

					resp, err := s.Query(c, &req)
					So(err, ShouldBeNil)
					So(resp, chk.shouldHaveLogPaths,
						"meta/archived/+/foo", "meta/binary/+/foo", "meta/datagram/+/foo", "meta/terminated/+/foo")
				})
			})

			Convey(`When querying for text streams, returns [archived, terminated]`, func() {
				req.StreamType = "text"

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "meta/archived/+/foo", "meta/terminated/+/foo")
			})

			Convey(`When querying for binary streams, returns [binary]`, func() {
				req.StreamType = "binary"

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "meta/binary/+/foo")
			})

			Convey(`When querying for datagram streams, returns [datagram]`, func() {
				req.StreamType = "datagram"

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "meta/datagram/+/foo")
			})

			Convey(`When querying for an invalid stream type, returns a BadRequest error.`, func() {
				req.StreamType = "invalid"

				_, err := s.Query(c, &req)
				So(err, ShouldBeBadRequestError)
			})
		})

		Convey(`When querying for content type "other", returns [other/+/foo/bar, other/+/baz].`, func() {
			req.ContentType = "other"

			resp, err := s.Query(c, &req)
			So(err, ShouldBeNil)
			So(resp, chk.shouldHaveLogPaths, "other/+/foo/bar", "other/+/baz")
		})

		Convey(`When querying for proto version, the current version returns all non-purged streams.`, func() {
			req.ProtoVersion = protocol.Version

			resp, err := s.Query(c, &req)
			So(err, ShouldBeNil)
			So(resp, chk.shouldHaveLogPaths, streamPaths)
		})

		Convey(`When querying for proto version, "invalid" returns nothing.`, func() {
			req.ProtoVersion = "invalid"

			resp, err := s.Query(c, &req)
			So(err, ShouldBeNil)
			So(resp, chk.shouldHaveLogPaths)
		})

		Convey(`When querying for tags`, func() {
			addTag := func(k, v string) {
				req.Tags = append(req.Tags, &lep.LogStreamDescriptorTag{Key: k, Value: v})
			}

			Convey(`Tag "baz", returns [testing/+/foo/bar/baz, testing/+/baz, other/+/baz]`, func() {
				addTag("baz", "")

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "testing/+/foo/bar/baz", "testing/+/baz", "other/+/baz")
			})

			Convey(`Tags "prefix=testing", "baz", returns [testing/+/foo/bar/baz, testing/+/baz]`, func() {
				addTag("baz", "")
				addTag("prefix", "testing")

				resp, err := s.Query(c, &req)
				So(err, ShouldBeNil)
				So(resp, chk.shouldHaveLogPaths, "testing/+/foo/bar/baz", "testing/+/baz")
			})

			Convey(`When an invalid tag is specified, returns BadRequest error`, func() {
				addTag("+++not a valid tag+++", "")

				_, err := s.Query(c, &req)
				So(err, ShouldBeBadRequestError, "invalid tag constraint")
			})

			Convey(`When the same tag is specified with different value, returns BadRequest error.`, func() {
				addTag("baz", "")
				addTag("baz", "ohai")

				_, err := s.Query(c, &req)
				So(err, ShouldBeBadRequestError, "conflicting tag query parameters")
			})
		})
	})
}
