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
	"iter"
	"regexp"
	"strconv"
	"strings"
)

//go:generate stringer -type FrameKind
type FrameKind int

const (
	// UnknownFrameKind is used for lines which don't parse
	// into any of the other kinds.
	//
	// These will always contain a single line.
	UnknownFrameKind FrameKind = 1 << iota

	// goroutine NN [<goroutine status>]:
	GoroutineHeaderKind

	// [originating from goroutine NN]:
	GoroutineAncestryKind

	// ...(additional| NN) frames elided...
	FramesElidedKind

	// created by path/of/pkg.funcName [in goroutine NN]
	// <tab>/path/to/file.go:NN[ +0xNN]
	CreatedByFrameKind

	// non-Go function at pc=...
	// <tab>/path/to/file.go:NN[ +0xNN]
	CgoUnknownFrameKind

	// path/of/pkg.funcName(...)
	// <tab>/path/to/file.go:NN[ +0xNN]
	StackFrameKind

	// Used to implement MaskAffected.
	maxFrameKind
)

func passthroughParse(fk FrameKind, lines []string) (Frame, bool) {
	return Frame{fk, "", "", lines}, true
}

func regexpPkgNameParse(re *regexp.Regexp) parseFunc {
	return func(fk FrameKind, lines []string) (Frame, bool) {
		groups := re.FindStringSubmatch(lines[0])
		if groups == nil {
			return Frame{}, false
		}
		if len(lines) > 1 {
			// Second line is source info - always starts with a tab.
			if lines[1][0] != '\t' {
				return Frame{}, false
			}
		}
		pkgName, funcName := groups[1], trimArgs(groups[2])
		if funcName == "" {
			// e.g. "panic(...)"
			funcName = trimArgs(pkgName)
			pkgName = ""
		}
		return Frame{fk, pkgName, funcName, lines}, true
	}
}

type parseFunc func(fk FrameKind, lines []string) (Frame, bool)

type parseConfig struct {
	frameKind            FrameKind
	hasSource            bool
	firstLinePrefix      string
	fallbackToStackFrame bool
	parse                parseFunc
}

func trimArgs(funcName string) string {
	if !strings.HasSuffix(funcName, ")") {
		return funcName
	}
	return funcName[:strings.LastIndexByte(funcName, '(')]
}

// created by name.func
// created by pkg/name.func
// created by pkg/name.func in goroutine NN
var createdByFrameParse = regexp.MustCompile(
	`^created by ((?:[^/]*/)*[^.]*)\.([^ ]*)( in goroutine.*)?\n$`,
)

// pkg/name.func(args)
var stackFrameParse = regexp.MustCompile(
	`^((?:[^/]*/)*[^.]*)\.?(.*)\n$`,
)

var stackFrameKindConfig = &parseConfig{
	StackFrameKind, true, "", false, regexpPkgNameParse(stackFrameParse),
}

var firstLetterPrefixMap = map[byte]*parseConfig{
	// These pass-through to StackFrameKind
	'c': {CreatedByFrameKind, true, "created by ", true, regexpPkgNameParse(createdByFrameParse)},
	'g': {GoroutineHeaderKind, false, "goroutine ", true, passthroughParse},
	'n': {CgoUnknownFrameKind, true, "non-Go function at ", true, passthroughParse},

	// These do not pass-through to StackFrameKind
	'[': {GoroutineAncestryKind, false, "[originating from goroutine ", false, passthroughParse},
	'.': {FramesElidedKind, false, "...", false, passthroughParse},
}

// MaskAffected yields all FrameKind values indicated by `f` as a bit mask.
//
// This will always yield FrameKind values from low to high.
func (f FrameKind) MaskAffected() iter.Seq[FrameKind] {
	return func(yield func(FrameKind) bool) {
		val := UnknownFrameKind
		for val < maxFrameKind {
			if (val & f) != 0 {
				if !yield(val) {
					return
				}
			}
			val <<= 1
		}
	}
}

// Frame represents a parsed stack frame in a [Trace].
//
// This contains the original lines that were parsed (as .Lines), as well as
// PkgName and/or FuncName (if the frame contained them).
//
// If you just want to reconstruct the original stack trace, you just need to
// concatenate Lines and can ignore the other fields entirely.
type Frame struct {
	// Kind is the detected type-of-frame.
	Kind FrameKind

	// PkgName is the package name, if this Frame has one.
	//
	// This will be something like `name/of/pkg` or `stdlibpkg`.
	//
	// NOTE: This is obtained by parsing the fully-qualified function name from
	// the first line of relevant frames until the last '/', and then scanning
	// until the first '.'. This means that if a package has a '.' in the last
	// directory there will be an incorrect parse.
	//
	// My best advice is "don't do that", but I'm documenting it here because
	// I assume it will trip someone (possibly me) up, and hopefully this doctring
	// saves them a couple minutes.
	//
	// This will be a strict substring of one of the strings in Lines.
	PkgName string

	// FuncName is the unqualified function name which is the remainder of the
	// line after parsing PkgName.
	//
	// See the NOTE on PkgName - the reciprocal issue could occur in FuncName
	// given the criteria documented in PkgName.
	//
	// This will be a strict substring of one of the strings in Lines.
	FuncName string

	// Lines are the original lines (including newlines) which corresponded with
	// this Frame.
	Lines []string
}

// SourceInfo returns the source path and line number (if this frame contains
// them).
//
// If this Frame does not have path info, this returns ("", -1)
// If this Frame has a path, but does not have lineNo info, this returns (path, 0)
//
// This function does not validate that `path` is anything resembling a real
// path, in the case that you [Parse]'d some garbled junk.
//
// UnknownFrameKind Frames never have SourceInfo.
func (f *Frame) SourceInfo() (path string, lineNo int) {
	lineNo = -1
	if f.Kind != UnknownFrameKind && len(f.Lines) >= 2 {
		if sourceLine, ok := strings.CutPrefix(f.Lines[1], "\t"); ok {
			lineNo = 0 // we have some sort of path, so zero lineNo
			// we need the last colon because windows paths have colon for the drive
			// separator.
			lastColonIdx := strings.LastIndexByte(sourceLine, ':')
			if lastColonIdx >= 0 {
				path = sourceLine[:lastColonIdx]
				numberStr, _, _ := strings.Cut(sourceLine[lastColonIdx+1:], " ")
				lineNo, _ = strconv.Atoi(numberStr)
			} else {
				path = sourceLine
			}
		}
	}
	return
}
