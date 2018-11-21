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

type line struct {
	value string
	delim string
}

func mkTextLogEntry(lines []line) *logpb.LogEntry {
	logLines := make([]*logpb.Text_Line, 0, len(lines))
	for _, l := range lines {
		logLines = append(logLines, &logpb.Text_Line{
			Value:     []byte(l.value),
			Delimiter: l.delim,
		})
	}
	le := &logpb.LogEntry{
		Content: &logpb.LogEntry_Text{
			Text: &logpb.Text{Lines: logLines},
		},
	}
	return le
}

func mkWrappedTextCb(values *[]string) bundler.StreamChunkCallback {
	cb := func(le *logpb.LogEntry) {
		for _, l := range le.GetText().Lines {
			*values = append(*values, fmt.Sprintf(
				"%s!%s", string(l.Value), l.Delimiter,
			))
		}
	}
	return GetWrappedTextCallback(cb)
}

func TestTextReassembler(t *testing.T) {
	t.Parallel()

	Convey(`Callback wrapper works`, t, func() {
		Convey(`With nil`, func() {
			values := []string{}
			mkWrappedTextCb(&values)(nil)
			So(values, ShouldResemble, []string{})
		})

		Convey(`With only complete lines`, func() {
			values := []string{}
			mkWrappedTextCb(&values)(mkTextLogEntry([]line{
				{"hi", "\n"},
				{"there", "\n"},
			}))
			So(values, ShouldResemble, []string{
				"hi!\n",
				"there!\n",
			})
		})

		Convey(`With partial lines`, func() {
			values := []string{}
			cbWrapped := mkWrappedTextCb(&values)

			Convey(`At the beginning of a LogEntry`, func() {
				cbWrapped(mkTextLogEntry([]line{
					{"h", ""},
				}))
				So(values, ShouldResemble, []string{})
			})

			Convey(`At the end of a LogEntry`, func() {
				cbWrapped(mkTextLogEntry([]line{
					{"hi", "\n"},
					{"there", "\n"},
					{"ho", ""},
				}))
				So(values, ShouldResemble, []string{
					"hi!\n",
					"there!\n",
				})

				Convey(`And correctly completes with the next LogEntry`, func() {
					cbWrapped(mkTextLogEntry([]line{
						{"w are you", "\n"},
					}))
					So(values, ShouldResemble, []string{
						"hi!\n",
						"there!\n",
						"how are you!\n",
					})
				})
			})
		})
	})

	Convey(`Callback wrapper panics`, t, func() {
		cbWrapped := mkWrappedTextCb(&[]string{})

		Convey(`When called on non-text LogEntries`, func() {
			So(
				func() {
					cbWrapped(&logpb.LogEntry{Content: &logpb.LogEntry_Datagram{}})
				},
				assertions.ShouldPanicLike,
				errors.Annotate(
					InvalidStreamType,
					fmt.Sprintf("got *logpb.LogEntry_Datagram, expected *logpb.LogEntry_Text"),
				).Err(),
			)
		})

		Convey(`When the partial line is not in expected location`, func() {
			Convey(`Like the first of multiple lines`, func() {
				So(
					func() {
						cbWrapped(mkTextLogEntry([]line{
							{"w ar", ""},
							{"e yo", ""},
						}))
					},
					ShouldPanicWith,
					PartialLineNotLast,
				)
			})

			Convey(`Like interspersed with complete lines`, func() {
				So(
					func() {
						cbWrapped(mkTextLogEntry([]line{
							{"w", "\n"},
							{"ar", ""},
							{"e you", "\n"},
						}))
					},
					ShouldPanicWith,
					PartialLineNotLast,
				)
			})
		})
	})
}
