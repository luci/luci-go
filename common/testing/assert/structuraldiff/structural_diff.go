// Copyright 2024 The LUCI Authors.
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

package structuraldiff

import (
	"reflect"

	"github.com/kylelemons/godebug/pretty"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// DebugDump is an extremely abstraction-breaking function that will print a Go Value.
//
// It prints zero values and unexported fields and ignores user-defined methods for pretty-printing.
func DebugDump(val any) string {
	config := &pretty.Config{
		Compact:             false,
		Diffable:            true,
		IncludeUnexported:   true,
		PrintStringers:      false,
		PrintTextMarshalers: false,
		SkipZeroFields:      false,
		ShortList:           30,
		Formatter:           nil,
		TrackCycles:         true,
	}
	return config.Sprint(val)
}

// A Result is either a list of diffs
type Result struct {
	diffs   []diffmatchpatch.Diff
	message string
}

const sorry = `No difference between arguments but they are not equal.`

// DebugCompare compares two values of the same type and produces a structural diff between them.
//
// If we can't detect a difference between the two structures, return an apologetic message but
// NOT a nil result. That way we can compare the Result to nil to see if there were any problems.
func DebugCompare[T any](left T, right T) *Result {
	if reflect.DeepEqual(left, right) {
		return nil
	}
	patcher := diffmatchpatch.New()
	out := patcher.DiffMain(DebugDump(left), DebugDump(right), true)
	if len(out) == 0 {
		return &Result{
			message: sorry,
		}
	}
	return &Result{
		diffs: out,
	}
}
