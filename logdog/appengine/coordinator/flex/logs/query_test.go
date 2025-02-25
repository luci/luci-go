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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/logdog/common/types"
)

func shouldHaveLogPaths(expected ...string) comparison.Func[*logdog.QueryResponse] {
	return func(actual *logdog.QueryResponse) *failure.Summary {
		paths := stringset.New(len(expected))
		if len(actual.Streams) > 0 {
			for _, s := range actual.Streams {
				paths.Add(s.Path)
			}
		}

		exp := stringset.NewFromSlice(expected...)

		diff := paths.Difference(exp)
		if diff.Len() == 0 {
			return nil
		}

		return comparison.NewSummaryBuilder("shouldHaveLogPaths").
			Actual(paths.ToSortedSlice()).
			Expected(exp.ToSortedSlice()).
			AddFindingf("Missing", "%#v", exp.ToSortedSlice()).
			Because("Missing log paths").
			Summary
	}
}

func TestQuery(t *testing.T) {
	t.Parallel()

	ftt.Run(`With a testing configuration, a Query request`, t, func(t *ftt.Test) {
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
						assert.Loosely(t, tls.State.ArchivalState().Archived(), should.BeTrue)
						fallthrough // Archived streams are also terminated.

					case "terminated":
						tls.State.TerminalIndex = 1337
						tls.State.TerminatedTime = now
						assert.Loosely(t, tls.State.Terminated(), should.BeTrue)

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

		t.Run(`An empty query will return an error.`, func(t *ftt.Test) {
			req.Path = ""
			_, err := svr.Query(c, &req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid query `path`"))
		})

		t.Run(`Handles non-existent project.`, func(t *ftt.Test) {
			req.Project = "does-not-exist"

			t.Run(`Anon`, func(t *ftt.Test) {
				env.ActAsAnon()
				_, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Unauthenticated))
			})

			t.Run(`User`, func(t *ftt.Test) {
				env.ActAsNobody()
				_, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})
		})

		t.Run(`Authorization rules`, func(t *ftt.Test) {
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
				t.Run(fmt.Sprintf("Case #%d", i), func(t *ftt.Test) {
					tls := ct.MakeStream(c, project, test.realm, "prefix/+/foo")
					assert.Loosely(t, tls.Put(c), should.BeNil)

					// Note: this overrides mocks set by ActAsReader.
					c := auth.WithState(c, &authtest.FakeState{
						Identity: test.ident,
						FakeDB:   authDB,
					})

					resp, err := svr.Query(c, &req)
					assert.Loosely(t, status.Code(err), should.Equal(test.code))
					if err == nil {
						assert.Loosely(t, resp.Project, should.Equal(project))
						if test.realm != "" {
							assert.Loosely(t, resp.Realm, should.Equal(test.realm))
						} else {
							assert.Loosely(t, resp.Realm, should.Equal(realms.LegacyRealm))
						}
					}
				})
			}
		})

		t.Run(`An empty query will include purged streams if admin.`, func(t *ftt.Test) {
			env.JoinAdmins()

			req.Path = "meta/+/**"
			resp, err := svr.Query(c, &req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
			assert.Loosely(t, resp, shouldHaveLogPaths(prefixToAllStreamPaths["meta"]...))
		})

		t.Run(`A query with an invalid path will return BadRequest error.`, func(t *ftt.Test) {
			req.Path = "***"

			_, err := svr.Query(c, &req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid query `path`"))
		})

		t.Run(`A query with an invalid Next cursor will return BadRequest error.`, func(t *ftt.Test) {
			req.Next = "invalid"
			req.Path = "testing/+/**"
			fb.BreakFeatures(errors.New("testing error"), "DecodeCursor")

			_, err := svr.Query(c, &req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invalid `next` value"))
		})

		t.Run(`A datastore query error will return InternalServer error.`, func(t *ftt.Test) {
			req.Path = "testing/+/**"
			fb.BreakFeatures(errors.New("testing error"), "Run")

			_, err := svr.Query(c, &req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
		})

		t.Run(`When querying for "testing/+/baz"`, func(t *ftt.Test) {
			req.Path = "testing/+/baz"

			tls := streams["testing/+/baz"]
			t.Run(`State is not returned.`, func(t *ftt.Test) {
				resp, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
				assert.Loosely(t, resp, shouldHaveLogPaths("testing/+/baz"))

				assert.Loosely(t, resp.Streams, should.HaveLength(1))
				assert.Loosely(t, resp.Streams[0].State, should.BeNil)
				assert.Loosely(t, resp.Streams[0].Desc, should.BeNil)
				assert.Loosely(t, resp.Streams[0].DescProto, should.BeNil)
			})

			t.Run(`When requesting state`, func(t *ftt.Test) {
				req.State = true

				t.Run(`When not requesting protobufs, returns a descriptor structure.`, func(t *ftt.Test) {
					resp, err := svr.Query(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogPaths("testing/+/baz"))

					assert.Loosely(t, resp.Streams, should.HaveLength(1))
					assert.Loosely(t, resp.Streams[0].State, should.Match(buildLogStreamState(tls.Stream, tls.State)))
					assert.Loosely(t, resp.Streams[0].Desc, should.Match(tls.Desc))
					assert.Loosely(t, resp.Streams[0].DescProto, should.BeNil)
				})

				t.Run(`When not requesting protobufs, and with a corrupt descriptor, returns InternalServer error.`, func(t *ftt.Test) {
					tls.Stream.SetDSValidate(false)
					tls.Stream.Descriptor = []byte{0x00} // Invalid protobuf, zero tag.
					if err := tls.Put(c); err != nil {
						panic(err)
					}
					ds.GetTestable(c).CatchupIndexes()

					_, err := svr.Query(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				})

				t.Run(`When requesting protobufs, returns the raw protobuf descriptor.`, func(t *ftt.Test) {
					req.Proto = true

					resp, err := svr.Query(c, &req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, resp, shouldHaveLogPaths("testing/+/baz"))

					assert.Loosely(t, resp.Streams, should.HaveLength(1))
					assert.Loosely(t, resp.Streams[0].State, should.Match(buildLogStreamState(tls.Stream, tls.State)))
					assert.Loosely(t, resp.Streams[0].Desc, should.Match(tls.Desc))
				})
			})
		})

		t.Run(`With a query limit of 3`, func(t *ftt.Test) {
			svrBase.resultLimit = 3

			t.Run(`Can iteratively query to retrieve all stream paths.`, func(t *ftt.Test) {
				var seen []string

				req.Path = "testing/+/**"
				streamPaths := append([]string(nil), prefixToStreamPaths["testing"]...)

				next := ""
				for {
					req.Next = next

					resp, err := svr.Query(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))

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
				assert.Loosely(t, seen, should.Match(streamPaths))
			})
		})

		t.Run(`When querying for meta streams`, func(t *ftt.Test) {
			req.Path = "meta/+/**"

			t.Run(`When purged=yes, returns BadRequest error.`, func(t *ftt.Test) {
				req.Purged = logdog.QueryRequest_YES

				_, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("non-admin user cannot request purged log streams"))
			})

			t.Run(`When the user is an administrator`, func(t *ftt.Test) {
				env.JoinAdmins()

				t.Run(`When purged=yes, returns [terminated/archived/purged, purged]`, func(t *ftt.Test) {
					req.Purged = logdog.QueryRequest_YES

					resp, err := svr.Query(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogPaths(
						"meta/+/terminated/archived/purged/foo",
						"meta/+/purged/foo",
					))
				})

				t.Run(`When purged=no, returns [binary, datagram, archived, terminated]`, func(t *ftt.Test) {
					req.Purged = logdog.QueryRequest_NO

					resp, err := svr.Query(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogPaths(
						"meta/+/binary/foo",
						"meta/+/datagram/foo",
						"meta/+/archived/foo",
						"meta/+/terminated/foo",
					))
				})
			})

			t.Run(`When querying for text streams, returns [archived, terminated]`, func(t *ftt.Test) {
				req.StreamType = &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_TEXT}

				resp, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
				assert.Loosely(t, resp, shouldHaveLogPaths("meta/+/archived/foo", "meta/+/terminated/foo"))
			})

			t.Run(`When querying for binary streams, returns [binary]`, func(t *ftt.Test) {
				req.StreamType = &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_BINARY}

				resp, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
				assert.Loosely(t, resp, shouldHaveLogPaths("meta/+/binary/foo"))
			})

			t.Run(`When querying for datagram streams, returns [datagram]`, func(t *ftt.Test) {
				req.StreamType = &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_DATAGRAM}

				resp, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
				assert.Loosely(t, resp, shouldHaveLogPaths("meta/+/datagram/foo"))
			})

			t.Run(`When querying for an invalid stream type, returns a BadRequest error.`, func(t *ftt.Test) {
				req.StreamType = &logdog.QueryRequest_StreamTypeFilter{Value: -1}

				_, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})
		})

		t.Run(`When querying for content type "other", returns [other/+/baz, other/+/foo/bar].`, func(t *ftt.Test) {
			req.Path = "other/+/**"
			req.ContentType = "other"

			resp, err := svr.Query(c, &req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
			assert.Loosely(t, resp, shouldHaveLogPaths("other/+/baz", "other/+/foo/bar"))
		})

		t.Run(`When querying for tags`, func(t *ftt.Test) {
			t.Run(`Tag "baz", returns [testing/+/baz, testing/+/foo/bar/baz]`, func(t *ftt.Test) {
				req.Path = "testing/+/**"
				req.Tags["baz"] = ""

				resp, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
				assert.Loosely(t, resp, shouldHaveLogPaths("testing/+/baz", "testing/+/foo/bar/baz"))
			})

			t.Run(`Tags "prefix=testing", "baz", returns [testing/+/baz, testing/+/foo/bar/baz]`, func(t *ftt.Test) {
				req.Path = "testing/+/**"
				req.Tags["baz"] = ""
				req.Tags["prefix"] = "testing"

				resp, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
				assert.Loosely(t, resp, shouldHaveLogPaths("testing/+/baz", "testing/+/foo/bar/baz"))
			})

			t.Run(`When an invalid tag is specified, returns BadRequest error`, func(t *ftt.Test) {
				req.Path = "testing/+/**"
				req.Tags["+++not a valid tag+++"] = ""

				_, err := svr.Query(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("invalid tag constraint"))
			})
		})
	})
}
