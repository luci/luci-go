// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/iotools"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/common/recordio"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/logdog/archive"
	"github.com/luci/luci-go/server/logdog/storage"
	memoryStorage "github.com/luci/luci-go/server/logdog/storage/memory"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

type staticArchiveSource []*logpb.LogEntry

func (s *staticArchiveSource) NextLogEntry() (le *logpb.LogEntry, err error) {
	if len(*s) == 0 {
		err = archive.ErrEndOfStream
	} else {
		le, *s = (*s)[0], (*s)[1:]
	}
	return
}

type testGSClient map[string][]byte

func (c testGSClient) key(bucket, relpath string) string {
	return fmt.Sprintf("%s/%s", bucket, relpath)
}

func (c testGSClient) put(bucket, relpath string, d []byte) {
	c[c.key(bucket, relpath)] = d
}

func (c testGSClient) get(bucket, relpath string) []byte {
	return c[c.key(bucket, relpath)]
}

func (c testGSClient) Close() error { return nil }
func (c testGSClient) NewWriter(string, string) (gs.Writer, error) {
	return nil, errors.New("not implemented")
}
func (c testGSClient) Delete(string, string) error { return errors.New("not implemented") }

func (c testGSClient) NewReader(bucket, relpath string, o gs.Options) (io.ReadCloser, error) {
	if d, ok := c["error"]; ok {
		return nil, errors.New(string(d))
	}

	d, ok := c[c.key(bucket, relpath)]
	if !ok {
		return nil, errors.New("does not exist")
	}

	to := int(o.To)
	if to == 0 {
		to = len(d)
	}
	d = d[int(o.From):to]

	r := make([]byte, len(d))
	copy(r, d)
	gsr := testGSReader(r)
	return &gsr, nil
}

type testGSReader []byte

func (r *testGSReader) Read(d []byte) (int, error) {
	if len(*r) == 0 {
		return 0, io.EOF
	}

	amt := copy(d, *r)
	*r = (*r)[amt:]
	return amt, nil
}

func (r *testGSReader) Close() error { return nil }

func shouldHaveLogs(actual interface{}, expected ...interface{}) string {
	resp := actual.(*logdog.GetResponse)

	respLogs := make([]int, len(resp.Logs))
	for i, le := range resp.Logs {
		respLogs[i] = int(le.StreamIndex)
	}

	expLogs := make([]int, len(expected))
	for i, exp := range expected {
		expLogs[i] = exp.(int)
	}

	return ShouldResemble(respLogs, expLogs)
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

		pos := int(cr.Count())
		for i := int64(0); i < s; i++ {
			d[pos+int(i)] = 0x00
		}

		// Read the (now-zeroed) data.
		trash.Reset()
		trash.ReadFrom(r)
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration, a Get request`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)

		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		c = ct.UseConfig(c, &svcconfig.Coordinator{
			AdminAuthGroup: "test-administrators",
		})

		// Generate our test stream.
		desc := ct.TestLogStreamDescriptor(c, "foo/bar")
		ls := ct.TestLogStream(c, desc)
		if err := ls.Put(ds.Get(c)); err != nil {
			panic(err)
		}

		tc.Add(time.Second)
		var entries []*logpb.LogEntry
		protobufs := map[uint64][]byte{}
		for _, v := range []int{0, 1, 2, 4, 5, 7} {
			le := ct.TestLogEntry(c, ls, v)
			le.GetText().Lines = append(le.GetText().Lines, &logpb.Text_Line{
				Value: "another line of text",
			})
			entries = append(entries, le)

			switch v {
			case 4:
				le.Content = &logpb.LogEntry_Binary{
					&logpb.Binary{
						Data: []byte{0x00, 0x01, 0x02, 0x03},
					},
				}

			case 5:
				le.Content = &logpb.LogEntry_Datagram{
					&logpb.Datagram{
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

		// Define and populate our Storage.
		s := Server{}

		ms := memoryStorage.Storage{}
		gsc := testGSClient{}
		s.StorageFunc = func(context.Context) (storage.Storage, error) {
			return &ms, nil
		}
		s.GSClientFunc = func(context.Context) (gs.Client, error) {
			return gsc, nil
		}

		Convey(`Testing Get requests (no logs)`, func() {
			req := logdog.GetRequest{
				Path: string(ls.Path()),
			}

			Convey(`Will fail if the Path is not a stream path or a hash.`, func() {
				req.Path = "not/a/full/stream/path"
				_, err := s.Get(c, &req)
				So(err, ShouldErrLike, "invalid path value")
			})

			Convey(`Will fail with Internal if the datastore Get() doesn't work.`, func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("testing error"), "GetMulti")

				_, err := s.Get(c, &req)
				So(err, ShouldBeRPCInternal)
			})

			Convey(`Will fail with NotFound if the log stream does not exist.`, func() {
				req.Path = "testing/+/does/not/exist"
				_, err := s.Get(c, &req)
				So(err, ShouldBeRPCNotFound)
			})
		})

		for _, v := range []bool{
			false,
			true,
		} {
			is := "is"
			if !v {
				is += " not"
			}

			Convey(fmt.Sprintf(`When the log %s archived`, is), func() {
				if !v {
					// Add the logs to the in-memory temporary storage.
					for _, le := range entries {
						err := ms.Put(storage.PutRequest{
							Path:   ls.Path(),
							Index:  types.MessageIndex(le.StreamIndex),
							Values: [][]byte{protobufs[le.StreamIndex]},
						})
						if err != nil {
							panic(fmt.Errorf("failed to Put() LogEntry: %v", err))
						}
					}
				} else {
					// Archive this log stream. We will generate one index entry for every
					// 2 log entries.
					src := staticArchiveSource(entries)
					var lbuf, ibuf bytes.Buffer
					m := archive.Manifest{
						Desc:             desc,
						Source:           &src,
						LogWriter:        &lbuf,
						IndexWriter:      &ibuf,
						StreamIndexRange: 2,
					}
					if err := archive.Archive(m); err != nil {
						panic(err)
					}

					gsc.put("testbucket", "stream", lbuf.Bytes())
					gsc.put("testbucket", "index", ibuf.Bytes())
					ls.State = coordinator.LSArchived
					ls.ArchiveStreamURL = "gs://testbucket/stream"
					ls.ArchiveIndexURL = "gs://testbucket/index"
				}
				if err := ls.Put(ds.Get(c)); err != nil {
					panic(err)
				}

				Convey(`Testing Get requests`, func() {
					req := logdog.GetRequest{
						Path: string(ls.Path()),
					}

					Convey(`When the log stream is purged`, func() {
						ls.Purged = true
						if err := ls.Put(ds.Get(c)); err != nil {
							panic(err)
						}

						Convey(`Will return NotFound if the user is not an administrator.`, func() {
							_, err := s.Get(c, &req)
							So(err, ShouldBeRPCNotFound)
						})

						Convey(`Will process the request if the user is an administrator.`, func() {
							fs.IdentityGroups = []string{"test-administrators"}

							resp, err := s.Get(c, &req)
							So(err, ShouldBeRPCOK)
							So(resp, shouldHaveLogs, 0, 1, 2)
						})
					})

					Convey(`Will return empty if no records were requested.`, func() {
						req.LogCount = -1
						req.State = false

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp.Logs, ShouldHaveLength, 0)
					})

					Convey(`Will successfully retrieve a stream path.`, func() {
						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0, 1, 2)
					})

					Convey(`Will successfully retrieve a stream path offset at 4.`, func() {
						req.Index = 4

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 4, 5)
					})

					Convey(`Will retrieve no logs for contiguous offset 6.`, func() {
						req.Index = 6

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(len(resp.Logs), ShouldEqual, 0)
					})

					Convey(`Will retrieve log 7 for non-contiguous offset 6.`, func() {
						req.NonContiguous = true
						req.Index = 6

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 7)
					})

					Convey(`With a byte limit of 1, will still return at least one log entry.`, func() {
						req.ByteCount = 1

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0)
					})

					Convey(`With a byte limit of sizeof(0), will return log entry 0.`, func() {
						req.ByteCount = int32(len(protobufs[0]))

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0)
					})

					Convey(`With a byte limit of sizeof(0)+1, will return log entry 0.`, func() {
						req.ByteCount = int32(len(protobufs[0]))

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0)
					})

					Convey(`With a byte limit of sizeof({0, 1}), will return log entries {0, 1}.`, func() {
						req.ByteCount = int32(len(protobufs[0]) + len(protobufs[1]))

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0, 1)
					})

					Convey(`With a byte limit of sizeof({0, 1, 2}), will return log entries {0, 1, 2}.`, func() {
						req.ByteCount = int32(len(protobufs[0]) + len(protobufs[1]) + len(protobufs[2]))

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0, 1, 2)
					})

					Convey(`With a byte limit of sizeof({0, 1, 2})+1, will return log entries {0, 1, 2}.`, func() {
						req.ByteCount = int32(len(protobufs[0]) + len(protobufs[1]) + len(protobufs[2]) + 1)

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0, 1, 2)
					})

					Convey(`Will successfully retrieve a stream path hash.`, func() {
						req.Path = ls.HashID()
						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0, 1, 2)
					})

					Convey(`When requesting state`, func() {
						req.State = true
						req.LogCount = -1

						Convey(`Will successfully retrieve stream state.`, func() {
							resp, err := s.Get(c, &req)
							So(err, ShouldBeRPCOK)
							So(resp.State, ShouldResemble, loadLogStreamState(ls))
							So(len(resp.Logs), ShouldEqual, 0)
						})

						Convey(`Will return Internal if the protobuf descriptor data is corrupt.`, func() {
							// We can't use "ls.Put" here because it validates the protobuf!
							ls.Descriptor = []byte{0x00} // Invalid protobuf, zero tag.
							if err := ds.Get(c).Put(ls); err != nil {
								panic(err)
							}

							_, err := s.Get(c, &req)
							So(err, ShouldBeRPCInternal)
						})
					})

					Convey(`Will return Internal if the protobuf log entry data is corrupt.`, func() {
						if v {
							// Corrupt the archive datastream.
							stream := gsc.get("testbucket", "stream")
							zeroRecords(stream)
						} else {
							// Add corrupted entry to Storage. Create a new entry here, since
							// the storage will reject a duplicate/overwrite.
							err := ms.Put(storage.PutRequest{
								Path:   types.StreamPath(req.Path),
								Index:  666,
								Values: [][]byte{{0x00}}, // Invalid protobuf, zero tag.
							})
							if err != nil {
								panic(err)
							}
							req.Index = 666
						}

						_, err := s.Get(c, &req)
						So(err, ShouldBeRPCInternal)
					})

					Convey(`Will successfully retrieve both logs and stream state.`, func() {
						req.State = true

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp.State, ShouldResemble, loadLogStreamState(ls))
						So(resp, shouldHaveLogs, 0, 1, 2)
					})

					Convey(`Will return Internal if the Storage is not working.`, func() {
						if v {
							gsc["error"] = []byte("test error")
						} else {
							ms.Close()
						}

						_, err := s.Get(c, &req)
						So(err, ShouldBeRPCInternal)
					})

					Convey(`Will enforce a maximum count of 2.`, func() {
						req.LogCount = 2
						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0, 1)
					})

					Convey(`When requesting protobufs`, func() {
						req.State = true

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0, 1, 2)

						// Confirm that this has protobufs.
						So(len(resp.Logs), ShouldEqual, 3)
						So(resp.Logs[0], ShouldNotBeNil)

						// Confirm that there is a descriptor protobuf.
						So(resp.Desc, ShouldResemble, desc)

						// Confirm that the state was returned.
						So(resp.State, ShouldNotBeNil)
					})

					Convey(`Will successfully retrieve all records if non-contiguous is allowed.`, func() {
						req.NonContiguous = true
						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0, 1, 2, 4, 5, 7)
					})

					Convey(`When newlines are not requested, does not include delimiters.`, func() {
						req.LogCount = 1

						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 0)

						So(resp.Logs[0].GetText(), ShouldResemble, &logpb.Text{
							Lines: []*logpb.Text_Line{
								{"log entry #0", "\n"},
								{"another line of text", ""},
							},
						})
					})

					Convey(`Will get a Binary LogEntry`, func() {
						req.Index = 4
						req.LogCount = 1
						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 4)
						So(resp.Logs[0].GetBinary(), ShouldResemble, &logpb.Binary{
							Data: []byte{0x00, 0x01, 0x02, 0x03},
						})
					})

					Convey(`Will get a Datagram LogEntry`, func() {
						req.Index = 5
						req.LogCount = 1
						resp, err := s.Get(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 5)
						So(resp.Logs[0].GetDatagram(), ShouldResemble, &logpb.Datagram{
							Data: []byte{0x00, 0x01, 0x02, 0x03},
							Partial: &logpb.Datagram_Partial{
								Index: 2,
								Size:  1024,
								Last:  false,
							},
						})
					})
				})

				Convey(`Testing tail requests`, func() {
					req := logdog.TailRequest{
						Path: string(ls.Path()),
					}

					Convey(`Will successfully retrieve a stream path.`, func() {
						resp, err := s.Tail(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 7)
					})

					Convey(`Will successfully retrieve a stream path hash and state.`, func() {
						req.Path = ls.HashID()
						req.State = true

						resp, err := s.Tail(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, shouldHaveLogs, 7)
						So(resp.State, ShouldResemble, loadLogStreamState(ls))
					})
				})
			})
		}
	})
}
