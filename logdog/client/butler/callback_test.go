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
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/output/null"
	"go.chromium.org/luci/logdog/common/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestButlerCallbacks(t *testing.T) {
	t.Parallel()

	Convey(`A testing Butler instance`, t, func() {
		c := gologger.StdConfig.Use(context.Background())
		b, err := New(c, Config{
			Output:     &null.Output{},
			BufferLogs: false,
		})
		So(err, ShouldBeNil)

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
				for _, line := range append(e.GetText().Lines) {
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

		Convey(`ignores unrelated streams`, func() {
			s := newTestStream("test")
			So(b.AddStream(s, s.desc), ShouldBeNil)
			s.data([]byte("Hello!\n"), nil)
			s.data([]byte("This\nis\na\ntest."), nil)
			s.data(nil, io.EOF)
			So(streamLines, ShouldBeNil)
			b.Activate()
			So(b.Wait(), ShouldBeNil)
		})

		Convey(`is called for target_stream`, func() {
			s := newTestStream("target_stream")
			So(b.AddStream(s, s.desc), ShouldBeNil)
			s.data([]byte("Hello!\n"), nil)
			s.data([]byte("This\nis\na\ntest."), nil)
			s.data(nil, io.EOF)
			b.Activate()
			So(b.Wait(), ShouldBeNil)

			So(streamLines, ShouldResemble, []string{
				"Hello!\n", "This\n", "is\n", "a\n", "test.", "<EOF>",
			})
		})

		Convey(`is called for target_stream (without data)`, func() {
			s := newTestStream("target_stream")
			So(b.AddStream(s, s.desc), ShouldBeNil)
			s.data(nil, io.EOF)
			b.Activate()
			So(b.Wait(), ShouldBeNil)

			So(streamLines, ShouldResemble, []string{"<EOF>"})
		})
	})
}
