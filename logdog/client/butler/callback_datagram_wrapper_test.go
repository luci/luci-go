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

package butler

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/api/logpb"
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

func mkWrappedDatagramCb(values *[][]byte, seq *[]uint64) StreamChunkCallback {
	cb := func(le *logpb.LogEntry) {
		if le == nil {
			return
		}
		*values = append(*values, append(le.GetDatagram().Data, 0xbb))
		*seq = append(*seq, le.Sequence)
	}
	return getWrappedDatagramCallback(cb)
}

func TestDatagramReassembler(t *testing.T) {
	t.Parallel()

	ftt.Run(`Callback wrapper works`, t, func(t *ftt.Test) {
		t.Run(`With nil`, func(t *ftt.Test) {
			values, seq := [][]byte{}, []uint64{}
			mkWrappedDatagramCb(&values, &seq)(nil)
			assert.Loosely(t, values, should.Resemble([][]byte{}))
		})

		t.Run(`With a complete datagram`, func(t *ftt.Test) {
			values, seq := [][]byte{}, []uint64{}
			cbWrapped := mkWrappedDatagramCb(&values, &seq)
			cbWrapped(mkDatagramLogEntry([]byte{0xca, 0xfe}, false, true, 0, 2, 0))
			assert.Loosely(t, values, should.Resemble([][]byte{
				{0xca, 0xfe, 0xbb},
			}))
			assert.Loosely(t, seq, should.Resemble([]uint64{0}))

			t.Run(`And doesn't call on an incomplete datagram`, func(t *ftt.Test) {
				cbWrapped(mkDatagramLogEntry([]byte{0xd0}, true, false, 0, 1, 1))
				cbWrapped(mkDatagramLogEntry([]byte{0x65, 0x10}, true, false, 1, 2, 1))
				assert.Loosely(t, values, should.Resemble([][]byte{
					{0xca, 0xfe, 0xbb},
				}))
				assert.Loosely(t, seq, should.Resemble([]uint64{0}))

				t.Run(`Until a LogEntry completes it`, func(t *ftt.Test) {
					cbWrapped(mkDatagramLogEntry([]byte{0xbb, 0x12}, true, true, 2, 2, 1))
					assert.Loosely(t, values, should.Resemble([][]byte{
						{0xca, 0xfe, 0xbb},
						{0xd0, 0x65, 0x10, 0xbb, 0x12, 0xbb},
					}))
					assert.Loosely(t, seq, should.Resemble([]uint64{0, 1}))
				})
			})
		})
	})

	ftt.Run(`Callback wrapper panics`, t, func(t *ftt.Test) {
		cbWrapped := mkWrappedDatagramCb(nil, nil)

		t.Run(`When called on non-datagram LogEntries`, func(t *ftt.Test) {
			assert.Loosely(t,
				func() {
					cbWrapped(&logpb.LogEntry{Content: &logpb.LogEntry_Text{}})
				},
				should.PanicLike(
					"expected *logpb.LogEntry_Datagram",
				))
		})

		t.Run(`When called on a complete datagram while buffered entries exist`, func(t *ftt.Test) {
			cbWrapped(mkDatagramLogEntry([]byte{0xd0}, true, false, 0, 1, 0))
			cbWrapped(mkDatagramLogEntry([]byte{0x65, 0x10}, true, false, 1, 2, 0))
			assert.Loosely(t,
				func() {
					cbWrapped(mkDatagramLogEntry([]byte{0xbb}, false, true, 0, 1, 1))
				},
				should.PanicLike(
					"got self-contained Datagram LogEntry while buffered LogEntries exist",
				))
		})
	})
}
