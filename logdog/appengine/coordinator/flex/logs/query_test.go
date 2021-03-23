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

package logs

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	ds "go.chromium.org/luci/gae/service/datastore"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func shouldHaveLogPaths(actual interface{}, expected ...interface{}) string {
	resp := actual.(*logdog.QueryResponse)
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

	return ShouldResemble(paths, exp)
}

func TestQuery(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration, a Query request`, t, func() {
		c, env := ct.Install(true)
		c, fb := featureBreaker.FilterRDS(c, nil)

		svr := New()
		svrBase := svr.(*logdog.DecoratedLogs).Service.(*server)

		const project = "proj-foo"

		// Stock query request, will be modified by each test.
		req := logdog.QueryRequest{
			Project: project,
			Tags:    map[string]string{},
		}

		// Install a set of stock log streams to query against.
		prefixToStreamPaths := map[string][]string{}
		prefixToAllStreamPaths := map[string][]string{}

		streams := map[string]*ct.TestStream{}
		for i, v := range []types.StreamPath{
			"testing/+/foo",
			"testing/+/foo/bar",
			"other/+/foo/bar",
			"other/+/baz",

			"meta/+/terminated/foo",
			"meta/+/archived/foo",
			"meta/+/purged/foo",
			"meta/+/terminated/archived/purged/foo",
			"meta/+/datagram/foo",
			"meta/+/binary/foo",

			"testing/+/foo/bar/baz",
			"testing/+/baz",
		} {
			tls := ct.MakeStream(c, project, "", v)
			tls.Desc.ContentType = tls.Stream.Prefix
			tls.Desc.Tags = map[string]string{
				"prefix": tls.Stream.Prefix,
				"name":   tls.Stream.Name,
			}

			// Set an empty tag for each name segment.
			for _, p := range types.StreamName(tls.Stream.Name).Segments() {
				tls.Desc.Tags[p] = ""
			}

			now := env.Clock.Now().UTC()
			prefix := tls.Stream.Prefix
			psegs := types.StreamName(tls.Stream.Name).Segments()
			if prefix == "meta" {
				for _, p := range psegs {
					switch p {
					case "purged":
						tls.Stream.Purged = true

					case "archived":
						tls.State.ArchiveStreamURL = "http://example.com"
						tls.State.ArchivedTime = now
						So(tls.State.ArchivalState().Archived(), ShouldBeTrue)
						fallthrough // Archived streams are also terminated.

					case "terminated":
						tls.State.TerminalIndex = 1337
						tls.State.TerminatedTime = now
						So(tls.State.Terminated(), ShouldBeTrue)

					case "datagram":
						tls.Desc.StreamType = logpb.StreamType_DATAGRAM

					case "binary":
						tls.Desc.StreamType = logpb.StreamType_BINARY
					}
				}
			}

			tls.Reload(c)
			if err := tls.Put(c); err != nil {
				panic(fmt.Errorf("failed to put log stream %d: %v", i, err))
			}

			streams[string(v)] = tls
			if !tls.Stream.Purged {
				prefixToStreamPaths[prefix] = append(
					prefixToStreamPaths[prefix], string(v))
			}
			prefixToAllStreamPaths[prefix] = append(
				prefixToAllStreamPaths[prefix], string(v))
			env.Clock.Add(time.Second)
		}
		ds.GetTestable(c).CatchupIndexes()

		// Invert streamPaths since we will return results in descending Created
		// order.
		invert := func(s []string) {
			for i := 0; i < len(s)/2; i++ {
				eidx := len(s) - i - 1
				s[i], s[eidx] = s[eidx], s[i]
			}
		}
		for _, paths := range prefixToStreamPaths {
			invert(paths)
		}
		for _, paths := range prefixToAllStreamPaths {
			invert(paths)
		}

		Convey(`An empty query will return an error.`, func() {
			_, err := svr.Query(c, &req)
			So(err, ShouldBeRPCInvalidArgument, "invalid query `path`")
		})

		Convey(`Handles non-existent project.`, func() {
			req.Project = "does-not-exist"
			req.Path = "testing/+/**"

			Convey(`Anon`, func() {
				_, err := svr.Query(c, &req)
				So(err, ShouldBeRPCUnauthenticated)
			})

			Convey(`User`, func() {
				env.LogIn()
				_, err := svr.Query(c, &req)
				So(err, ShouldBeRPCPermissionDenied)
			})
		})

		Convey(`Authorization rules`, func() {
			// "proj-exclusive" has restricted legacy ACLs, see coordinatorTest.
			const project = "proj-exclusive"
			req.Project = "proj-exclusive"
			req.Path = "exclusive/+/**"

			const (
				User   = "user:caller@example.com"
				Anon   = identity.AnonymousIdentity
				Legacy = "user:legacy@example.com" // present in @legacy realm ACL
			)

			const (
				NoRealm        = ""
				AllowedRealm   = "allowed"
				ForbiddenRealm = "forbidden"
			)

			const (
				AllowLegacy  = true
				ForbidLegacy = false
			)

			realm := func(short string) string {
				return realms.Join(project, short)
			}

			cases := []struct {
				enforce     bool              // is enforce_realms_in ON or OFF
				ident       identity.Identity // who's making the call
				realm       string            // the realm to put the stream in
				allowLegacy bool              // mock legacy ACLs to allow access
				code        codes.Code        // the expected gRPC code
			}{
				// Enforcement is on, legacy ACLs are ignored.
				{true, User, NoRealm, AllowLegacy, codes.PermissionDenied},
				{true, User, NoRealm, ForbidLegacy, codes.PermissionDenied},
				{true, Legacy, NoRealm, AllowLegacy, codes.OK},
				{true, Legacy, NoRealm, ForbidLegacy, codes.OK},
				{true, User, AllowedRealm, AllowLegacy, codes.OK},
				{true, User, AllowedRealm, ForbidLegacy, codes.OK},
				{true, User, ForbiddenRealm, AllowLegacy, codes.PermissionDenied},
				{true, User, ForbiddenRealm, ForbidLegacy, codes.PermissionDenied},

				// Enforcement is off, have a fallback on legacy ACLs.
				{false, User, NoRealm, AllowLegacy, codes.OK},
				{false, User, NoRealm, ForbidLegacy, codes.PermissionDenied},
				{false, Legacy, NoRealm, AllowLegacy, codes.OK},
				{false, Legacy, NoRealm, ForbidLegacy, codes.OK},
				{false, User, AllowedRealm, AllowLegacy, codes.OK},
				{false, User, AllowedRealm, ForbidLegacy, codes.OK},
				{false, User, ForbiddenRealm, AllowLegacy, codes.OK},
				{false, User, ForbiddenRealm, ForbidLegacy, codes.PermissionDenied},

				// Permission denied for anon => Unauthenticated.
				{true, Anon, ForbiddenRealm, AllowLegacy, codes.Unauthenticated},
				{false, Anon, NoRealm, ForbidLegacy, codes.Unauthenticated},
			}

			for i, test := range cases {
				Convey(fmt.Sprintf("Case #%d", i), func() {
					tls := ct.MakeStream(c, project, test.realm, "exclusive/+/foo")
					tls.Put(c)

					mocks := []authtest.MockedDatum{
						authtest.MockPermission(test.ident, realm(AllowedRealm), coordinator.PermLogsList),
						authtest.MockPermission(Legacy, realm(realms.LegacyRealm), coordinator.PermLogsList),
					}
					if test.enforce {
						mocks = append(mocks, authtest.MockRealmData(
							realm(realms.RootRealm),
							&protocol.RealmData{EnforceInService: []string{env.ServiceID}},
						))
					}
					if test.allowLegacy {
						mocks = append(mocks, authtest.MockMembership(test.ident, "auth"))
					}

					c := auth.WithState(c, &authtest.FakeState{
						Identity: test.ident,
						FakeDB:   authtest.NewFakeDB(mocks...),
					})

					resp, err := svr.Query(c, &req)
					So(status.Code(err), ShouldEqual, test.code)
					if err == nil {
						So(resp.Project, ShouldEqual, project)
						So(resp.Realm, ShouldEqual, test.realm)
					}
				})
			}
		})

		Convey(`An empty query will include purged streams if admin.`, func() {
			env.JoinGroup("admin")

			req.Path = "meta/+/**"
			resp, err := svr.Query(c, &req)
			So(err, ShouldBeRPCOK)
			So(resp, shouldHaveLogPaths, prefixToAllStreamPaths["meta"])
		})

		Convey(`A query with an invalid path will return BadRequest error.`, func() {
			req.Path = "***"

			_, err := svr.Query(c, &req)
			So(err, ShouldBeRPCInvalidArgument, "invalid query `path`")
		})

		Convey(`A query with an invalid Next cursor will return BadRequest error.`, func() {
			req.Next = "invalid"
			req.Path = "testing/+/**"
			fb.BreakFeatures(errors.New("testing error"), "DecodeCursor")

			_, err := svr.Query(c, &req)
			So(err, ShouldBeRPCInvalidArgument, "invalid `next` value")
		})

		Convey(`A datastore query error will return InternalServer error.`, func() {
			req.Path = "testing/+/**"
			fb.BreakFeatures(errors.New("testing error"), "Run")

			_, err := svr.Query(c, &req)
			So(err, ShouldBeRPCInternal)
		})

		Convey(`When querying for "testing/+/baz"`, func() {
			req.Path = "testing/+/baz"

			tls := streams["testing/+/baz"]
			Convey(`State is not returned.`, func() {
				resp, err := svr.Query(c, &req)
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
					resp, err := svr.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths, "testing/+/baz")

					So(resp.Streams, ShouldHaveLength, 1)
					So(resp.Streams[0].State, ShouldResemble, buildLogStreamState(tls.Stream, tls.State))
					So(resp.Streams[0].Desc, ShouldResembleProto, tls.Desc)
					So(resp.Streams[0].DescProto, ShouldBeNil)
				})

				Convey(`When not requesting protobufs, and with a corrupt descriptor, returns InternalServer error.`, func() {
					tls.Stream.SetDSValidate(false)
					tls.Stream.Descriptor = []byte{0x00} // Invalid protobuf, zero tag.
					if err := tls.Put(c); err != nil {
						panic(err)
					}
					ds.GetTestable(c).CatchupIndexes()

					_, err := svr.Query(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`When requesting protobufs, returns the raw protobuf descriptor.`, func() {
					req.Proto = true

					resp, err := svr.Query(c, &req)
					So(err, ShouldBeNil)
					So(resp, shouldHaveLogPaths, "testing/+/baz")

					So(resp.Streams, ShouldHaveLength, 1)
					So(resp.Streams[0].State, ShouldResemble, buildLogStreamState(tls.Stream, tls.State))
					So(resp.Streams[0].Desc, ShouldResembleProto, tls.Desc)
				})
			})
		})

		Convey(`With a query limit of 3`, func() {
			svrBase.resultLimit = 3

			Convey(`Can iteratively query to retrieve all stream paths.`, func() {
				var seen []string

				req.Path = "testing/+/**"
				streamPaths := append([]string(nil), prefixToStreamPaths["testing"]...)

				next := ""
				for {
					req.Next = next

					resp, err := svr.Query(c, &req)
					So(err, ShouldBeRPCOK)

					for _, svr := range resp.Streams {
						seen = append(seen, svr.Path)
					}

					next = resp.Next
					if next == "" {
						break
					}
				}

				sort.Strings(seen)
				sort.Strings(streamPaths)
				So(seen, ShouldResemble, streamPaths)
			})
		})

		Convey(`When querying against timestamp constraints`, func() {
			req.Older = timestamppb.New(env.Clock.Now())

			Convey(`Querying for entries created at or after 2 seconds ago (latest 2 entries).`, func() {
				req.Path = "testing/+/**"
				req.Newer = timestamppb.New(env.Clock.Now().Add(-2*time.Second - time.Millisecond))

				resp, err := svr.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "testing/+/baz", "testing/+/foo/bar/baz")
			})

			Convey(`With a query limit of 2`, func() {
				svrBase.resultLimit = 2

				Convey(`A query request will return the newest 2 entries and have a Next cursor for the next 2.`, func() {
					req.Path = "testing/+/**"
					resp, err := svr.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths, "testing/+/baz", "testing/+/foo/bar/baz")
					So(resp.Next, ShouldNotEqual, "")

					// Iterate.
					req.Next = resp.Next

					resp, err = svr.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths, "testing/+/foo/bar", "testing/+/foo")
				})

				Convey(`A datastore query error will return InternalServer error.`, func() {
					req.Path = "testing/+/**"
					fb.BreakFeatures(errors.New("testing error"), "Run")

					_, err := svr.Query(c, &req)
					So(err, ShouldBeRPCInternal)
				})
			})
		})

		Convey(`When querying for meta streams`, func() {
			req.Path = "meta/+/**"

			Convey(`When purged=yes, returns BadRequest error.`, func() {
				req.Purged = logdog.QueryRequest_YES

				_, err := svr.Query(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "non-admin user cannot request purged log streams")
			})

			Convey(`When the user is an administrator`, func() {
				env.JoinGroup("admin")

				Convey(`When purged=yes, returns [terminated/archived/purged, purged]`, func() {
					req.Purged = logdog.QueryRequest_YES

					resp, err := svr.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths,
						"meta/+/terminated/archived/purged/foo",
						"meta/+/purged/foo",
					)
				})

				Convey(`When purged=no, returns [binary, datagram, archived, terminated]`, func() {
					req.Purged = logdog.QueryRequest_NO

					resp, err := svr.Query(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, shouldHaveLogPaths,
						"meta/+/binary/foo",
						"meta/+/datagram/foo",
						"meta/+/archived/foo",
						"meta/+/terminated/foo",
					)
				})
			})

			Convey(`When querying for text streams, returns [archived, terminated]`, func() {
				req.StreamType = &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_TEXT}

				resp, err := svr.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/+/archived/foo", "meta/+/terminated/foo")
			})

			Convey(`When querying for binary streams, returns [binary]`, func() {
				req.StreamType = &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_BINARY}

				resp, err := svr.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/+/binary/foo")
			})

			Convey(`When querying for datagram streams, returns [datagram]`, func() {
				req.StreamType = &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_DATAGRAM}

				resp, err := svr.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "meta/+/datagram/foo")
			})

			Convey(`When querying for an invalid stream type, returns a BadRequest error.`, func() {
				req.StreamType = &logdog.QueryRequest_StreamTypeFilter{Value: -1}

				_, err := svr.Query(c, &req)
				So(err, ShouldBeRPCInvalidArgument)
			})
		})

		Convey(`When querying for content type "other", returns [other/+/baz, other/+/foo/bar].`, func() {
			req.Path = "other/+/**"
			req.ContentType = "other"

			resp, err := svr.Query(c, &req)
			So(err, ShouldBeRPCOK)
			So(resp, shouldHaveLogPaths, "other/+/baz", "other/+/foo/bar")
		})

		Convey(`When querying for tags`, func() {
			Convey(`Tag "baz", returns [testing/+/baz, testing/+/foo/bar/baz]`, func() {
				req.Path = "testing/+/**"
				req.Tags["baz"] = ""

				resp, err := svr.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "testing/+/baz", "testing/+/foo/bar/baz")
			})

			Convey(`Tags "prefix=testing", "baz", returns [testing/+/baz, testing/+/foo/bar/baz]`, func() {
				req.Path = "testing/+/**"
				req.Tags["baz"] = ""
				req.Tags["prefix"] = "testing"

				resp, err := svr.Query(c, &req)
				So(err, ShouldBeRPCOK)
				So(resp, shouldHaveLogPaths, "testing/+/baz", "testing/+/foo/bar/baz")
			})

			Convey(`When an invalid tag is specified, returns BadRequest error`, func() {
				req.Path = "testing/+/**"
				req.Tags["+++not a valid tag+++"] = ""

				_, err := svr.Query(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "invalid tag constraint")
			})
		})
	})
}
