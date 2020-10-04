// Copyright 2020 The LUCI Authors.
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

package history

import (
	"bytes"
	"io"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReaderWriter(t *testing.T) {
	t.Parallel()
	Convey(`ReaderWriter`, t, func() {
		buf := bytes.NewBuffer(nil)

		records := []*evalpb.Record{
			parseRecord("rejection { timestamp { seconds: 1 } }"),
			parseRecord("rejection { timestamp { seconds: 2 } }"),
			parseRecord("rejection { timestamp { seconds: 3 } }"),
		}

		// Write the records.
		w := NewWriter(buf)
		for _, r := range records {
			So(w.Write(r), ShouldBeNil)
		}
		So(w.Close(), ShouldBeNil)

		// Read the records back.
		r := NewReader(buf)
		defer r.Close()
		for _, expected := range records {
			actual, err := r.Read()
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, expected)
		}
		_, err := r.Read()
		So(err, ShouldEqual, io.EOF)
	})
}

func parseRecord(text string) *evalpb.Record {
	rec := &evalpb.Record{}
	err := prototext.Unmarshal([]byte(text), rec)
	So(err, ShouldBeNil)
	return rec
}
