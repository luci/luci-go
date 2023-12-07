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

package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/smarty/assertions"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExternalStorage(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := memlogger.Use(context.Background())
		log := logging.Get(ctx).(*memlogger.MemLogger)

		s := ExternalStorage{}

		// Initially empty.
		b, _, err := s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(b.Values, ShouldBeNil)

		// Added some.
		So(s.Load(ctx, strings.NewReader(`{"k1":"a", "k2": "b", "k3": "c"}`)), ShouldBeNil)
		So(lastLogJSON(log), assertions.ShouldEqualJSON, `{
			"added": {"k1":"a", "k2": "b", "k3": "c"},
			"changed": {},
			"removed": {}
		}`)
		log.Reset()

		b, _, err = s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(b.Values, ShouldResemble, map[string]*json.RawMessage{
			"k1": {34, 97, 34}, // "a"
			"k2": {34, 98, 34}, // "b"
			"k3": {34, 99, 34}, // "c"
		})

		// Removed some and mutated some.
		So(s.Load(ctx, strings.NewReader(`{"k1":"d", "k3": "c"}`)), ShouldBeNil)
		So(lastLogJSON(log), assertions.ShouldEqualJSON, `{
			"added": {},
			"changed": {"k1": {"old": "a", "new": "d"}},
			"removed": {"k2": "b"}
		}`)
		log.Reset()

		b, _, err = s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(b.Values, ShouldResemble, map[string]*json.RawMessage{
			"k1": {34, 100, 34}, // "d"
			"k3": {34, 99, 34},  // "c"
		})

		// Errors do not change settings.
		So(s.Load(ctx, strings.NewReader("???")), ShouldNotBeNil)
		b, _, err = s.FetchAllSettings(ctx)
		So(err, ShouldBeNil)
		So(b.Values, ShouldResemble, map[string]*json.RawMessage{
			"k1": {34, 100, 34}, // "d"
			"k3": {34, 99, 34},  // "c"
		})
	})
}

// lastLogJSON returns JSON-serialized 'data' portion of the last log entry.
func lastLogJSON(log *memlogger.MemLogger) string {
	m := log.Messages()
	if len(m) == 0 {
		return ""
	}
	blob, err := json.MarshalIndent(m[len(m)-1].Data, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(blob)
}
