// Copyright 2025 The LUCI Authors.
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

package debugstack

import (
	"bytes"
	"iter"
	"strings"
)

// Trace is a linear collection of Frames.
//
// You can retrieve the identical bytes passed to Parse by concatenating the
// Lines in the Frames contained here (which is what [Trace.String] does).
//
// Traces can also be filtered with [Trace.Filter] to remove frames for certain
// packages, remove some frames from the top, etc.
type Trace []Frame

// Stats returns a mapping of FrameKind to the number of times that kind of
// frame appears in the Trace.
func (t Trace) Stats() map[FrameKind]int {
	if len(t) == 0 {
		return nil
	}
	stats := map[FrameKind]int{}
	for _, frame := range t {
		stats[frame.Kind]++
	}
	return stats
}

func (t Trace) String() string {
	bld := strings.Builder{}
	for _, frame := range t {
		for _, line := range frame.Lines {
			bld.WriteString(line)
		}
	}
	return bld.String()
}

func parseImpl(sIterNoNewline iter.Seq[string]) Trace {
	next, stop := iter.Pull(sIterNoNewline)
	defer stop()

	ret := Trace{}
	for {
		line, ok := next()
		if !ok {
			break
		}

		// Figure out what is the likely config to use. Since normal stack frames
		// don't have any consistent prefix, that's our default guess.
		cfg := stackFrameKindConfig

		// Check if the line is one of our 'special' frame types.
		if possibleConfig, ok := firstLetterPrefixMap[line[0]]; ok {
			if strings.HasPrefix(line, possibleConfig.firstLinePrefix) {
				cfg = possibleConfig
			} else if possibleConfig.fallbackToStackFrame {
				cfg = stackFrameKindConfig
			} else {
				cfg = nil
			}
		}

		// Now, gather any extra lines the proposed config needs
		lines := []string{line}
		if cfg != nil {
			parse := cfg.parse
			if cfg.hasSource {
				if sourceLine, ok := next(); ok {
					// we treat source line as optional in the event of a truncated trace.
					lines = append(lines, sourceLine)
				}
			}

			// If parse is not-nil, run it and see if the parse is ok
			if parse != nil {
				if frame, ok := parse(cfg.frameKind, lines); ok {
					ret = append(ret, frame)
					continue
				}
			}
		}

		// cfg or parse was nil (iterator didn't yield enough lines) or the parse
		// failed, so, mark all accumulated lines as unknown frames.
		for _, line := range lines {
			ret = append(ret, Frame{UnknownFrameKind, "", "", []string{line}})
		}
	}

	return ret
}

// Parse expects a single stack trace from e.g. debug.Stack(), and will return
// the parsed Trace (as best as can be determined).
//
// Passing garbage in here will just get you a Trace with UnknownFrameKind
// frames, one per line.
//
// The original data `trace` can be obtained by calling [Trace.String].
func Parse(trace []byte) Trace {
	return parseImpl(func(yield func(string) bool) {
		for len(trace) > 0 {
			nlIdx := bytes.IndexByte(trace, '\n')
			if nlIdx < 0 {
				if len(trace) > 0 {
					yield(string(trace))
				}
				return
			}
			if !yield(string(trace[:nlIdx+1])) {
				return
			}
			trace = trace[nlIdx+1:]
		}
	})
}

// ParseString is just the string variant of Parse, in case you are consuming
// a `string` stack trace..
func ParseString(trace string) Trace {
	return parseImpl(func(yield func(string) bool) {
		for len(trace) > 0 {
			nlIdx := strings.IndexByte(trace, '\n')
			if nlIdx < 0 {
				if len(trace) > 0 {
					yield(trace)
				}
				return
			}
			if !yield(trace[:nlIdx+1]) {
				return
			}
			trace = trace[nlIdx+1:]
		}
	})
}
