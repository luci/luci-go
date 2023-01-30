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
	"go.chromium.org/luci/common/errors"
)

var (
	// ErrMalformedStatsdLine is returned by ParseStatsdMetric if it receives
	// an input that doesn't look like a statsd metric line.
	ErrMalformedStatsdLine = errors.New("statsd line has wrong format")

	// ErrUnsupportedType is returned by ParseStatsdMetric if the statsd metric
	// has a type we don't support.
	ErrUnsupportedType = errors.New("unsupported metric type")
)

// StatsdMetricType enumerates supports statsd metric types.
type StatsdMetricType int

const (
	StatsdMetricUnknown StatsdMetricType = 0
	StatsdMetricGauge   StatsdMetricType = 1 // 'g'
	StatsdMetricCounter StatsdMetricType = 2 // 'c'
	StatsdMetricTimer   StatsdMetricType = 3 // 'ms'
)

// StatsdMetric holds some parsed statsd metric.
//
// It retains pointers to the buffer it was parsed from.
//
// A metric line "a.b.c:1234|c" is represented by
//
//	m := StatsdMetric{
//	  Name: [][]byte{
//	    {'a'},
//	    {'b'},
//	    {'c'},
//	  },
//	  Type: StatsdMetricCounter,
//	  Value: []byte("1234"),
//	}
type StatsdMetric struct {
	Name  [][]byte
	Type  StatsdMetricType
	Value []byte
}

// ParseStatsdMetric parses one statsd metric line from the buffer.
//
// Returns the number of bytes read and the parsed metric. The parsed metric
// retains pointers to the `buf`. Make sure `buf` is not modified as long as
// there are StatsdMetric that point to it.
func ParseStatsdMetric(buf []byte, metric *StatsdMetric) (read int, err error) {
	// Reset the state (but retain Name buffer).
	metric.Name = metric.Name[:0]
	metric.Type = StatsdMetricUnknown
	metric.Value = nil

	// Buf contains one or more lines like:
	//
	// xxx.yyy.zzz:12345|c
	// xxx.yyy.zzz:12345|c|<ignore>
	// xxx.yyy.zzz:12345|c#ignore
	//
	// We parse only the first line using a simple state machine. That way we can
	// avoid unnecessary memory allocations on a hot code path.
	const (
		S_NAME        = iota // in 'xxx'
		S_VALUE              // in '1234'
		S_TYPE               // in 'c'
		S_SKIP               // waiting for \n or EOF
		S_SKIP_BROKEN        // waiting for \n or EOF
	)
	state := S_NAME

	nameIdx := 0       // index where the name component started
	valueIdx := 0      // index where the value portion started
	typIdx := 0        // index where the type portion started
	typ := []byte(nil) // the type portion

	idx := 0       // the current scanning pointer
	chr := byte(0) // the current character

	for idx, chr = range buf {
		switch {
		case state == S_NAME && (chr == '.' || chr == ':'):
			// This happens when parsing e.g. "abc..def".
			if nameIdx == idx {
				state = S_SKIP_BROKEN
				continue
			}
			metric.Name = append(metric.Name, buf[nameIdx:idx])
			if chr == '.' {
				nameIdx = idx + 1
				state = S_NAME // keep reading the name
			} else { // ':'
				valueIdx = idx + 1
				state = S_VALUE // reading the value now
			}

		case state == S_VALUE && chr == '|':
			metric.Value = buf[valueIdx:idx]
			typIdx = idx + 1
			state = S_TYPE

		case state == S_TYPE && (chr == '|' || chr == '#' || chr == '\n'):
			typ = buf[typIdx:idx]
			state = S_SKIP
		}

		if chr == '\n' {
			break
		}
	}
	read = idx + 1

	// S_TYPE transitions into S_SKIP on EOF/EOL.
	if state == S_TYPE {
		typ = buf[typIdx : idx+1]
		state = S_SKIP
	}

	// S_SKIP is the only valid state on EOF/EOL.
	if state != S_SKIP {
		err = ErrMalformedStatsdLine
	} else {
		// Parse `typ`.
		switch {
		case len(typ) == 1 && typ[0] == 'g':
			metric.Type = StatsdMetricGauge
		case len(typ) == 1 && typ[0] == 'c':
			metric.Type = StatsdMetricCounter
		case len(typ) == 2 && typ[0] == 'm' && typ[1] == 's':
			metric.Type = StatsdMetricTimer
		default:
			err = ErrUnsupportedType
		}
	}

	return
}
