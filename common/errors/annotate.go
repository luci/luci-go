// Copyright 2016 The LUCI Authors.
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

package errors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/text/indented"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/goroutine"
)

type stack struct {
	id     goroutine.ID
	frames []uintptr
}

func (s *stack) findPointOfDivergence(other *stack) int {
	// TODO(iannucci): can we optimize this search routine to not overly penalize
	// tail-recursive functions? Searching 'up' from both stacks doesn't work in
	// the face of recursion because there will be multiple ambiguous stack
	// frames. The algorithm below is correct, but it potentially collects and
	// then throws away a bunch of data (e.g. if we could walk up from the current
	// stack and then find the intersection in the captured stack, we could stop
	// walking at that point instead of walking all the way back from the root on
	// every Annotate call.
	if s.id != other.id {
		panic(fmt.Errorf(
			"finding intersection between unrelated stacks: %d v %d", s.id, other.id))
	}

	myIdx := len(s.frames) - 1
	oIdx := len(other.frames) - 1

	for s.frames[myIdx] == other.frames[oIdx] {
		myIdx--
		oIdx--
	}

	return myIdx
}

// stackContexter is the interface that an error may implement if it has data
// associated with a specific stack frame.
type stackContexter interface {
	stackContext() stackContext
}

// stackFrameInfo holds a stack and an index into that stack for association
// with stackContexts.
type stackFrameInfo struct {
	frameIdx int
	forStack *stack
}

// stackContext represents the annotation data associated with an error, or an
// annotation of an error.
type stackContext struct {
	frameInfo stackFrameInfo
	// publicly-facing reason, and will show up in the Error() string.
	reason string

	// used for printing tracebacks, but will not show up in the Error() string.
	internalReason string

	// tags are any data associated with this frame.
	tags map[TagKey]interface{}
}

// renderPublic renders the public error.Error()-style string for this frame,
// combining this frame's Reason with the inner error.
func (s *stackContext) renderPublic(inner error) string {
	switch {
	case inner == nil:
		return s.reason
	case s.reason == "":
		return inner.Error()
	}
	return fmt.Sprintf("%s: %s", s.reason, inner.Error())
}

// render renders the frame as a single entry in a stack trace. This looks like:
//
//  reason: "The literal content of the reason field: %(key2)d"
//  internal reason: I am an internal reason formatted with key1: value
func (s *stackContext) render() lines {
	siz := len(s.tags)
	if s.internalReason != "" {
		siz++
	}
	if s.reason != "" {
		siz++
	}
	if siz == 0 {
		return nil
	}

	ret := make(lines, 0, siz)
	if s.reason != "" {
		ret = append(ret, fmt.Sprintf("reason: %s", s.reason))
	}
	if s.internalReason != "" {
		ret = append(ret, fmt.Sprintf("internal reason: %s", s.internalReason))
	}
	keys := make(tagKeySlice, 0, len(s.tags))
	for key := range s.tags {
		keys = append(keys, key)
	}
	sort.Sort(keys)
	for _, key := range keys {
		if key != nil {
			ret = append(ret, fmt.Sprintf("tag[%q]: %#v", key.description, s.tags[key]))
		} else {
			ret = append(ret, fmt.Sprintf("tag[nil]: %#v", s.tags[key]))
		}
	}

	return ret
}

type terminalStackError struct {
	error
	finfo stackFrameInfo
	tags  map[TagKey]interface{}
}

var _ interface {
	error
	stackContexter
} = (*terminalStackError)(nil)

func (e *terminalStackError) stackContext() stackContext {
	return stackContext{frameInfo: e.finfo, tags: e.tags}
}

type annotatedError struct {
	inner error
	ctx   stackContext
}

var _ interface {
	error
	stackContexter
	Wrapped
} = (*annotatedError)(nil)

func (e *annotatedError) Error() string              { return e.ctx.renderPublic(e.inner) }
func (e *annotatedError) stackContext() stackContext { return e.ctx }
func (e *annotatedError) Unwrap() error              { return e.inner }

// Annotator is a builder for annotating errors. Obtain one by calling Annotate
// on an existing error or using Reason.
//
// See the example test for Annotate to see how this is meant to be used.
type Annotator struct {
	inner error
	ctx   stackContext
}

// InternalReason adds a stack-trace-only internal reason string (for humans) to
// this error.
//
// The text here will only be visible when using `errors.Log` or
// `errors.RenderStack`, not when calling the .Error() method of the resulting
// error.
//
// The `reason` string is formatted with `args` and may contain Sprintf-style
// formatting directives.
func (a *Annotator) InternalReason(reason string, args ...interface{}) *Annotator {
	if a == nil {
		return a
	}
	a.ctx.internalReason = fmt.Sprintf(reason, args...)
	return a
}

// Tag adds a tag with an optional value to this error.
//
// `value` is a unary optional argument, and must be a simple type (i.e. has
// a reflect.Kind which is a base data type like bool, string, or int).
func (a *Annotator) Tag(tags ...TagValueGenerator) *Annotator {
	if a == nil {
		return a
	}
	tagMap := make(map[TagKey]interface{}, len(tags))
	for _, t := range tags {
		v := t.GenerateErrorTagValue()
		tagMap[v.Key] = v.Value
	}
	if len(tagMap) > 0 {
		if a.ctx.tags == nil {
			a.ctx.tags = tagMap
		} else {
			for k, v := range tagMap {
				a.ctx.tags[k] = v
			}
		}
	}
	return a
}

// Err returns the finalized annotated error.
//
//go:noinline
func (a *Annotator) Err() error {
	if a == nil {
		return nil
	}
	return (*annotatedError)(a)
}

// Log logs the full error. If this is an Annotated error, it will log the full
// stack information as well.
//
// This is a shortcut for logging the output of RenderStack(err).
func Log(c context.Context, err error, excludePkgs ...string) {
	log := logging.Get(c)
	for _, l := range RenderStack(err, excludePkgs...) {
		log.Errorf("%s", l)
	}
}

// lines is just a list of printable lines.
//
// It's a type because it's most frequently used as []lines, and [][]string
// doesn't read well.
type lines []string

// renderedFrame represents a single, rendered stack frame.
type renderedFrame struct {
	pkg      string
	file     string
	lineNum  int
	funcName string

	// wrappers is any frame-info-less errors.Wrapped that were encountered when
	// rendering that didn't have any associated frame info: this is the closest
	// frame to where they were added to the error.
	wrappers []lines

	// annotations is any Annotate context associated directly with this Frame.
	annotations []lines
}

var nlSlice = []byte{'\n'}

// dumpWrappersTo formats the wrappers portion of this renderedFrame.
func (r *renderedFrame) dumpWrappersTo(w io.Writer, from, to int) (n int, err error) {
	return iotools.WriteTracker(w, func(rawWriter io.Writer) error {
		w := &indented.Writer{Writer: rawWriter, UseSpaces: true}
		fmt.Fprintf(w, "From frame %d to %d, the following wrappers were found:\n", from, to)
		for i, wrp := range r.wrappers {
			if i != 0 {
				w.Write(nlSlice)
			}
			w.Level = 2
			for i, line := range wrp {
				if i == 0 {
					fmt.Fprintf(w, "%s\n", line)
					w.Level += 2
				} else {
					fmt.Fprintf(w, "%s\n", line)
				}
			}
		}
		return nil
	})
}

// dumpTo formats the Header and annotations for this renderedFrame.
func (r *renderedFrame) dumpTo(w io.Writer, idx int) (n int, err error) {
	return iotools.WriteTracker(w, func(rawWriter io.Writer) error {
		w := &indented.Writer{Writer: rawWriter, UseSpaces: true}

		fmt.Fprintf(w, "#%d %s/%s:%d - %s()\n", idx, r.pkg, r.file,
			r.lineNum, r.funcName)
		w.Level += 2
		switch len(r.annotations) {
		case 0:
			// pass
		case 1:
			for _, line := range r.annotations[0] {
				fmt.Fprintf(w, "%s\n", line)
			}
		default:
			for i, ann := range r.annotations {
				fmt.Fprintf(w, "annotation #%d:\n", i)
				w.Level += 2
				for _, line := range ann {
					fmt.Fprintf(w, "%s\n", line)
				}
				w.Level -= 2
			}
		}
		return nil
	})
}

// renderedStack is a single rendered stack from one goroutine.
type renderedStack struct {
	goID   goroutine.ID
	frames []*renderedFrame
}

// dumpTo formats the full stack.
func (r *renderedStack) dumpTo(w io.Writer, excludePkgs ...string) (n int, err error) {
	excludeSet := stringset.NewFromSlice(excludePkgs...)

	return iotools.WriteTracker(w, func(w io.Writer) error {
		fmt.Fprintf(w, "goroutine %d:\n", r.goID)

		lastIdx := 0
		needNL := false
		skipCount := 0
		skipPkg := ""
		flushSkips := func(extra string) {
			if skipCount != 0 {
				if needNL {
					w.Write(nlSlice)
					needNL = false
				}
				fmt.Fprintf(w, "... skipped %d frames in pkg %q...\n%s", skipCount, skipPkg, extra)
				skipCount = 0
				skipPkg = ""
			}
		}
		for i, f := range r.frames {
			if needNL {
				w.Write(nlSlice)
				needNL = false
			}
			if excludeSet.Has(f.pkg) {
				if skipPkg == f.pkg {
					skipCount++
				} else {
					flushSkips("")
					skipCount++
					skipPkg = f.pkg
				}
				continue
			}
			flushSkips("\n")
			if len(f.wrappers) > 0 {
				f.dumpWrappersTo(w, lastIdx, i)
				w.Write(nlSlice)
			}
			if len(f.annotations) > 0 {
				lastIdx = i
				needNL = true
			}
			f.dumpTo(w, i)
		}
		flushSkips("")

		return nil
	})
}

// renderedError is a series of RenderedStacks, one for each goroutine that the
// error was annotated on.
type renderedError struct {
	originalError string
	stacks        []*renderedStack
}

// toLines renders a full-information stack trace as a series of lines.
func (r *renderedError) toLines(excludePkgs ...string) lines {
	buf := bytes.Buffer{}
	r.dumpTo(&buf, excludePkgs...)
	return strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
}

// dumpTo writes the full-information stack trace to the writer.
func (r *renderedError) dumpTo(w io.Writer, excludePkgs ...string) (n int, err error) {
	return iotools.WriteTracker(w, func(w io.Writer) error {
		if r.originalError != "" {
			fmt.Fprintf(w, "original error: %s\n\n", r.originalError)
		}

		for i := len(r.stacks) - 1; i >= 0; i-- {
			if i != len(r.stacks)-1 {
				w.Write(nlSlice)
			}
			r.stacks[i].dumpTo(w, excludePkgs...)
		}
		return nil
	})
}

func frameHeaderDetails(frm uintptr) (pkg, filename, funcName string, lineno int) {
	// this `frm--` is to get the correct line/function information, since the
	// Frame is actually the `return` pc. See runtime.Callers.
	frm--

	fn := runtime.FuncForPC(frm)
	file, lineno := fn.FileLine(frm)

	var dirpath string
	dirpath, filename = filepath.Split(file)
	pkgTopLevelName := filepath.Base(dirpath)

	fnName := fn.Name()
	lastSlash := strings.LastIndex(fnName, "/")
	if lastSlash == -1 {
		funcName = fnName
		pkg = pkgTopLevelName
	} else {
		funcName = fnName[lastSlash+1:]
		pkg = fmt.Sprintf("%s/%s", fnName[:lastSlash], pkgTopLevelName)
	}
	return
}

// RenderStack renders the error to a list of lines.
func RenderStack(err error, excludePkgs ...string) []string {
	return renderStack(err).toLines(excludePkgs...)
}

func renderStack(err error) *renderedError {
	ret := &renderedError{}

	lastAnnotatedFrame := 0
	var wrappers = []lines{}
	getCurFrame := func(fi *stackFrameInfo) *renderedFrame {
		if len(ret.stacks) == 0 || ret.stacks[len(ret.stacks)-1].goID != fi.forStack.id {
			lastAnnotatedFrame = len(fi.forStack.frames) - 1
			toAdd := &renderedStack{
				goID:   fi.forStack.id,
				frames: make([]*renderedFrame, len(fi.forStack.frames)),
			}
			for i, frm := range fi.forStack.frames {
				pkgPath, filename, functionName, line := frameHeaderDetails(frm)
				toAdd.frames[i] = &renderedFrame{
					pkg: pkgPath, file: filename, lineNum: line, funcName: functionName}
			}
			ret.stacks = append(ret.stacks, toAdd)
		}
		curStack := ret.stacks[len(ret.stacks)-1]

		if fi.frameIdx < lastAnnotatedFrame {
			lastAnnotatedFrame = fi.frameIdx
			frm := curStack.frames[lastAnnotatedFrame]
			frm.wrappers = wrappers
			wrappers = nil
			return frm
		}
		return curStack.frames[lastAnnotatedFrame]
	}

	for err != nil {
		if sc, ok := err.(stackContexter); ok {
			ctx := sc.stackContext()
			if stk := ctx.frameInfo.forStack; stk != nil {
				frm := getCurFrame(&ctx.frameInfo)
				if rendered := ctx.render(); len(rendered) > 0 {
					frm.annotations = append(frm.annotations, rendered)
				}
			} else {
				wrappers = append(wrappers, ctx.render())
			}
		} else {
			wrappers = append(wrappers, lines{fmt.Sprintf("unknown wrapper %T", err)})
		}
		switch x := err.(type) {
		case MultiError:
			// TODO(riannucci): it's kinda dumb that we have to walk the MultiError
			// twice (once in its stackContext method, and again here).
			err = x.First()
		case Wrapped:
			err = x.Unwrap()
		default:
			ret.originalError = err.Error()
			err = nil
		}
	}

	return ret
}

// Annotate captures the current stack frame and returns a new annotatable
// error, attaching the publicly readable `reason` format string to the error.
// You can add additional metadata to this error with the 'InternalReason' and
// 'Tag' methods, and then obtain a real `error` with the Err() function.
//
// If this is passed nil, it will return a no-op Annotator whose .Err() function
// will also return nil.
//
// The original error may be recovered by using Wrapped.Unwrap on the
// returned error.
//
// Rendering the derived error with Error() will render a summary version of all
// the public `reason`s as well as the initial underlying error's Error() text.
// It is intended that the initial underlying error and all annotated reasons
// only contain user-visible information, so that the accumulated error may be
// returned to the user without worrying about leakage.
//
// You should assume that end-users (including unauthenticated end users) may
// see the text in the `reason` field here. To only attach an internal reason,
// leave the `reason` argument blank and don't pass any additional formatting
// arguments.
//
// The `reason` string is formatted with `args` and may contain Sprintf-style
// formatting directives.
func Annotate(err error, reason string, args ...interface{}) *Annotator {
	if err == nil {
		return nil
	}
	return &Annotator{err, stackContext{
		frameInfo: stackFrameInfoForError(1, err),
		reason:    fmt.Sprintf(reason, args...),
	}}
}

// Reason builds a new Annotator starting with reason. This allows you to use
// all the formatting directives you would normally use with Annotate, in case
// your originating error needs tags or an internal reason.
//
//   errors.Reason("something bad: %d", value).Tag(transient.Tag).Err()
//
// Prefer this form to errors.New(fmt.Sprintf("..."))
func Reason(reason string, args ...interface{}) *Annotator {
	currentStack := captureStack(1)
	frameInfo := stackFrameInfo{0, currentStack}
	return (&Annotator{nil, stackContext{
		frameInfo: frameInfo,
		reason:    fmt.Sprintf(reason, args...),
	}})
}

// New is an API-compatible version of the standard errors.New function. Unlike
// the stdlib errors.New, this will capture the current stack information at the
// place this error was created.
func New(msg string, tags ...TagValueGenerator) error {
	tse := &terminalStackError{
		errors.New(msg), stackFrameInfo{forStack: captureStack(1)}, nil}
	if len(tags) > 0 {
		tse.tags = make(map[TagKey]interface{}, len(tags))
		for _, t := range tags {
			v := t.GenerateErrorTagValue()
			tse.tags[v.Key] = v.Value
		}
	}
	return tse
}

func captureStack(skip int) *stack {
	fullStk := stack{goroutine.CurID(), nil}
	stk := make([]uintptr, 16)
	offset := skip + 2
	for n := len(stk); n == len(stk); {
		n = runtime.Callers(offset, stk)
		offset += n
		fullStk.frames = append(fullStk.frames, stk[:n]...)
	}
	return &fullStk
}

func getCapturedStack(err error) (ret *stack) {
	Walk(err, func(err error) bool {
		if sc, ok := err.(stackContexter); ok {
			ret = sc.stackContext().frameInfo.forStack
			return false
		}
		return true
	})
	return
}

// stackFrameInfoForError returns a stackFrameInfo suitable for use to implement
// the stackContexter interface.
//
// It skips the provided number of frames when collecting the current trace
// (which should be equal to the number of functions between your error library
// and the user's code).
//
// The returned stackFrameInfo will find the appropriate frame in the error's
// existing stack information (if the error was created with errors.New), or
// include the current stack if it was not.
func stackFrameInfoForError(skip int, err error) stackFrameInfo {
	currentStack := captureStack(skip + 1)
	currentlyCapturedStack := getCapturedStack(err)
	if currentlyCapturedStack == nil || currentStack.id != currentlyCapturedStack.id {
		// This is the very first annotation on this error OR
		// We switched goroutines.
		return stackFrameInfo{forStack: currentStack}
	}
	return stackFrameInfo{
		frameIdx: currentlyCapturedStack.findPointOfDivergence(currentStack),
		forStack: currentlyCapturedStack,
	}
}
