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

package bundler

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
)

func parse(desc string) (*logpb.ButlerLogBundle_Entry, []*logpb.LogEntry) {
	comp := strings.Split(desc, ":")
	name, entries := comp[0], comp[1:]

	be := &logpb.ButlerLogBundle_Entry{
		Desc: &logpb.LogStreamDescriptor{
			Name: name,
		},
	}

	logs := make([]*logpb.LogEntry, len(entries))
	for idx, l := range entries {
		comp := strings.SplitN(l, "@", 2)
		key, size := comp[0], 0
		if len(comp) == 2 {
			size, _ = strconv.Atoi(comp[1])
		}

		le := &logpb.LogEntry{
			Content: &logpb.LogEntry_Text{Text: &logpb.Text{
				Lines: []*logpb.Text_Line{
					{Value: []byte(key)},
				},
			}},
		}

		// Pad missing data, if requested.
		if size > 0 {
			missing := size - protoSize(le)
			if missing > 0 {
				le.GetText().Lines = append(le.GetText().Lines, &logpb.Text_Line{
					Value: []byte(strings.Repeat("!", missing)),
				})
			}
		}
		logs[idx] = le
	}
	return be, logs
}

func logEntryName(le *logpb.LogEntry) string {
	t := le.GetText()
	if t == nil || len(t.Lines) == 0 {
		return ""
	}
	return string(t.Lines[0].Value)
}

// "expected" is a notation to express a bundle entry and its keys:
//
//	"a": a bundle entry keyed on "a".
//	"+a": a terminal bundle entry keyed on "a".
//	"a:1:2:3": a bundle entry keyed on "a" with three log entries, each keyed on
//	         "1", "2", and "3" respectively.
func shouldHaveBundleEntries(expected ...string) comparison.Func[*logpb.ButlerLogBundle] {
	return func(actual *logpb.ButlerLogBundle) *failure.Summary {
		var errors []string
		fail := func(f string, args ...any) {
			errors = append(errors, fmt.Sprintf(f, args...))
		}

		term := make(map[string]bool)
		exp := make(map[string][]string)

		// Parse expectation strings.
		for _, s := range expected {
			if len(s) == 0 {
				continue
			}

			t := false
			if s[0] == '+' {
				t = true
				s = s[1:]
			}

			parts := strings.Split(s, ":")
			name := parts[0]
			term[name] = t

			if len(parts) > 1 {
				exp[name] = append(exp[name], parts[1:]...)
			}
		}

		entries := make(map[string]*logpb.ButlerLogBundle_Entry)
		for _, be := range actual.Entries {
			entries[be.Desc.Name] = be
		}
		for name, t := range term {
			be := entries[name]
			if be == nil {
				fail("No bundle entry for [%s]", name)
				continue
			}
			delete(entries, name)

			if t != be.Terminal {
				fail("Bundle entry [%s] doesn't match expected terminal state (exp: %v != act: %v)",
					name, t, be.Terminal)
			}

			logs := exp[name]
			for i, l := range logs {
				if i >= len(be.Logs) {
					fail("Bundle entry [%s] missing log: %s", name, l)
					continue
				}
				le := be.Logs[i]

				if logEntryName(le) != l {
					fail("Bundle entry [%s] log %d doesn't match expected (exp: %s != act: %s)",
						name, i, l, logEntryName(le))
					continue
				}
			}
			if len(be.Logs) > len(logs) {
				for _, le := range be.Logs[len(logs):] {
					fail("Bundle entry [%s] has extra log entry: %s", name, logEntryName(le))
				}
			}
		}
		for k := range entries {
			fail("Unexpected bundle entry present: [%s]", k)
		}
		if len(errors) == 0 {
			return nil
		}
		ret := comparison.NewSummaryBuilder("shouldHaveBundleEntries").
			Because("Encountered one or more errors")
		ret.Findings = append(ret.Findings, &failure.Finding{
			Name:  "Errors",
			Value: errors,
			Level: failure.FindingLogLevel_Error,
		})
		return ret.Summary
	}
}

func TestBuilder(t *testing.T) {
	ftt.Run(`A builder`, t, func(t *ftt.Test) {
		tc := testclock.New(time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		b := &builder{
			template: logpb.ButlerLogBundle{
				Timestamp: timestamppb.New(tc.Now()),
			},
		}
		templateSize := protoSize(&b.template)

		t.Run(`Is not ready by default, and has no content.`, func(t *ftt.Test) {
			b.size = templateSize + 1
			assert.Loosely(t, b.ready(), should.BeFalse)
			assert.Loosely(t, b.hasContent(), should.BeFalse)

			t.Run(`When exceeding the desired size with content, is ready.`, func(t *ftt.Test) {
				be, _ := parse("a")
				b.size = 1
				b.setStreamTerminal(be, 0)
				assert.Loosely(t, b.ready(), should.BeTrue)
			})
		})

		t.Run(`Has a bundleSize() and remaining value of the template.`, func(t *ftt.Test) {
			b.size = 1024

			assert.Loosely(t, b.bundleSize(), should.Equal(templateSize))
			assert.Loosely(t, b.remaining(), should.Equal(1024-templateSize))
		})

		t.Run(`With a size of 1024 and a 512-byte LogEntry, has content, but is not ready.`, func(t *ftt.Test) {
			b.size = 1024
			be, logs := parse("a:1@512")
			b.add(be, logs[0])
			assert.Loosely(t, b.hasContent(), should.BeTrue)
			assert.Loosely(t, b.ready(), should.BeFalse)

			t.Run(`After adding another 512-byte LogEntry, is ready.`, func(t *ftt.Test) {
				be, logs := parse("a:2@512")
				b.add(be, logs[0])
				assert.Loosely(t, b.ready(), should.BeTrue)
			})
		})

		t.Run(`Has content after adding a terminal entry.`, func(t *ftt.Test) {
			assert.Loosely(t, b.hasContent(), should.BeFalse)
			be, _ := parse("a")
			b.setStreamTerminal(be, 1024)
			assert.Loosely(t, b.hasContent(), should.BeTrue)
		})

		for _, test := range []struct {
			title string

			streams  []string
			terminal bool
			expected []string
		}{
			{`Empty terminal entry`,
				[]string{"a"}, true, []string{"+a"}},
			{`Single non-terminal entry`,
				[]string{"a:1"}, false, []string{"a:1"}},
			{`Multiple non-terminal entries`,
				[]string{"a:1:2:3:4"}, false, []string{"a:1:2:3:4"}},
			{`Single large entry`,
				[]string{"a:1@1024"}, false, []string{"a:1"}},
			{`Multiple terminal streams.`,
				[]string{"a:1", "b:1", "a:2", "c:1"}, true, []string{"+a:1:2", "+b:1", "+c:1"}},
			{`Multiple large non-terminal streams.`,
				[]string{"a:1@1024", "b:1@8192", "a:2@4096", "c:1"}, false, []string{"a:1:2", "b:1", "c:1"}},
		} {
			t.Run(fmt.Sprintf(`Test Case: %q`, test.title), func(t *ftt.Test) {
				for _, s := range test.streams {
					be, logs := parse(s)
					for _, le := range logs {
						b.add(be, le)
					}

					if test.terminal {
						b.setStreamTerminal(be, 1)
					}
				}

				t.Run(`Constructed bundle matches expected.`, func(t *ftt.Test) {
					assert.Loosely(t, b.bundle(), shouldHaveBundleEntries(test.expected...))
				})

				t.Run(`Calculated size matches actual.`, func(t *ftt.Test) {
					assert.Loosely(t, b.bundleSize(), should.Equal(protoSize(b.bundle())))
				})
			})
		}
	})
}
