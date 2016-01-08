// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/ephelper"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/services"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/logdog/storage"
	memoryStorage "github.com/luci/luci-go/server/logdog/storage/memory"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/appengine/ephelper/assertions"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func shouldHaveLogs(actual interface{}, expected ...interface{}) string {
	resp := actual.(*GetResponse)

	logs := make([]int, len(resp.Logs))
	for i, le := range resp.Logs {
		if le.Proto != nil {
			leProto := protocol.LogEntry{}
			if err := proto.Unmarshal(le.Proto, &leProto); err != nil {
				return fmt.Sprintf("Failed to unmarshal entry #%d protobuf: %v", i, err)
			}
			logs[i] = int(leProto.StreamIndex)
		} else {
			logs[i] = int(le.Entry.StreamIndex)
		}
	}

	expLogs := make([]int, len(expected))
	for i, exp := range expected {
		expLogs[i] = exp.(int)
	}

	return ShouldResembleV(logs, expLogs)
}

func putLogEntry(st storage.Storage, path types.StreamPath, le *protocol.LogEntry) []byte {
	d, err := proto.Marshal(le)
	if err != nil {
		panic(fmt.Errorf("failed to marshal protobuf: %v", err))
	}

	err = st.Put(&storage.PutRequest{
		Path:  path,
		Index: types.MessageIndex(le.StreamIndex),
		Value: d,
	})
	if err != nil {
		panic(fmt.Errorf("failed to Put() LogEntry: %v", err))
	}
	return d
}

func TestGet(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration, a Get request`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)

		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		c = ct.UseConfig(c, &services.Coordinator{
			AdminAuthGroup: "test-administrators",
		})

		req := GetRequest{
			Path: "testing/+/foo/bar",
		}

		// Define and populate our Storage.
		protobufs := map[int][]byte{}
		ms := memoryStorage.Storage{}

		s := Logs{
			ServiceBase: ephelper.ServiceBase{
				Middleware: ephelper.TestMode,
			},
			storageFunc: func(context.Context) (storage.Storage, error) {
				return &ms, nil
			},
		}

		desc := ct.TestLogStreamDescriptor(c, "foo/bar")
		ls, err := ct.TestLogStream(c, desc)
		So(err, ShouldBeNil)
		So(ls.Put(ds.Get(c)), ShouldBeNil)

		tc.Add(time.Second)
		for _, v := range []int{0, 1, 2, 4, 5, 7} {
			le := ct.TestLogEntry(c, ls, v)
			le.GetText().Lines = append(le.GetText().Lines, &protocol.Text_Line{
				Value: "another line of text",
			})
			protobufs[v] = putLogEntry(&ms, types.StreamPath(req.Path), le)
		}

		Convey(`Will fail if the Path is not a stream path or a hash.`, func() {
			req.Path = "not/a/full/stream/path"
			_, err := s.Get(c, &req)
			So(err, ShouldErrLike, "invalid path value")
		})

		Convey(`Will fail with InternalServerError if the datastore Get() doesn't work.`, func() {
			c, fb := featureBreaker.FilterRDS(c, nil)
			fb.BreakFeatures(errors.New("testing error"), "GetMulti")

			_, err := s.Get(c, &req)
			So(err, ShouldBeInternalServerError)
		})

		Convey(`Will fail with NotFound if the log stream does not exist.`, func() {
			req.Path = "testing/+/does/not/exist"
			_, err := s.Get(c, &req)
			So(err, ShouldBeNotFoundError)
		})

		Convey(`When the log stream is purged`, func() {
			ls.Purged = true
			So(ls.Put(ds.Get(c)), ShouldBeNil)

			Convey(`Will return NotFound if the user is not an administrator.`, func() {
				_, err := s.Get(c, &req)
				So(err, ShouldBeNotFoundError)
			})

			Convey(`Will process the request if the user is an administrator.`, func() {
				fs.IdentityGroups = []string{"test-administrators"}

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0, 1, 2)
			})
		})

		Convey(`Testing head requests`, func() {
			Convey(`Will return nil if no records were requested.`, func() {
				req.Count = -1
				req.State = false

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, ShouldBeNil)
			})

			Convey(`Will successfully retrieve a stream path.`, func() {
				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0, 1, 2)
			})

			Convey(`Will successfully retrieve a stream path offset at 4.`, func() {
				req.Index = 4

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 4, 5)
			})

			Convey(`Will retrieve no logs for contiguous offset 6.`, func() {
				req.Index = 6

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(len(resp.Logs), ShouldEqual, 0)
			})

			Convey(`Will retrieve log 7 for non-contiguous offset 6.`, func() {
				req.NonContiguous = true
				req.Index = 6

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 7)
			})

			Convey(`With a byte limit of 1, will still return at least one log entry.`, func() {
				req.Bytes = 1

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0)
			})

			Convey(`With a byte limit of sizeof(0), will return log entry 0.`, func() {
				req.Bytes = len(protobufs[0])

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0)
			})

			Convey(`With a byte limit of sizeof(0)+1, will return log entry 0.`, func() {
				req.Bytes = len(protobufs[0])

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0)
			})

			Convey(`With a byte limit of sizeof({0, 1}), will return log entries {0, 1}.`, func() {
				req.Bytes = len(protobufs[0]) + len(protobufs[1])

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0, 1)
			})

			Convey(`With a byte limit of sizeof({0, 1, 2}), will return log entries {0, 1, 2}.`, func() {
				req.Bytes = len(protobufs[0]) + len(protobufs[1]) + len(protobufs[2])

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0, 1, 2)
			})

			Convey(`With a byte limit of sizeof({0, 1, 2})+1, will return log entries {0, 1, 2}.`, func() {
				req.Bytes = len(protobufs[0]) + len(protobufs[1]) + len(protobufs[2]) + 1

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0, 1, 2)
			})

			Convey(`Will successfully retrieve a stream path hash.`, func() {
				req.Path = ls.HashID()
				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 0, 1, 2)
			})
		})

		Convey(`Testing tail requests`, func() {
			req.Tail = true

			Convey(`Will successfully retrieve a stream path.`, func() {
				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 7)
			})

			Convey(`Will successfully retrieve a stream path hash.`, func() {
				req.Path = ls.HashID()
				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 7)
			})
		})

		Convey(`When requesting state`, func() {
			req.State = true
			req.Count = -1

			Convey(`Will successfully retrieve stream state.`, func() {
				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp.State, ShouldResembleV, lep.LoadLogStreamState(ls))
				So(len(resp.Logs), ShouldEqual, 0)
			})

			Convey(`Will return InternalServerError if the protobuf descriptor data is corrupt.`, func() {
				// We can't use "ls.Put" here because it validates the protobuf!
				ls.Descriptor = []byte{0x00} // Invalid protobuf, zero tag.
				So(ds.Get(c).Put(ls), ShouldBeNil)

				_, err := s.Get(c, &req)
				So(err, ShouldBeInternalServerError)
			})
		})

		Convey(`Will return InternalServerError if the protobuf log entry data is corrupt.`, func() {
			So(ms.Put(&storage.PutRequest{
				Path:  types.StreamPath(req.Path),
				Index: 666,
				Value: []byte{0x00}, // Invalid protobuf, zero tag.
			}), ShouldBeNil)

			req.Index = 666
			_, err := s.Get(c, &req)
			So(err, ShouldBeInternalServerError)
		})

		Convey(`Will successfully retrieve both logs and stream state.`, func() {
			req.State = true

			resp, err := s.Get(c, &req)
			So(err, ShouldBeNil)
			So(resp.State, ShouldResembleV, lep.LoadLogStreamState(ls))
			So(resp, shouldHaveLogs, 0, 1, 2)
		})

		Convey(`Will return InternalServerError if the Storage is not working.`, func() {
			ms.Close()

			_, err := s.Get(c, &req)
			So(err, ShouldBeInternalServerError)
		})

		Convey(`Will enforce a maximum count of 2.`, func() {
			req.Count = 2
			resp, err := s.Get(c, &req)
			So(err, ShouldBeNil)
			So(resp, shouldHaveLogs, 0, 1)
		})

		Convey(`When requesting protobufs`, func() {
			req.Proto = true
			req.State = true

			resp, err := s.Get(c, &req)
			So(err, ShouldBeNil)
			So(resp, shouldHaveLogs, 0, 1, 2)

			// Confirm that this has protobufs.
			So(len(resp.Logs), ShouldEqual, 3)
			So(resp.Logs[0].Proto, ShouldNotBeNil)

			// Confirm that there is a descriptor protobuf.
			So(resp.DescriptorProto, ShouldNotBeNil)
			respDesc := protocol.LogStreamDescriptor{}
			err = proto.Unmarshal(resp.DescriptorProto, &respDesc)
			So(err, ShouldBeNil)
			So(&respDesc, ShouldResembleV, desc)

			// Confirm that the state was returned.
			So(resp.State, ShouldNotBeNil)
		})

		Convey(`Will successfully retrieve all records if non-contiguous is allowed.`, func() {
			req.NonContiguous = true
			resp, err := s.Get(c, &req)
			So(err, ShouldBeNil)
			So(resp, shouldHaveLogs, 0, 1, 2, 4, 5, 7)
		})

		Convey(`When newlines are not requested, does not include delimiters.`, func() {
			req.Count = 1

			resp, err := s.Get(c, &req)
			So(err, ShouldBeNil)
			So(resp, shouldHaveLogs, 0)

			So(resp.Logs[0].Entry.Text, ShouldResembleV, []string{"log entry #0", "another line of text"})
		})

		Convey(`When newlines are requested, will include delimiters.`, func() {
			req.Newlines = true
			req.Count = 1

			resp, err := s.Get(c, &req)
			So(err, ShouldBeNil)
			So(resp, shouldHaveLogs, 0)

			So(resp.Logs[0].Entry.Text, ShouldResembleV, []string{"log entry #0\n", "another line of text"})
		})

		Convey(`A Binary LogEntry`, func() {
			le := protocol.LogEntry{
				StreamIndex: 777,

				// Fill in content so zero-indexed LogEntry isn't zero bytes.
				Content: &protocol.LogEntry_Binary{
					&protocol.Binary{
						Data: []byte{0x00, 0x01, 0x02, 0x03},
					},
				},
			}
			putLogEntry(&ms, types.StreamPath(req.Path), &le)

			Convey(`Will return binary data.`, func() {
				req.Index = 777

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 777)

				So(resp.Logs[0].Entry.Binary, ShouldResembleV, []byte{0x00, 0x01, 0x02, 0x03})
			})
		})

		Convey(`A partial Datagram LogEntry`, func() {
			le := protocol.LogEntry{
				StreamIndex: 777,

				// Fill in content so zero-indexed LogEntry isn't zero bytes.
				Content: &protocol.LogEntry_Datagram{
					&protocol.Datagram{
						Data: []byte{0x00, 0x01, 0x02, 0x03},
						Partial: &protocol.Datagram_Partial{
							Index: 2,
							Size:  1024,
							Last:  false,
						},
					},
				},
			}
			putLogEntry(&ms, types.StreamPath(req.Path), &le)

			Convey(`Will return datagram data.`, func() {
				req.Index = 777

				resp, err := s.Get(c, &req)
				So(err, ShouldBeNil)
				So(resp, shouldHaveLogs, 777)

				So(resp.Logs[0].Entry.Datagram, ShouldResembleV, &LogEntryDatagram{
					Data: []byte{0x00, 0x01, 0x02, 0x03},
					Partial: &LogEntryDatagramPartial{
						Index: 2,
						Size:  1024,
						Last:  false,
					},
				})
			})
		})
	})
}
