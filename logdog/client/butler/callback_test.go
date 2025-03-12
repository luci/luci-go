// Copyright 2019 The LUCI Authors.
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
	"context"
	"io"
	"testing"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/output/null"
	"go.chromium.org/luci/logdog/common/types"
)

func TestButlerCallbacks(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing Butler instance`, t, func(t *ftt.Test) {
		c := gologger.StdConfig.Use(context.Background())
		b, err := New(c, Config{
			Output:     &null.Output{},
			BufferLogs: false,
		})
		assert.Loosely(t, err, should.BeNil)

		defer func() {
		}()

		var streamLines []string
		b.AddStreamRegistrationCallback(func(d *logpb.LogStreamDescriptor) StreamChunkCallback {
			if d.Name != "target_stream" {
				return nil
			}
			return func(e *logpb.LogEntry) {
				if e == nil {
					streamLines = append(streamLines, "<EOF>")
					return
				}
				for _, line := range e.GetText().GetLines() {
					streamLines = append(streamLines, string(line.Value)+line.Delimiter)
				}
			}
		}, true)

		newTestStream := func(name string) *testStream {
			desc := &logpb.LogStreamDescriptor{
				Name:        name,
				StreamType:  logpb.StreamType_TEXT,
				ContentType: string(types.ContentTypeText),
			}
			return &testStream{
				inC:     make(chan *testStreamData, 16),
				closedC: make(chan struct{}),
				desc:    desc,
			}
		}

		t.Run(`ignores unrelated streams`, func(t *ftt.Test) {
			s := newTestStream("test")
			assert.Loosely(t, b.AddStream(s, s.desc), should.BeNil)
			s.data([]byte("Hello!\n"), nil)
			s.data([]byte("This\nis\na\ntest."), nil)
			s.data(nil, io.EOF)
			assert.Loosely(t, streamLines, should.BeNil)
			b.Activate()
			assert.Loosely(t, b.Wait(), should.BeNil)
		})

		t.Run(`is called for target_stream`, func(t *ftt.Test) {
			s := newTestStream("target_stream")
			assert.Loosely(t, b.AddStream(s, s.desc), should.BeNil)
			s.data([]byte("Hello!\n"), nil)
			s.data([]byte("This\nis\na\ntest."), nil)
			s.data(nil, io.EOF)
			b.Activate()
			assert.Loosely(t, b.Wait(), should.BeNil)

			assert.Loosely(t, streamLines, should.Match([]string{
				"Hello!\n", "This\n", "is\n", "a\n", "test.", "<EOF>",
			}))
		})

		t.Run(`is called for target_stream (without data)`, func(t *ftt.Test) {
			s := newTestStream("target_stream")
			assert.Loosely(t, b.AddStream(s, s.desc), should.BeNil)
			s.data(nil, io.EOF)
			b.Activate()
			assert.Loosely(t, b.Wait(), should.BeNil)

			assert.Loosely(t, streamLines, should.Match([]string{"<EOF>"}))
		})
	})
}
