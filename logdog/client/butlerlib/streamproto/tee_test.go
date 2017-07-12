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
	"math"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// Test the TeeType struct.
func TestTeeType(t *testing.T) {
	Convey(`A TeeNone type should return a nil writer`, t, func() {
		So(TeeNone.Writer(), ShouldBeNil)
	})

	Convey(`A TeeStdout type should return STDOUT file.`, t, func() {
		So(TeeStdout.Writer(), ShouldEqual, os.Stdout)
	})

	Convey(`A TeeStderr type should return STDERR file.`, t, func() {
		So(TeeStderr.Writer(), ShouldEqual, os.Stderr)
	})

	Convey(`An invalid value should panic.`, t, func() {
		So(func() { TeeType(math.MaxUint32).Writer() }, ShouldPanic)
	})
}

func TestTeeTypeFlag(t *testing.T) {
	Convey(`A TeeType flag`, t, func() {
		var value TeeType

		fs := flag.NewFlagSet("Testing", flag.ContinueOnError)
		fs.Var(&value, "tee_type", "TeeType test.")

		Convey(`Can be loaded as a flag.`, func() {
			err := fs.Parse([]string{"-tee_type", "stdout"})
			So(err, ShouldBeNil)
			So(value, ShouldEqual, TeeStdout)
		})

		Convey(`Will unmmarshal from JSON.`, func() {
			var s struct {
				Value TeeType `json:"value"`
			}

			err := json.Unmarshal([]byte(`{"value":"stderr"}`), &s)
			So(err, ShouldBeNil)
			So(s.Value, ShouldEqual, TeeStderr)
		})

		Convey(`Will marshal to JSON.`, func() {
			var s struct {
				Value TeeType `json:"value"`
			}
			s.Value = TeeNone

			v, err := json.Marshal(&s)
			So(err, ShouldBeNil)
			So(string(v), ShouldEqual, `{"value":"none"}`)
		})
	})
}
