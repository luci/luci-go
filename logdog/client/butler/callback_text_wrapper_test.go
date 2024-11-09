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
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
)

type line struct {
	value string
	delim string
}

func mkTextLogEntry(lines []line, seq uint64) *logpb.LogEntry {
	logLines := make([]*logpb.Text_Line, 0, len(lines))
	for _, l := range lines {
		logLines = append(logLines, &logpb.Text_Line{
			Value:     []byte(l.value),
			Delimiter: l.delim,
		})
	}
	le := &logpb.LogEntry{
		Sequence: seq,
		Content: &logpb.LogEntry_Text{
			Text: &logpb.Text{Lines: logLines},
		},
	}
	return le
}

func mkWrappedTextCb(values *[]string, seq *[]uint64) StreamChunkCallback {
	cb := func(le *logpb.LogEntry) {
		if le == nil {
			return
		}
		for _, l := range le.GetText().Lines {
			*values = append(*values, fmt.Sprintf(
				"%s!%s", string(l.Value), l.Delimiter,
			))
		}
		*seq = append(*seq, le.Sequence)
	}
	return getWrappedTextCallback(cb)
}

func TestTextReassembler(t *testing.T) {
	t.Parallel()

	ftt.Run(`Callback wrapper works`, t, func(t *ftt.Test) {
		t.Run(`With nil`, func(t *ftt.Test) {
			values, seq := []string{}, []uint64{}
			mkWrappedTextCb(&values, &seq)(nil)
			assert.Loosely(t, values, should.Resemble([]string{}))
		})

		t.Run(`With only complete lines`, func(t *ftt.Test) {
			values, seq := []string{}, []uint64{}
			mkWrappedTextCb(&values, &seq)(mkTextLogEntry([]line{
				{"hi", "\n"},
				{"there", "\n"},
			}, 0))
			assert.Loosely(t, values, should.Resemble([]string{
				"hi!\n",
				"there!\n",
			}))
			assert.Loosely(t, seq, should.Resemble([]uint64{0}))
		})

		t.Run(`With partial lines`, func(t *ftt.Test) {
			values, seq := []string{}, []uint64{}
			cbWrapped := mkWrappedTextCb(&values, &seq)

			t.Run(`At the beginning of a LogEntry`, func(t *ftt.Test) {
				cbWrapped(mkTextLogEntry([]line{
					{"h", ""},
				}, 0))
				assert.Loosely(t, values, should.Resemble([]string{}))
				assert.Loosely(t, seq, should.Resemble([]uint64{}))
			})

			t.Run(`At the end of a LogEntry`, func(t *ftt.Test) {
				cbWrapped(mkTextLogEntry([]line{
					{"hi", "\n"},
					{"there", "\n"},
					{"ho", ""},
				}, 0))
				assert.Loosely(t, values, should.Resemble([]string{
					"hi!\n",
					"there!\n",
				}))
				assert.Loosely(t, seq, should.Resemble([]uint64{0}))

				t.Run(`And correctly completes with the next LogEntry`, func(t *ftt.Test) {
					cbWrapped(mkTextLogEntry([]line{
						{"w are you", "\n"},
					}, 2))
					assert.Loosely(t, values, should.Resemble([]string{
						"hi!\n",
						"there!\n",
						"how are you!\n",
					}))
					assert.Loosely(t, seq, should.Resemble([]uint64{0, 2}))
				})

				t.Run(`Flushes when called with niil`, func(t *ftt.Test) {
					cbWrapped(nil)
					assert.Loosely(t, values, should.Resemble([]string{
						"hi!\n",
						"there!\n",
						"ho!",
					}))
					assert.Loosely(t, seq, should.Resemble([]uint64{0, 2}))
				})
			})
		})
	})

	ftt.Run(`Callback wrapper panics`, t, func(t *ftt.Test) {
		cbWrapped := mkWrappedTextCb(nil, nil)

		t.Run(`When called on non-text LogEntries`, func(t *ftt.Test) {
			assert.Loosely(t,
				func() {
					cbWrapped(&logpb.LogEntry{Content: &logpb.LogEntry_Datagram{}})
				},
				should.PanicLike(
					"expected *logpb.LogEntry_Text",
				))
		})

		t.Run(`When the partial line is not in expected location`, func(t *ftt.Test) {
			t.Run(`Like the first of multiple lines`, func(t *ftt.Test) {
				assert.Loosely(t,
					func() {
						cbWrapped(mkTextLogEntry([]line{
							{"w ar", ""},
							{"e yo", ""},
						}, 2))
					},
					should.PanicLike(
						"partial line not last",
					))
			})

			t.Run(`Like interspersed with complete lines`, func(t *ftt.Test) {
				assert.Loosely(t,
					func() {
						cbWrapped(mkTextLogEntry([]line{
							{"w", "\n"},
							{"ar", ""},
							{"e you", "\n"},
						}, 2))
					},
					should.PanicLike(
						"partial line not last",
					))
			})
		})
	})
}
