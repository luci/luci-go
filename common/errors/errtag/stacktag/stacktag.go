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

// Package stacktag implements an error tag (using
// go.chromium.org/luci/common/errors/errtag) which contains a debug stacktrace
// at the point where the error was first tagged.
//
// The stacktag package makes the assumptions that:
//   - If an error has NO stack trace, attach one (excluding stacks generated at
//     init-time, as these are rarely useful).
//   - If an error has ONE stack trace, don't bother collecting another one.
//   - If we have a multi-error, where multiple errors each have a stack trace,
//     assume that the longest one will be the most useful.
//
// This last assumption is possibly the most contentious, and may need to be
// revised if it turns out to be too simplistic.
//
// # Previously
//
// A predecessor to this library used to do the following:
//
// Every time you 'annotated' an error it would collect the goroutine ID (by
// parsing the first ~64 bytes of a stack trace), and then look at the existing
// annotations in the error. An annotated error contained a list of
// (goroutineID, stack trace frames), where the 0th entry in this list was the
// first goroutine collected for this error, and the Nth entry was the most
// recently collected goroutine.
//
// After finding the goroutine ID, it would take a full stack trace, and use
// this to _insert_ the annotation into the closest matching stack trace frame.
//
// This had the effect that if you had code like:
//
//	func InnerFunc() (err error) {
//	  wg := sync.WaitGroup{}
//	  wg.Add(1)
//	  go func() {  // goroutine Inner
//	    err = errors.Reason("something").Err()   // LINE 1
//	    wg.Done()
//	  }()
//	  wg.Wait()
//	  return errors.Annotate(err, "additional").Err()  // LINE 2
//	}
//
//	func OuterFunc() error {
//	  if err := InnerFunc(); err != nil {  // LINE 3
//	    return errors.Annotate(err, "outer").Err()  // LINE 4
//	  }
//	  return nil
//	}
//
//	func Unrelated() error {
//	  return OuterFunc() // LINE 5
//	}
//
// When printing the stack trace you would observe a stack trace essentially like:
//
//	goroutine:Inner
//	file:LINE 1
//	  Reason: something
//
//	goroutine:Outer
//	file:LINE 2
//	  reason: additional
//
//	file:LINE 3
//	  reason: outer
//
//	file:LINE 5
//
// Notably, you wouldn't see LINE 4 at all, since it doesn't actually impact the
// genesis of `err` - the annotation of 'outer' is associated with LINE 3 which
// is where the error actually originated in the original call stack for the
// "Outer" goroutine.
//
// However, implementing this is actually quite complicated and not cheap.
package stacktag

import (
	"regexp"

	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/runtime/debugstack"
)

// Tag holds a stacktrace as a string.
//
// If multiple stack traces are present in an error, the longest of the stack
// traces will be returned as the canonical value.
var Tag = errtag.MakeWithMerge("stacktag.Capture", "", func(values []*string) *string {
	picked := values[0]
	lPicked := len(*picked)
	for _, val := range values[1:] {
		if valLen := len(*val); valLen > lPicked {
			picked = val
			lPicked = valLen
		}
	}
	return picked
})

var initFuncPattern = regexp.MustCompile(`init(?:\.\d+)?`)

// Capture will capture and attach a debug stack to the error.
//
// If the error already contains a stack trace, this will be a no-op.
//
// As a special case, if the captured stack contains only one StackFrameKind
// whose FuncName is "init" or matches "init\.\d+.*", this will not apply a stack
// trace to the error. This condition happens when defining sentinel errors at
// the module level, or within an explicit `func init()` in a module:
//
//	var SentinelErr = errors.New("something")  // runs in "init"
//	var OtherSentinelErr error
//
//	func init() {  // will be a function named e.g. "init.0"
//	  OtherSentinelErr = errors.New("something")
//	}
func Capture(err error, skipFrames int) error {
	if err == nil {
		return nil
	}
	if _, ok := Tag.Value(err); ok {
		return err
	}
	captured := debugstack.Capture(skipFrames + 1)
	match := false
	for _, frame := range captured {
		if frame.Kind == debugstack.StackFrameKind {
			if match { // found second StackFrameKind frame, bail
				match = false
				break
			}
			if initFuncPattern.MatchString(frame.FuncName) {
				match = true
				// so far so good, keep looping to see if there are more
				// StackFrameKind's.
			} else {
				// doesn't match the pattern give up immediately.
				match = false
				break
			}
		}
	}
	if match {
		return err
	}
	return Tag.ApplyValue(err, captured.String())
}
