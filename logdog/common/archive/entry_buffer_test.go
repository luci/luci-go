// Copyright 2021 The LUCI Authors.
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

package archive

import (
	"fmt"
	"strings"
	"testing"
	"time"

	cl "cloud.google.com/go/logging"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
)

func TestEntryBuffer(t *testing.T) {
	t.Parallel()

	ftt.Run(`entryBuffer`, t, func(t *ftt.Test) {
		maxPayload := 15
		desc := &logpb.LogStreamDescriptor{
			Prefix:    "test",
			Name:      "foo",
			Timestamp: timestamppb.New(testclock.TestTimeUTC),
		}

		genEntry := func(lines ...string) *logpb.LogEntry {
			e := &logpb.LogEntry{
				Content: &logpb.LogEntry_Text{
					Text: &logpb.Text{},
				},
			}
			// only the last line can be a partial line.
			var pl string
			if last := lines[len(lines)-1]; !strings.HasSuffix(last, "\n") {
				pl = last
				lines = lines[:len(lines)-1]
			}

			// add complete lines
			for _, l := range lines {
				e.GetText().Lines = append(e.GetText().Lines, &logpb.Text_Line{
					Value:     []byte(strings.TrimSuffix(l, "\n")),
					Delimiter: "\n",
				})
			}
			// add the partial line
			if pl != "" {
				e.GetText().Lines = append(e.GetText().Lines, &logpb.Text_Line{
					Value: []byte(pl),
				})
			}
			return e
		}

		var eb *entryBuffer
		toCLEs := func(entries ...*logpb.LogEntry) (ret []*cl.Entry) {
			eb = newEntryBuffer(maxPayload, "stream-id", desc)

			if len(entries) == 0 {
				return
			}
			for _, entry := range entries {
				ret = append(ret, eb.append(entry)...)
			}
			if e := eb.flush(); e != nil {
				ret = append(ret, e)
			}
			return
		}

		checkPayloads := func(ces []*cl.Entry, payloads ...string) {
			var actual []string
			for _, e := range ces {
				actual = append(actual, e.Payload.(string))
			}
			assert.Loosely(t, actual, should.Match(payloads))
		}

		t.Run("Sets the entry timestamp based on the stream timestamp", func(t *ftt.Test) {
			es := []*logpb.LogEntry{
				genEntry("line-123\n"),
				genEntry("line-4567\n"),
			}
			es[0].TimeOffset = durationpb.New(time.Second)
			es[1].TimeOffset = durationpb.New(2 * time.Second)
			ces := toCLEs(es...)

			assert.Loosely(t, ces, should.HaveLength(2))
			assert.Loosely(t, ces[0].Timestamp, should.Match(desc.Timestamp.AsTime().Add(1*time.Second)))
			assert.Loosely(t, ces[1].Timestamp, should.Match(desc.Timestamp.AsTime().Add(2*time.Second)))
		})

		t.Run("Sets the trace with the stream ID", func(t *ftt.Test) {
			ces := toCLEs(
				genEntry("line-123\n"),
				genEntry("line-456\n"),
			)
			for _, e := range ces {
				assert.Loosely(t, e.Trace, should.Match(eb.streamID))
			}
		})

		t.Run("Sets entries with unique InsertIDs", func(t *ftt.Test) {
			ces := toCLEs(
				genEntry("line-1\n", "line-2\n", "line-3\n"),
				genEntry("line-4\n"),
			)
			for i, e := range ces {
				assert.Loosely(t, e.InsertID, should.Match(fmt.Sprintf("%s/%d", eb.streamID, i)))
			}
		})

		t.Run("Handles empty lines", func(t *ftt.Test) {
			ces := toCLEs(
				genEntry("\n"),
				genEntry("\n"),
				genEntry(""),
			)
			checkPayloads(ces) // len(ces) should be 0.

			ces = toCLEs(
				genEntry("\n", "\n"),
				genEntry(""),
			)
			checkPayloads(ces) // len(ces) should be 0.

			ces = toCLEs(
				genEntry("line\n", "\n", ""),
				genEntry("\n"),
			)
			checkPayloads(ces, "line")
		})

		t.Run("Merges lines without a trailing delimiter", func(t *ftt.Test) {
			// tests with complete lines.
			ces := toCLEs(
				genEntry("line-1\n", "line-2\n"),
				genEntry("line-3\n"),
			)
			checkPayloads(ces, "line-1\nline-2", "line-3")

			ces = toCLEs(
				genEntry("line-1\n", "line-2\n", "line-3\n"),
				genEntry("line-4\n"),
			)
			checkPayloads(ces, "line-1\nline-2", "line-3", "line-4")

			// tests with partial lines
			ces = toCLEs(
				genEntry("this"),
			)
			checkPayloads(ces, "this")

			ces = toCLEs(
				genEntry("this "),
				genEntry("is"),
			)
			checkPayloads(ces, "this is")

			ces = toCLEs(
				genEntry("this "),
				genEntry("is "),
				genEntry("a line"),
			)
			checkPayloads(ces, "this is a line")

			// tests with a mix of both
			ces = toCLEs(
				genEntry("this "),
				genEntry("is "),
				genEntry("a line\n"),
			)
			checkPayloads(ces, "this is a line")

			ces = toCLEs(
				genEntry("line-1\n", "line-2\n", "this is "),
				genEntry("a line\n", "another "),
				genEntry("line?\n"),
			)
			checkPayloads(ces, "line-1\nline-2", "this is a line", "another line?")

			ces = toCLEs(
				genEntry("it\n", "has "),
				genEntry("all\n", "the "),
				genEntry("lines\n"),
			)
			checkPayloads(ces, "it", "has all", "the lines")
		})

		t.Run("Truncates lines", func(t *ftt.Test) {
			// tests with complete lines
			ces := toCLEs(
				genEntry("this is tooooooooo long\n"),
			)
			checkPayloads(ces, "this is toooooo")

			ces = toCLEs(
				genEntry("this is\n", "short\n", "this is long enough"),
			)
			checkPayloads(ces, "this is\nshort", "this is long en")

			// tests with partial lines
			ces = toCLEs(
				genEntry("this is going "),
				genEntry("to be a long line, "),
				genEntry("is it?"),
			)
			checkPayloads(ces, "this is going t")

			// tests with a mix of both
			ces = toCLEs(
				genEntry("this is going "),
				genEntry("to be a long line\n", "this is also long\n"),
			)
			checkPayloads(ces, "this is going t", "this is also lo")
		})
	})
}
