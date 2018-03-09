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

package cli

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHumanReadable(t *testing.T) {
	t.Parallel()

	Convey("mixed length", t, func() {
		b := bytes.Buffer{}
		p := newHumanReadable(&b)
		p.Row("A1-really-long-value", "B1")
		p.Row("A2", "B2")
		p.Flush()
		So(b.String(), ShouldEqual, "A1-really-long-value    B1\nA2                      B2\n")
	})

	Convey("ok", t, func() {
		b := bytes.Buffer{}
		p := newHumanReadable(&b)
		p.Row("A1", "B1", "C1")
		p.Row("A2", "B2", "C2")
		p.Flush()
		So(b.String(), ShouldEqual, "A1    B1    C1\nA2    B2    C2\n")
	})
}

func TestMachineReadable(t *testing.T) {
	t.Parallel()

	Convey("mixed length", t, func() {
		b := bytes.Buffer{}
		p := newMachineReadable(&b)
		p.Row("A1-really-long-value", "B1")
		p.Row("A2", "B2")
		p.Flush()
		So(b.String(), ShouldEqual, "A1-really-long-value\tB1\nA2\tB2\n")
	})

	Convey("ok", t, func() {
		b := bytes.Buffer{}
		p := newMachineReadable(&b)
		p.Row("A1", "B1", "C1")
		p.Row("A2", "B2", "C2")
		p.Flush()
		So(b.String(), ShouldEqual, "A1\tB1\tC1\nA2\tB2\tC2\n")
	})
}
