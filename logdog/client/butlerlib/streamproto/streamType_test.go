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

package streamproto

import (
	"encoding/json"
	"flag"
	"fmt"
	"testing"

	"github.com/luci/luci-go/logdog/api/logpb"
	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamType(t *testing.T) {
	Convey(`A StreamType flag`, t, func() {
		value := StreamType(0)

		fs := flag.NewFlagSet("Testing", flag.ContinueOnError)
		fs.Var(&value, "stream-type", "StreamType test.")

		Convey(`Can be loaded as a flag.`, func() {
			err := fs.Parse([]string{"-stream-type", "datagram"})
			So(err, ShouldBeNil)
			So(value, ShouldEqual, logpb.StreamType_DATAGRAM)
		})

		Convey(`Will unmmarshal from JSON.`, func() {
			var s struct {
				Value StreamType `json:"value"`
			}

			err := json.Unmarshal([]byte(`{"value": "text"}`), &s)
			So(err, ShouldBeNil)
			So(s.Value, ShouldEqual, logpb.StreamType_TEXT)
		})

		Convey(`Will marshal to JSON.`, func() {
			var s struct {
				Value StreamType `json:"value"`
			}
			s.Value = StreamType(logpb.StreamType_BINARY)

			v, err := json.Marshal(&s)
			So(err, ShouldBeNil)
			So(string(v), ShouldResemble, `{"value":"binary"}`)
		})

		for _, t := range []logpb.StreamType{
			logpb.StreamType_TEXT,
			logpb.StreamType_BINARY,
			logpb.StreamType_DATAGRAM,
		} {
			Convey(fmt.Sprintf(`Stream type [%s] has a default content type.`, t), func() {
				st := StreamType(t)
				So(st.DefaultContentType(), ShouldNotEqual, "")
			})
		}
	})
}
