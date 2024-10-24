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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseStatsdMetric(t *testing.T) {
	t.Parallel()

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		roundTrip := func(line string) bool {
			m := &StatsdMetric{}
			read, err := ParseStatsdMetric([]byte(line), m)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, read, should.Equal(len(line)))

			// There should be no special chars in parsed portions.
			for _, n := range m.Name {
				assert.Loosely(t, n, should.NotContain[byte]('.'))
				assert.Loosely(t, n, should.NotContain[byte](':'))
				assert.Loosely(t, n, should.NotContain[byte]('|'))
			}
			assert.Loosely(t, m.Value, should.NotContain[byte]('.'))
			assert.Loosely(t, m.Value, should.NotContain[byte](':'))
			assert.Loosely(t, m.Value, should.NotContain[byte]('|'))

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

		assert.Loosely(t, roundTrip("a.bcd.xyz:123|g"), should.BeTrue)
		assert.Loosely(t, roundTrip("a.b.c:123|c"), should.BeTrue)
		assert.Loosely(t, roundTrip("a.b.c:123|ms"), should.BeTrue)
		assert.Loosely(t, roundTrip("a:123|g"), should.BeTrue)
	})

	ftt.Run("Parses one line only", t, func(t *ftt.Test) {
		buf := []byte("a:123|g\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, buf[read:], should.Resemble([]byte("stuff")))

		assert.Loosely(t, m, should.Resemble(&StatsdMetric{
			Name:  [][]byte{{'a'}},
			Type:  StatsdMetricGauge,
			Value: []byte("123"),
		}))
	})

	ftt.Run("Skips junk", t, func(t *ftt.Test) {
		buf := []byte("a:123|g|skipped junk\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, buf[read:], should.Resemble([]byte("stuff")))

		assert.Loosely(t, m, should.Resemble(&StatsdMetric{
			Name:  [][]byte{{'a'}},
			Type:  StatsdMetricGauge,
			Value: []byte("123"),
		}))
	})

	ftt.Run("ErrMalformedStatsdLine", t, func(t *ftt.Test) {
		buf := []byte("a:b:c\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		assert.Loosely(t, err, should.Equal(ErrMalformedStatsdLine))
		assert.Loosely(t, buf[read:], should.Resemble([]byte("stuff")))
	})

	ftt.Run("ErrMalformedStatsdLine on empty line", t, func(t *ftt.Test) {
		buf := []byte("\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		assert.Loosely(t, err, should.Equal(ErrMalformedStatsdLine))
		assert.Loosely(t, buf[read:], should.Resemble([]byte("stuff")))
	})

	ftt.Run("ErrMalformedStatsdLine on empty name component", t, func(t *ftt.Test) {
		call := func(pat string) error {
			buf := []byte(pat + "\nother stuff")
			read, err := ParseStatsdMetric(buf, &StatsdMetric{})
			assert.Loosely(t, buf[read:], should.Resemble([]byte("other stuff")))
			return err
		}

		assert.Loosely(t, call("ab..cd:123|g"), should.Equal(ErrMalformedStatsdLine))
		assert.Loosely(t, call("a.:123|g"), should.Equal(ErrMalformedStatsdLine))
		assert.Loosely(t, call(".a:123|g"), should.Equal(ErrMalformedStatsdLine))
		assert.Loosely(t, call(".:123|g"), should.Equal(ErrMalformedStatsdLine))
		assert.Loosely(t, call(":|g"), should.Equal(ErrMalformedStatsdLine))
	})

	ftt.Run("ErrUnsupportedType", t, func(t *ftt.Test) {
		buf := []byte("a:123|h\nstuff")
		m := &StatsdMetric{}
		read, err := ParseStatsdMetric(buf, m)
		assert.Loosely(t, err, should.Equal(ErrUnsupportedType))
		assert.Loosely(t, buf[read:], should.Resemble([]byte("stuff")))
	})
}
