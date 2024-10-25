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
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/logdog/common/archive"
	"go.chromium.org/luci/logdog/common/renderer"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/gae/filter/featureBreaker"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
)

func shouldHaveLogs(expected ...int) comparison.Func[*logdog.GetResponse] {
	return func(actual *logdog.GetResponse) *failure.Summary {
		respLogs := make([]int, len(actual.Logs))
		for i, le := range actual.Logs {
			respLogs[i] = int(le.StreamIndex)
		}

		ret := should.Match(expected, cmpopts.EquateEmpty())(respLogs)
		if ret != nil {
			ret.Comparison.Name = "shouldHaveLogs"
		}
		return ret
	}
}

// zeroRecords reads a recordio stream and clears all of the record data,
// preserving size data.
func zeroRecords(d []byte) {
	r := bytes.NewReader(d)
	cr := iotools.CountingReader{Reader: r}
	rio := recordio.NewReader(&cr, 4096)
	trash := bytes.Buffer{}

	for {
		s, r, err := rio.ReadFrame()
		if err != nil {
			break
		}

		pos := int(cr.Count)
		for i := int64(0); i < s; i++ {
			d[pos+int(i)] = 0x00
		}

		// Read the (now-zeroed) data.
		trash.Reset()
		trash.ReadFrom(r)
	}
}

func testGetImpl(t *testing.T, archived bool) {
	ftt.Run(fmt.Sprintf(`With a testing configuration, a Get request (archived=%v)`, archived), t, func(t *ftt.Test) {
		c, env := ct.Install()

		svr := New()

		const project = "some-project"
		const realm = "some-realm"

		env.AddProject(c, project)
		env.ActAsReader(project, realm)

		// Generate our test stream.
		tls := ct.MakeStream(c, project, realm, "testing/+/foo/bar")

		putLogStream := func(c context.Context) {
			if err := tls.Put(c); err != nil {
				panic(err)
			}
		}
		putLogStream(c)

		env.Clock.Add(time.Second)
		var entries []*logpb.LogEntry
		protobufs := map[uint64][]byte{}
		for _, v := range []int{0, 1, 2, 4, 5, 7} {
			le := tls.LogEntry(c, v)
			le.GetText().Lines = append(le.GetText().Lines, &logpb.Text_Line{
				Value: []byte("another line of text"),
			})
			entries = append(entries, le)

			switch v {
			case 4:
				le.Content = &logpb.LogEntry_Binary{
					Binary: &logpb.Binary{
						Data: []byte{0x00, 0x01, 0x02, 0x03},
					},
				}

			case 5:
				le.Content = &logpb.LogEntry_Datagram{
					Datagram: &logpb.Datagram{
						Data: []byte{0x00, 0x01, 0x02, 0x03},
						Partial: &logpb.Datagram_Partial{
							Index: 2,
							Size:  1024,
							Last:  false,
						},
					},
				}
			}

			d, err := proto.Marshal(le)
			if err != nil {
				panic(err)
			}
			protobufs[uint64(v)] = d
		}

		// frameSize returns the full RecordIO frame size for the named log protobuf
		// indices.
		frameSize := func(indices ...uint64) int32 {
			var size int
			for _, idx := range indices {
				pb := protobufs[idx]
				size += recordio.FrameHeaderSize(int64(len(pb))) + len(pb)
			}
			if size > math.MaxInt32 {
				panic(size)
			}
			return int32(size)
		}

		t.Run(`Testing Get requests (no logs)`, func(t *ftt.Test) {
			req := logdog.GetRequest{
				Project: project,
				Path:    string(tls.Path),
			}

			t.Run(`Will fail if the Path is not a stream path or a hash.`, func(t *ftt.Test) {
				req.Path = "not/a/full/stream/path"
				_, err := svr.Get(c, &req)
				assert.Loosely(t, err, should.ErrLike("invalid path value"))
			})

			t.Run(`Will fail with Internal if the datastore Get() doesn't work.`, func(t *ftt.Test) {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("testing error"), "GetMulti")

				_, err := svr.Get(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
			})

			t.Run(`Will fail with InvalidArgument if the project name is invalid.`, func(t *ftt.Test) {
				req.Project = "!!! invalid project name !!!"
				_, err := svr.Get(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run(`Will fail with NotFound if the log path does not exist (different path).`, func(t *ftt.Test) {
				req.Path = "testing/+/does/not/exist"
				_, err := svr.Get(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})

			t.Run(`Will handle non-existent project.`, func(t *ftt.Test) {
				req.Project = "does-not-exist"
				req.Path = "testing/+/**"

				t.Run(`Anon`, func(t *ftt.Test) {
					env.ActAsAnon()
					_, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				})

				t.Run(`User`, func(t *ftt.Test) {
					env.ActAsNobody()
					_, err := svr.Get(c, &req)
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
					authtest.MockPermission(User, realm(AllowedRealm), coordinator.PermLogsGet),
					authtest.MockPermission(Legacy, realm(realms.LegacyRealm), coordinator.PermLogsGet),
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
						tls = ct.MakeStream(c, project, test.realm, types.StreamPath(req.Path))
						putLogStream(c)

						// Note: this overrides mocks set by ActAsReader.
						c := auth.WithState(c, &authtest.FakeState{
							Identity: test.ident,
							FakeDB:   authDB,
						})

						resp, err := svr.Get(c, &req)
						assert.Loosely(t, status.Code(err), should.Equal(test.code))
						if err == nil {
							assert.Loosely(t, resp, shouldHaveLogs())
							assert.Loosely(t, resp.Project, should.Equal(project))
							assert.Loosely(t, resp.Realm, should.Equal(test.realm))
						}
					})
				}
			})
		})

		t.Run(`When testing log data is added`, func(t *ftt.Test) {
			putLogData := func() {
				if !archived {
					// Add the logs to the in-memory temporary storage.
					for _, le := range entries {
						err := env.BigTable.Put(c, storage.PutRequest{
							Project: project,
							Path:    tls.Path,
							Index:   types.MessageIndex(le.StreamIndex),
							Values:  [][]byte{protobufs[le.StreamIndex]},
						})
						if err != nil {
							panic(fmt.Errorf("failed to Put() LogEntry: %v", err))
						}
					}
				} else {
					// Archive this log stream. We will generate one index entry for every
					// 2 log entries.
					src := renderer.StaticSource(entries)
					var lbuf, ibuf bytes.Buffer
					m := archive.Manifest{
						Desc:             tls.Desc,
						Source:           &src,
						LogWriter:        &lbuf,
						IndexWriter:      &ibuf,
						StreamIndexRange: 2,
					}
					if err := archive.Archive(m); err != nil {
						panic(err)
					}

					now := env.Clock.Now().UTC()

					env.GSClient.Put("gs://testbucket/stream", lbuf.Bytes())
					env.GSClient.Put("gs://testbucket/index", ibuf.Bytes())
					tls.State.TerminatedTime = now
					tls.State.ArchivedTime = now
					tls.State.ArchiveStreamURL = "gs://testbucket/stream"
					tls.State.ArchiveIndexURL = "gs://testbucket/index"

					assert.Loosely(t, tls.State.ArchivalState().Archived(), should.BeTrue)
				}
			}
			putLogData()
			putLogStream(c)

			t.Run(`Testing Get requests`, func(t *ftt.Test) {
				req := logdog.GetRequest{
					Project: string(project),
					Path:    string(tls.Path),
				}

				t.Run(`When the log stream is purged`, func(t *ftt.Test) {
					tls.Stream.Purged = true
					putLogStream(c)

					t.Run(`Will return NotFound if the user is not an administrator.`, func(t *ftt.Test) {
						_, err := svr.Get(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
					})

					t.Run(`Will process the request if the user is an administrator.`, func(t *ftt.Test) {
						env.JoinAdmins()

						resp, err := svr.Get(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
						assert.Loosely(t, resp, shouldHaveLogs(0, 1, 2))
					})
				})

				t.Run(`Will return empty if no records were requested.`, func(t *ftt.Test) {
					req.LogCount = -1
					req.State = false

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp.Logs, should.HaveLength(0))
				})

				t.Run(`Will successfully retrieve a stream path.`, func(t *ftt.Test) {
					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0, 1, 2))

					t.Run(`Will successfully retrieve the stream path again (caching).`, func(t *ftt.Test) {
						resp, err := svr.Get(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
						assert.Loosely(t, resp, shouldHaveLogs(0, 1, 2))
					})
				})

				t.Run(`Will successfully retrieve a stream path offset at 4.`, func(t *ftt.Test) {
					req.Index = 4

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(4, 5))
				})

				t.Run(`Will retrieve no logs for contiguous offset 6.`, func(t *ftt.Test) {
					req.Index = 6

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, len(resp.Logs), should.BeZero)
				})

				t.Run(`Will retrieve log 7 for non-contiguous offset 6.`, func(t *ftt.Test) {
					req.NonContiguous = true
					req.Index = 6

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(7))
				})

				t.Run(`With a byte limit of 1, will still return at least one log entry.`, func(t *ftt.Test) {
					req.ByteCount = 1

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0))
				})

				t.Run(`With a byte limit of sizeof(0), will return log entry 0.`, func(t *ftt.Test) {
					req.ByteCount = frameSize(0)

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0))
				})

				t.Run(`With a byte limit of sizeof(0)+1, will return log entry 0.`, func(t *ftt.Test) {
					req.ByteCount = frameSize(0) + 1

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0))
				})

				t.Run(`With a byte limit of sizeof({0, 1}), will return log entries {0, 1}.`, func(t *ftt.Test) {
					req.ByteCount = frameSize(0, 1)

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0, 1))
				})

				t.Run(`With a byte limit of sizeof({0, 1, 2}), will return log entries {0, 1, 2}.`, func(t *ftt.Test) {
					req.ByteCount = frameSize(0, 1, 2)

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0, 1, 2))
				})

				t.Run(`With a byte limit of sizeof({0, 1, 2})+1, will return log entries {0, 1, 2}.`, func(t *ftt.Test) {
					req.ByteCount = frameSize(0, 1, 2) + 1

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0, 1, 2))
				})

				t.Run(`When requesting state`, func(t *ftt.Test) {
					req.State = true
					req.LogCount = -1

					t.Run(`Will successfully retrieve stream state.`, func(t *ftt.Test) {
						resp, err := svr.Get(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
						assert.Loosely(t, resp.State, should.Resemble(buildLogStreamState(tls.Stream, tls.State)))
						assert.Loosely(t, len(resp.Logs), should.BeZero)
					})

					t.Run(`Will return Internal if the protobuf descriptor data is corrupt.`, func(t *ftt.Test) {
						tls.Stream.SetDSValidate(false)
						tls.Stream.Descriptor = []byte{0x00} // Invalid protobuf, zero tag.
						putLogStream(c)

						_, err := svr.Get(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
					})
				})

				t.Run(`When requesting a signed URL`, func(t *ftt.Test) {
					const duration = 10 * time.Hour
					req.LogCount = -1

					sr := logdog.GetRequest_SignURLRequest{
						Lifetime: durationpb.New(duration),
						Stream:   true,
						Index:    true,
					}
					req.GetSignedUrls = &sr

					if archived {
						t.Run(`Will successfully retrieve the URL.`, func(t *ftt.Test) {
							resp, err := svr.Get(c, &req)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, resp.Logs, should.HaveLength(0))

							assert.Loosely(t, resp.SignedUrls, should.NotBeNil)
							assert.Loosely(t, resp.SignedUrls.Stream, should.HaveSuffix("&signed=true"))
							assert.Loosely(t, resp.SignedUrls.Index, should.HaveSuffix("&signed=true"))
							assert.Loosely(t, resp.SignedUrls.Expiration.AsTime(), should.Resemble(clock.Now(c).Add(duration)))
						})
					} else {
						t.Run(`Will succeed, but return no URL.`, func(t *ftt.Test) {
							resp, err := svr.Get(c, &req)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, resp.Logs, should.HaveLength(0))
							assert.Loosely(t, resp.SignedUrls, should.BeNil)
						})
					}
				})

				t.Run(`Will return Internal if the protobuf log entry data is corrupt.`, func(t *ftt.Test) {
					if archived {
						// Corrupt the archive datastream.
						stream := env.GSClient.Get("gs://testbucket/stream")
						zeroRecords(stream)
					} else {
						// Add corrupted entry to Storage. Create a new entry here, since
						// the storage will reject a duplicate/overwrite.
						err := env.BigTable.Put(c, storage.PutRequest{
							Project: project,
							Path:    types.StreamPath(req.Path),
							Index:   666,
							Values:  [][]byte{{0x00}}, // Invalid protobuf, zero tag.
						})
						if err != nil {
							panic(err)
						}
						req.Index = 666
					}

					_, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				})

				t.Run(`Will successfully retrieve both logs and stream state.`, func(t *ftt.Test) {
					req.State = true

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp.State, should.Resemble(buildLogStreamState(tls.Stream, tls.State)))
					assert.Loosely(t, resp, shouldHaveLogs(0, 1, 2))
				})

				t.Run(`Will return Internal if the Storage is not working.`, func(t *ftt.Test) {
					if archived {
						env.GSClient["error"] = []byte("test error")
					} else {
						env.BigTable.SetErr(errors.New("not working"))
					}

					_, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				})

				t.Run(`Will enforce a maximum count of 2.`, func(t *ftt.Test) {
					req.LogCount = 2
					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0, 1))
				})

				t.Run(`When requesting protobufs`, func(t *ftt.Test) {
					req.State = true

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0, 1, 2))

					// Confirm that this has protobufs.
					assert.Loosely(t, len(resp.Logs), should.Equal(3))
					assert.Loosely(t, resp.Logs[0], should.NotBeNil)

					// Confirm that there is a descriptor protobuf.
					assert.Loosely(t, resp.Desc, should.Resemble(tls.Desc))

					// Confirm that the state was returned.
					assert.Loosely(t, resp.State, should.NotBeNil)
				})

				t.Run(`Will successfully retrieve all records if non-contiguous is allowed.`, func(t *ftt.Test) {
					req.NonContiguous = true
					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0, 1, 2, 4, 5, 7))
				})

				t.Run(`When newlines are not requested, does not include delimiters.`, func(t *ftt.Test) {
					req.LogCount = 1

					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(0))

					assert.Loosely(t, resp.Logs[0].GetText(), should.Resemble(&logpb.Text{
						Lines: []*logpb.Text_Line{
							{Value: []byte("log entry #0"), Delimiter: "\n"},
							{Value: []byte("another line of text"), Delimiter: ""},
						},
					}))
				})

				t.Run(`Will get a Binary LogEntry`, func(t *ftt.Test) {
					req.Index = 4
					req.LogCount = 1
					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(4))
					assert.Loosely(t, resp.Logs[0].GetBinary(), should.Resemble(&logpb.Binary{
						Data: []byte{0x00, 0x01, 0x02, 0x03},
					}))
				})

				t.Run(`Will get a Datagram LogEntry`, func(t *ftt.Test) {
					req.Index = 5
					req.LogCount = 1
					resp, err := svr.Get(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(5))
					assert.Loosely(t, resp.Logs[0].GetDatagram(), should.Resemble(&logpb.Datagram{
						Data: []byte{0x00, 0x01, 0x02, 0x03},
						Partial: &logpb.Datagram_Partial{
							Index: 2,
							Size:  1024,
							Last:  false,
						},
					}))
				})
			})

			t.Run(`Testing tail requests`, func(t *ftt.Test) {
				req := logdog.TailRequest{
					Project: string(project),
					Path:    string(tls.Path),
					State:   true,
				}

				// If the stream is archived, the tail index will be 7. Otherwise, it
				// will be 2 (streaming).
				tailIndex := 7
				if !archived {
					tailIndex = 2
				}

				t.Run(`Will successfully retrieve a stream path.`, func(t *ftt.Test) {
					resp, err := svr.Tail(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, shouldHaveLogs(tailIndex))
					assert.Loosely(t, resp.State, should.Resemble(buildLogStreamState(tls.Stream, tls.State)))

					// For non-archival: 1 miss and 1 put, for the tail row.
					// For archival: 1 miss and 1 put, for the index.
					assert.Loosely(t, env.StorageCache.Stats(), should.Resemble(ct.StorageCacheStats{Puts: 1, Misses: 1}))

					t.Run(`Will retrieve the stream path again (caching).`, func(t *ftt.Test) {
						env.StorageCache.Clear()

						resp, err := svr.Tail(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
						assert.Loosely(t, resp, shouldHaveLogs(tailIndex))
						assert.Loosely(t, resp.State, should.Resemble(buildLogStreamState(tls.Stream, tls.State)))

						// For non-archival: 1 hit, for the tail row.
						// For archival: 1 hit, for the index.
						assert.Loosely(t, env.StorageCache.Stats(), should.Resemble(ct.StorageCacheStats{Hits: 1}))
					})
				})
			})
		})
	})
}

func TestGetIntermediate(t *testing.T) {
	t.Parallel()

	testGetImpl(t, false)
}

func TestGetArchived(t *testing.T) {
	t.Parallel()

	testGetImpl(t, true)
}
