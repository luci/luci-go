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

package main

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseStatsdMetric(t *testing.T) {
	t.Parallel()

	Convey("Happy path", t, func() {
		roundTrip := func(line string) bool {
			m := &StatsdMetric{}
			read, err := ParseStatsdMetric([]byte(line), m)
			So(err, ShouldBeNil)
			So(read, ShouldEqual, len(line))

			// There should be no special chars in parsed portions.
			for _, n := range m.Name {
				So(n, ShouldNotContain, '.')
				So(n, ShouldNotContain, ':')
				So(n, ShouldNotContain, '|')
			}
			So(m.Value, ShouldNotContain, '.')
			So(m.Value, ShouldNotContain, ':')
			So(m.Value, ShouldNotContain, '|')

			// Reconstruct the original line from parsed components.
			buf := bytes.Join(m.Name, []byte{'.'})
			buf = append(buf, ':')
			buf = append(buf, m.Value...)
			buf = append(buf, '|')
			switch m.Type {
			case StatsdMetricGauge:
				buf = append(buf, 'g')
			case StatsdMetricCounter:
				buf = append(buf, 'c')
			case StatsdMetricTimer:
				buf = append(buf, 'm', 's')
			}

			return string(buf) == line
		}

		So(roundTrip("a.bcd.xyz:123|g"), ShouldBeTrue)
		So(roundTrip("a.b.c:123|c"), ShouldBeTrue)
		So(roundTrip("a.b.c:123|ms"), ShouldBeTrue)
		So(roundTrip("a:123|g"), ShouldBeTrue)
	})

	Convey("Parses one line only", t, func() {
		buf := []byte("a:123|g\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		So(err, ShouldBeNil)
		So(buf[read:], ShouldResemble, []byte("stuff"))

		So(m, ShouldResemble, &StatsdMetric{
			Name:  [][]byte{{'a'}},
			Type:  StatsdMetricGauge,
			Value: []byte("123"),
		})
	})

	Convey("Skips junk", t, func() {
		buf := []byte("a:123|g|skipped junk\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		So(err, ShouldBeNil)
		So(buf[read:], ShouldResemble, []byte("stuff"))

		So(m, ShouldResemble, &StatsdMetric{
			Name:  [][]byte{{'a'}},
			Type:  StatsdMetricGauge,
			Value: []byte("123"),
		})
	})

	Convey("ErrMalformedStatsdLine", t, func() {
		buf := []byte("a:b:c\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		So(err, ShouldEqual, ErrMalformedStatsdLine)
		So(buf[read:], ShouldResemble, []byte("stuff"))
	})

	Convey("ErrMalformedStatsdLine on empty line", t, func() {
		buf := []byte("\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		So(err, ShouldEqual, ErrMalformedStatsdLine)
		So(buf[read:], ShouldResemble, []byte("stuff"))
	})

	Convey("ErrMalformedStatsdLine on empty name component", t, func() {
		call := func(pat string) error {
			buf := []byte(pat + "\nother stuff")
			read, err := ParseStatsdMetric(buf, &StatsdMetric{})
			So(buf[read:], ShouldResemble, []byte("other stuff"))
			return err
		}

		So(call("ab..cd:123|g"), ShouldEqual, ErrMalformedStatsdLine)
		So(call("a.:123|g"), ShouldEqual, ErrMalformedStatsdLine)
		So(call(".a:123|g"), ShouldEqual, ErrMalformedStatsdLine)
		So(call(".:123|g"), ShouldEqual, ErrMalformedStatsdLine)
		So(call(":|g"), ShouldEqual, ErrMalformedStatsdLine)
	})

	Convey("ErrUnsupportedType", t, func() {
		buf := []byte("a:123|h\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		So(err, ShouldEqual, ErrUnsupportedType)
		So(buf[read:], ShouldResemble, []byte("stuff"))
	})
}
