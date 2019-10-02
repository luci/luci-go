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

	"go.chromium.org/luci/logdog/api/logpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey(`Callback wrapper works`, t, func() {
		Convey(`With nil`, func() {
			values, seq := []string{}, []uint64{}
			mkWrappedTextCb(&values, &seq)(nil)
			So(values, ShouldResemble, []string{})
		})

		Convey(`With only complete lines`, func() {
			values, seq := []string{}, []uint64{}
			mkWrappedTextCb(&values, &seq)(mkTextLogEntry([]line{
				{"hi", "\n"},
				{"there", "\n"},
			}, 0))
			So(values, ShouldResemble, []string{
				"hi!\n",
				"there!\n",
			})
			So(seq, ShouldResemble, []uint64{0})
		})

		Convey(`With partial lines`, func() {
			values, seq := []string{}, []uint64{}
			cbWrapped := mkWrappedTextCb(&values, &seq)

			Convey(`At the beginning of a LogEntry`, func() {
				cbWrapped(mkTextLogEntry([]line{
					{"h", ""},
				}, 0))
				So(values, ShouldResemble, []string{})
				So(seq, ShouldResemble, []uint64{})
			})

			Convey(`At the end of a LogEntry`, func() {
				cbWrapped(mkTextLogEntry([]line{
					{"hi", "\n"},
					{"there", "\n"},
					{"ho", ""},
				}, 0))
				So(values, ShouldResemble, []string{
					"hi!\n",
					"there!\n",
				})
				So(seq, ShouldResemble, []uint64{0})

				Convey(`And correctly completes with the next LogEntry`, func() {
					cbWrapped(mkTextLogEntry([]line{
						{"w are you", "\n"},
					}, 2))
					So(values, ShouldResemble, []string{
						"hi!\n",
						"there!\n",
						"how are you!\n",
					})
					So(seq, ShouldResemble, []uint64{0, 2})
				})

				Convey(`Flushes when called with niil`, func() {
					cbWrapped(nil)
					So(values, ShouldResemble, []string{
						"hi!\n",
						"there!\n",
						"ho!",
					})
					So(seq, ShouldResemble, []uint64{0, 2})
				})
			})
		})
	})

	Convey(`Callback wrapper panics`, t, func() {
		cbWrapped := mkWrappedTextCb(nil, nil)

		Convey(`When called on non-text LogEntries`, func() {
			So(
				func() {
					cbWrapped(&logpb.LogEntry{Content: &logpb.LogEntry_Datagram{}})
				},
				ShouldPanicLike,
				"expected *logpb.LogEntry_Text",
			)
		})

		Convey(`When the partial line is not in expected location`, func() {
			Convey(`Like the first of multiple lines`, func() {
				So(
					func() {
						cbWrapped(mkTextLogEntry([]line{
							{"w ar", ""},
							{"e yo", ""},
						}, 2))
					},
					ShouldPanicLike,
					"partial line not last",
				)
			})

			Convey(`Like interspersed with complete lines`, func() {
				So(
					func() {
						cbWrapped(mkTextLogEntry([]line{
							{"w", "\n"},
							{"ar", ""},
							{"e you", "\n"},
						}, 2))
					},
					ShouldPanicLike,
					"partial line not last",
				)
			})
		})
	})
}
