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
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func shouldHaveLogPaths(actual interface{}, expected ...interface{}) string {
	resp := actual.(*logs.QueryResponse)
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

	return ShouldResembleV(paths, exp)
}

func TestQuery(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration, a Query request`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		c, fb := featureBreaker.FilterRDS(c, nil)

		di := ds.Get(c)

		// Add LogStream indexes.
		//
		// These should be kept in sync with the "query endpoint" indexes in the
		// vmuser module's "index.yaml" file.
		indexDefs := [][]string{
			{"Prefix", "-Created"},
			{"Name", "-Created"},
			{"State", "-Created"},
			{"Purged", "-Created"},
			{"ProtoVersion", "-Created"},
			{"ContentType", "-Created"},
			{"StreamType", "-Created"},
			{"Timestamp", "-Created"},
			{"_C", "-Created"},
			{"_Tags", "-Created"},
			{"_Terminated", "-Created"},
			{"_Archived", "-Created"},
		}
		indexes := make([]*ds.IndexDefinition, len(indexDefs))
		for i, id := range indexDefs {
			cols := make([]ds.IndexColumn, len(id))
			for j, ic := range id {
				var err error
				cols[j], err = ds.ParseIndexColumn(ic)
				if err != nil {
					panic(fmt.Errorf("failed to parse index %q: %s", ic, err))
				}
			}
			indexes[i] = &ds.IndexDefinition{Kind: "LogStream", SortBy: cols}
		}
		di.Testable().AddIndexes(indexes...)
		di.Testable().Consistent(true)

		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		c = ct.UseConfig(c, &svcconfig.Coordinator{
			AdminAuthGroup: "test-administrators",
		})

		s := Server{}

		req := logs.QueryRequest{
			Tags: map[string]string{},
		}

		// Install a set of stock log streams to query against.
		var streamPaths []string
		var purgedStreamPaths []string
		streams := map[string]*coordinator.LogStream{}
		descs := map[string]*logpb.LogStreamDescriptor{}
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
			desc.Tags = map[string]string{
				"prefix": string(prefix),
				"name":   string(name),
			}

			// Set an empty tag for each name segment.
			for _, p := range name.Segments() {
				desc.Tags[p] = ""
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

					case "terminated":
						ls.State = coordinator.LSTerminated
						ls.TerminalIndex = 1337

					case "archived":
						ls.ArchiveStreamURL = "http://example.com"
						ls.State = coordinator.LSArchived

					case "datagram":
						ls.StreamType = logpb.StreamType_DATAGRAM

					case "binary":
						ls.StreamType = logpb.StreamType_BINARY
					}
				}
			}

			if err := ls.Put(ds.Get(c)); err != nil {
				panic(fmt.Errorf("failed to put log stream %d: %v", i, err))
			}

			descs[string(v)] = desc
			streams[string(v)] = ls
			if !ls.Purged {
				streamPaths = append(streamPaths, string(v))
			}
			purgedStreamPaths = append(purgedStreamPaths, string(v))
			tc.Add(time.Second)
		}
		di.Testable().CatchupIndexes()

		// Invert streamPaths since we will return results in descending Created
		// order.
		invert := func(s []string) {
			for i := 0; i < len(s)/2; i++ {
				eidx := len(s) - i - 1
				s[i], s[eidx] = s[eidx], s[i]
			}
		}
		invert(streamPaths)
		invert(purgedStreamPaths)

		Convey(`An empty query will return all log streams.`, func() {
			resp, err := s.Query(c, &req)
			So(err, ShouldBeRPCOK)
			So(resp, shouldHaveLogPaths, streamPaths)
		})

		Convey(`An empty query will include purged streams if admin.`, func() {
			fs.IdentityGroups = []string{"test-administrators"}

			resp, err := s.Query(c, &req)
			So(err, ShouldBeRPCOK)
			So(resp, shouldHaveLogPaths, purgedStreamPaths)
		})

		Convey(`A query with an invalid path will return BadRequest error.`, func() {
			req.Path = "***"

			_, err := s.Query(c, &req)
			So(err, ShouldBeRPCInvalidArgument, "invalid query `path`")
		})

		Convey(`A query with an invalid Next cursor will return BadRequest error.`, func() {
			req.Next = "invalid"
			fb.BreakFeatures(errors.New("testing error"), "DecodeCursor")

			_, err := s.Query(c, &req)
			So(err, ShouldBeRPCInvalidArgument, "invalid `next` value")
		})

		Convey(`A datastore query error will return InternalServer error.`, func() {
			fb.BreakFeatures(errors.New("testing error"), "Run")

			_, err := s.Query(c, &req)
			So(err, ShouldBeRPCInternal)
		})

		Convey(`When querying for "testing/+/baz"`, func() {
			req.Path = "testing/+/baz"

			stream := streams["testing/+/baz"]
			desc := descs["testing/+/baz"]

			Convey(`State is not returned.`, func() {
				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "testing/+/baz")

				So(resp.Streams, ShouldHaveLength, 1)
				So(resp.Streams[0].State, ShouldBeNil)
				So(resp.Streams[0].Desc, ShouldBeNil)
				So(resp.Streams[0].DescProto, ShouldBeNil)
			})

			Convey(`When requesting state`, func() {
				req.State = true

				Convey(`When not requesting protobufs, returns a descriptor structure.`, func() {
					resp, err := s.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths, "testing/+/baz")

					So(resp.Streams, ShouldHaveLength, 1)
					So(resp.Streams[0].State, ShouldResembleV, loadLogStreamState(stream))
					So(resp.Streams[0].Desc, ShouldResembleV, desc)
					So(resp.Streams[0].DescProto, ShouldBeNil)
				})

				Convey(`When not requesting protobufs, and with a corrupt descriptor, returns InternalServer error.`, func() {
					// We can't use "stream.Put" here because it validates the protobuf!
					stream.Descriptor = []byte{0x00} // Invalid protobuf, zero tag.
					So(di.Put(stream), ShouldBeNil)
					di.Testable().CatchupIndexes()

					_, err := s.Query(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`When requesting protobufs, returns the raw protobuf descriptor.`, func() {
					req.Proto = true

					resp, err := s.Query(c, &req)
					So(err, ShouldBeNil)
					So(resp, shouldHaveLogPaths, "testing/+/baz")

					So(resp.Streams, ShouldHaveLength, 1)
					So(resp.Streams[0].State, ShouldResembleV, loadLogStreamState(stream))
					So(resp.Streams[0].Desc, ShouldResembleV, desc)
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
					So(err, ShouldBeRPCOK)

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
			req.Older = google.NewTimestamp(tc.Now())

			// Invert our streamPaths, since we're going to receive results
			// newest-to-oldest and this makes it much easier to check/express.
			for i := 0; i < len(streamPaths)/2; i++ {
				eidx := len(streamPaths) - i - 1
				streamPaths[i], streamPaths[eidx] = streamPaths[eidx], streamPaths[i]
			}

			Convey(`Querying for entries created at or after 2 seconds ago (latest 2 entries).`, func() {
				req.Newer = google.NewTimestamp(tc.Now().Add(-2*time.Second - time.Millisecond))

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "testing/+/baz", "testing/+/foo/bar/baz")
			})

			Convey(`With a query limit of 3`, func() {
				s.queryResultLimit = 3

				Convey(`A query request will return the newest 3 entries and have a Next cursor for the next 3.`, func() {
					resp, err := s.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths, "testing/+/baz", "testing/+/foo/bar/baz", "meta/binary/+/foo")
					So(resp.Next, ShouldNotEqual, "")

					// Iterate.
					req.Next = resp.Next

					resp, err = s.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths, "meta/datagram/+/foo", "meta/archived/+/foo", "meta/terminated/+/foo")
				})

				Convey(`A datastore query error will return InternalServer error.`, func() {
					fb.BreakFeatures(errors.New("testing error"), "Run")

					_, err := s.Query(c, &req)
					So(err, ShouldBeRPCInternal)
				})
			})
		})

		Convey(`When querying for meta streams`, func() {
			req.Path = "meta/**/+/**"

			Convey(`When terminated=yes, returns [archived, terminated].`, func() {
				req.Terminated = logs.QueryRequest_YES

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/archived/+/foo", "meta/terminated/+/foo")
			})

			Convey(`When terminated=no, returns [binary, datagram]`, func() {
				req.Terminated = logs.QueryRequest_NO

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/binary/+/foo", "meta/datagram/+/foo")
			})

			Convey(`When archived=yes, returns [archived]`, func() {
				req.Archived = logs.QueryRequest_YES

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/archived/+/foo")
			})

			Convey(`When archived=no, returns [binary, datagram, terminated]`, func() {
				req.Archived = logs.QueryRequest_NO

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/binary/+/foo", "meta/datagram/+/foo", "meta/terminated/+/foo")
			})

			Convey(`When purged=yes, returns BadRequest error.`, func() {
				req.Purged = logs.QueryRequest_YES

				_, err := s.Query(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "non-admin user cannot request purged log streams")
			})

			Convey(`When the user is an administrator`, func() {
				fs.IdentityGroups = []string{"test-administrators"}

				Convey(`When purged=yes, returns [terminated/archived/purged, purged]`, func() {
					req.Purged = logs.QueryRequest_YES

					resp, err := s.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths, "meta/terminated/archived/purged/+/foo", "meta/purged/+/foo")
				})

				Convey(`When purged=no, returns [binary, datagram, archived, terminated]`, func() {
					req.Purged = logs.QueryRequest_NO

					resp, err := s.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths, "meta/binary/+/foo", "meta/datagram/+/foo", "meta/archived/+/foo", "meta/terminated/+/foo")
				})
			})

			Convey(`When querying for text streams, returns [archived, terminated]`, func() {
				req.StreamType = &logs.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_TEXT}

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/archived/+/foo", "meta/terminated/+/foo")
			})

			Convey(`When querying for binary streams, returns [binary]`, func() {
				req.StreamType = &logs.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_BINARY}

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/binary/+/foo")
			})

			Convey(`When querying for datagram streams, returns [datagram]`, func() {
				req.StreamType = &logs.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_DATAGRAM}

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/datagram/+/foo")
			})

			Convey(`When querying for an invalid stream type, returns a BadRequest error.`, func() {
				req.StreamType = &logs.QueryRequest_StreamTypeFilter{Value: -1}

				_, err := s.Query(c, &req)
				So(err, ShouldBeRPCInvalidArgument)
			})
		})

		Convey(`When querying for content type "other", returns [other/+/baz, other/+/foo/bar].`, func() {
			req.ContentType = "other"

			resp, err := s.Query(c, &req)
			So(err, ShouldBeRPCOK)
			So(resp, shouldHaveLogPaths, "other/+/baz", "other/+/foo/bar")
		})

		Convey(`When querying for proto version, the current version returns all non-purged streams.`, func() {
			req.ProtoVersion = logpb.Version

			resp, err := s.Query(c, &req)
			So(err, ShouldBeRPCOK)
			So(resp, shouldHaveLogPaths, streamPaths)
		})

		Convey(`When querying for proto version, "invalid" returns nothing.`, func() {
			req.ProtoVersion = "invalid"

			resp, err := s.Query(c, &req)
			So(err, ShouldBeRPCOK)
			So(resp, shouldHaveLogPaths)
		})

		Convey(`When querying for tags`, func() {
			Convey(`Tag "baz", returns [testing/+/baz, testing/+/foo/bar/baz, other/+/baz]`, func() {
				req.Tags["baz"] = ""

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "testing/+/baz", "testing/+/foo/bar/baz", "other/+/baz")
			})

			Convey(`Tags "prefix=testing", "baz", returns [testing/+/baz, testing/+/foo/bar/baz]`, func() {
				req.Tags["baz"] = ""
				req.Tags["prefix"] = "testing"

				resp, err := s.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "testing/+/baz", "testing/+/foo/bar/baz")
			})

			Convey(`When an invalid tag is specified, returns BadRequest error`, func() {
				req.Tags["+++not a valid tag+++"] = ""

				_, err := s.Query(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "invalid tag constraint")
			})
		})
	})
}
