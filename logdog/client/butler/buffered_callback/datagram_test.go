// Copyright 2018 The LUCI Authors.
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

package buffered_callback

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bundler"
)

func mkDatagramLogEntry(data []byte, partial, last bool, index uint32, size uint64, seq uint64) *logpb.LogEntry {
	le := &logpb.LogEntry{
		Sequence: seq,
		Content: &logpb.LogEntry_Datagram{
			Datagram: &logpb.Datagram{Data: data},
		},
	}
	if partial {
		le.GetDatagram().Partial = &logpb.Datagram_Partial{
			Index: index,
			Size:  size,
			Last:  last,
		}
	}
	return le
}

func mkWrappedDatagramCb(values *[][]byte, seq *[]uint64) bundler.StreamChunkCallback {
	cb := func(le *logpb.LogEntry) {
		*values = append(*values, append(le.GetDatagram().Data, 0xbb))
		*seq = append(*seq, le.Sequence)
	}
	return GetWrappedDatagramCallback(cb)
}

func TestDatagramReassembler(t *testing.T) {
	t.Parallel()

	Convey(`Callback wrapper works`, t, func() {
		Convey(`With nil`, func() {
			values, seq := [][]byte{}, []uint64{}
			mkWrappedDatagramCb(&values, &seq)(nil)
			So(values, ShouldResemble, [][]byte{})
		})

		Convey(`With a complete datagram`, func() {
			values, seq := [][]byte{}, []uint64{}
			cbWrapped := mkWrappedDatagramCb(&values, &seq)
			cbWrapped(mkDatagramLogEntry([]byte{0xca, 0xfe}, false, true, 0, 2, 0))
			So(values, ShouldResemble, [][]byte{
				{0xca, 0xfe, 0xbb},
			})
			So(seq, ShouldResemble, []uint64{0})

			Convey(`And doesn't call on an incomplete datagram`, func() {
				cbWrapped(mkDatagramLogEntry([]byte{0xd0}, true, false, 0, 1, 1))
				cbWrapped(mkDatagramLogEntry([]byte{0x65, 0x10}, true, false, 1, 2, 1))
				So(values, ShouldResemble, [][]byte{
					{0xca, 0xfe, 0xbb},
				})
				So(seq, ShouldResemble, []uint64{0})

				Convey(`Until a LogEntry completes it`, func() {
					cbWrapped(mkDatagramLogEntry([]byte{0xbb, 0x12}, true, true, 2, 2, 1))
					So(values, ShouldResemble, [][]byte{
						{0xca, 0xfe, 0xbb},
						{0xd0, 0x65, 0x10, 0xbb, 0x12, 0xbb},
					})
					So(seq, ShouldResemble, []uint64{0, 1})
				})
			})
		})
	})

	Convey(`Callback wrapper panics`, t, func() {
		cbWrapped := mkWrappedDatagramCb(nil, nil)

		Convey(`When called on non-datagram LogEntries`, func() {
			So(
				func() {
					cbWrapped(&logpb.LogEntry{Content: &logpb.LogEntry_Text{}})
				},
				assertions.ShouldPanicLike,
				errors.Annotate(
					InvalidStreamType,
					fmt.Sprintf("got *logpb.LogEntry_Text, expected *logpb.LogEntry_Datagram"),
				).Err(),
			)
		})

		Convey(`When called on a complete datagram while buffered entries exist`, func() {
			cbWrapped(mkDatagramLogEntry([]byte{0xd0}, true, false, 0, 1, 0))
			cbWrapped(mkDatagramLogEntry([]byte{0x65, 0x10}, true, false, 1, 2, 0))
			So(
				func() {
					cbWrapped(mkDatagramLogEntry([]byte{0xbb}, false, true, 0, 1, 1))
				},
				ShouldPanicWith,
				LostDatagramChunk,
			)
		})
	})
}
