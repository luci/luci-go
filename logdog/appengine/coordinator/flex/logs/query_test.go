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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
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

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func shouldHaveLogPaths(actual any, expected ...any) string {
	resp := actual.(*logdog.QueryResponse)
	paths := stringset.New(len(expected))
	if len(resp.Streams) > 0 {
		for _, s := range resp.Streams {
			paths.Add(s.Path)
		}
	}

	exp := stringset.New(len(expected))
	for _, e := range expected {
		switch t := e.(type) {
		case []string:
			exp.AddAll(t)

		case string:
			exp.Add(t)

		case types.StreamPath:
			exp.Add(string(t))

		default:
			panic(fmt.Errorf("unsupported expected type %T: %v", t, t))
		}
	}

	return ShouldBeEmpty(paths.Difference(exp))
}

func TestQuery(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration, a Query request`, t, func() {
		c, env := ct.Install()
		c, fb := featureBreaker.FilterRDS(c, nil)

		svr := New()
		svrBase := svr.(*logdog.DecoratedLogs).Service.(*server)

		const project = "some-project"
		const realm = "some-realm"

		env.AddProject(c, project)
		env.ActAsReader(project, realm)

		// Stock query request, will be modified by each test.
		req := logdog.QueryRequest{
			Project: project,
			Path:    "prefix/+/**",
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
			tls := ct.MakeStream(c, project, realm, v)
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
			req.Path = ""
			_, err := svr.Query(c, &req)
			So(err, ShouldBeRPCInvalidArgument, "invalid query `path`")
		})

		Convey(`Handles non-existent project.`, func() {
			req.Project = "does-not-exist"

			Convey(`Anon`, func() {
				env.ActAsAnon()
				_, err := svr.Query(c, &req)
				So(err, ShouldBeRPCUnauthenticated)
			})

			Convey(`User`, func() {
				env.ActAsNobody()
				_, err := svr.Query(c, &req)
				So(err, ShouldBeRPCPermissionDenied)
			})
		})

		Convey(`Authorization rules`, func() {
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

			realm := func(short string) string {
				return realms.Join(project, short)
			}
			authDB := authtest.NewFakeDB(
				authtest.MockPermission(User, realm(AllowedRealm), coordinator.PermLogsList),
				authtest.MockPermission(Legacy, realm(realms.LegacyRealm), coordinator.PermLogsList),
			)

			cases := []struct {
				ident identity.Identity // who's making the call
				realm string            // the realm to put the stream in
				code  codes.Code        // the expected gRPC code
			}{
				{User, NoRealm, codes.PermissionDenied},
				{User, AllowedRealm, codes.OK},
				{User, ForbiddenRealm, codes.PermissionDenied},
				{Legacy, NoRealm, codes.OK},

				// Permission denied for anon => Unauthenticated.
				{Anon, ForbiddenRealm, codes.Unauthenticated},
				{Anon, NoRealm, codes.Unauthenticated},
			}

			for i, test := range cases {
				Convey(fmt.Sprintf("Case #%d", i), func() {
					tls := ct.MakeStream(c, project, test.realm, "prefix/+/foo")
					So(tls.Put(c), ShouldBeNil)

					// Note: this overrides mocks set by ActAsReader.
					c := auth.WithState(c, &authtest.FakeState{
						Identity: test.ident,
						FakeDB:   authDB,
					})

					resp, err := svr.Query(c, &req)
					So(status.Code(err), ShouldEqual, test.code)
					if err == nil {
						So(resp.Project, ShouldEqual, project)
						if test.realm != "" {
							So(resp.Realm, ShouldEqual, test.realm)
						} else {
							So(resp.Realm, ShouldEqual, realms.LegacyRealm)
						}
					}
				})
			}
		})

		Convey(`An empty query will include purged streams if admin.`, func() {
			env.JoinAdmins()

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

		Convey(`When querying for meta streams`, func() {
			req.Path = "meta/+/**"

			Convey(`When purged=yes, returns BadRequest error.`, func() {
				req.Purged = logdog.QueryRequest_YES

				_, err := svr.Query(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "non-admin user cannot request purged log streams")
			})

			Convey(`When the user is an administrator`, func() {
				env.JoinAdmins()

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
