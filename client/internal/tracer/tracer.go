// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tracer

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"runtime"
	"sync"
	"time"
)

// Scope is used with Instant event to determine the scope of the instantaneous
// event.
type Scope string

const (
	Global  Scope = "g"
	Process Scope = "p"
	Thread  Scope = "t"
)

// Args is user-defined arguments for an event. It can be anything as long as
// it is JSON serializable.
type Args map[string]interface{}

// Start starts the trace. There can be only one trace at a time. If a trace
// was already started, the current trace will not be affected and an error
// will be returned.
//
// Initial context has pid 1 and tid 1. Stop() must be called on exit to
// generate a valid JSON trace file.
//
// If stackDepth is non-zero, up to 'stackDepth' PC entries are kept for each
// log entry.
//
// TODO(maruel): Implement stackDepth.
func Start(w io.Writer, stackDepth int) error {
	lockWriter.Lock()
	defer lockWriter.Unlock()
	if out != nil {
		return errors.New("tracer was already started")
	}
	if w == nil {
		return errors.New("invalid output file")
	}

	lockContexts.Lock()
	defer lockContexts.Unlock()
	contexts = map[interface{}]*context{}
	tids = map[int]int{1: 1}

	out = w
	encoder = json.NewEncoder(out)
	first = true
	wd, _ := os.Getwd()
	args := Args{
		"args":   os.Args,
		"cwd":    wd,
		"goroot": runtime.GOROOT(),
	}

	// {
	//   "context": { ... },
	//   "traceEvents": [
	//     { ..., "ph": "B", "name": "A", "sf": 7},
	//     { ..., "ph": "E", "name": "A", "sf": 9}
	//   ],
	//   "stackFrames": {
	//     5: { "name": "main", "category": "my app" },
	//     7: { "parent": 5, "name": "SomeFunction", "category": "my app" },
	//     9: { "parent": 5, "name": "SomeFunction", "category": "my app" }
	//   }
	// }
	var err error
	if _, err = out.Write([]byte("{")); err == nil {
		if _, err = out.Write([]byte("\"context\":")); err == nil {
			if err = encoder.Encode(args); err == nil {
				if _, err = out.Write([]byte(",")); err == nil {
					_, err = out.Write([]byte("\"traceEvents\":["))
				}
			}
		}
	}
	if err != nil {
		// Unroll initialization.
		out = nil
		contexts = nil
		tids = nil
	}
	return err
}

// Stop stops the trace. It is important to call it so the trace file is
// properly formatted.
func Stop() {
	// Wait for on-going traces.
	wg.Wait()
	lockWriter.Lock()
	defer lockWriter.Unlock()
	lockContexts.Lock()
	defer lockContexts.Unlock()
	if out != nil {
		// TODO(maruel): Dump all the stack frames.
		_, _ = out.Write([]byte("]}"))
	}
	out = nil
	contexts = nil
	tids = nil
}

// Span defines an event with a duration. The caller MUST call the returned
// callback to 'close' the event.
//
// The callback doesn't need to be called from the same goroutine as the
// initial caller.
func Span(marker interface{}, category string, name string, args Args) func(args Args) {
	c := getContext(marker)
	if c == nil {
		return dummy
	}
	tsStart := time.Since(start)
	return func(argsEnd Args) {
		tsEnd := time.Since(start)
		if args != nil && argsEnd != nil {
			// Use a pair of eventBegin/eventEnd.
			c.emit(&event{
				Category:  category,
				Type:      eventBegin,
				Name:      name,
				Args:      args,
				Timestamp: fromDuration(tsStart),
			})
			c.emit(&event{
				Category:  category,
				Type:      eventEnd,
				Name:      name,
				Args:      argsEnd,
				Timestamp: fromDuration(tsEnd),
			})
		} else {
			// Use a single event for compactness.
			if args == nil {
				args = argsEnd
			}
			c.emit(&event{
				Category:  category,
				Type:      eventComplete,
				Name:      name,
				Args:      args,
				Timestamp: fromDuration(tsStart),
				Duration:  fromDuration(tsEnd - tsStart),
			})
		}
	}
}

// Instant registers an intantaneous event.
func Instant(marker interface{}, category string, name string, s Scope, args Args) {
	if c := getContext(marker); c != nil {
		c.emit(&event{
			Category: category,
			Type:     eventInstant,
			Name:     name,
			Scope:    s,
			Args:     args,
		})
	}
}

// NewPID assigns a pseudo-process ID for this marker and TID 1. Optionally
// assigns name to the 'process' and the initial thread.
//
// The main use is to create a logical group of 'threads'.
func NewPID(marker interface{}, pname, tname string) {
	lockContexts.Lock()
	defer lockContexts.Unlock()
	if contexts == nil {
		return
	}
	newPID := 0
	for pid := range tids {
		if pid > newPID {
			newPID = pid
		}
	}
	newPID++
	tids[newPID] = 1
	c := &context{newPID, 1}
	contexts[marker] = c
	if pname != "" {
		c.metadata(processName, Args{"name": pname})
	}
	if tname != "" {
		c.metadata(threadName, Args{"name": tname})
	}
}

// NewTID assigns a pseudo thread ID for this marker in the same process group
// as the parent. parent can be nil, in this case the new thread id is within
// process id 1. The new thread id is guaranteed to be above 1. Optionally
// assigns a name to the thread.
func NewTID(marker interface{}, parent interface{}, tname string) {
	parentC := getContext(parent)
	if parentC == nil {
		return
	}
	lockContexts.Lock()
	defer lockContexts.Unlock()
	tids[parentC.pid]++
	c := &context{pid: parentC.pid, tid: tids[parentC.pid]}
	contexts[marker] = c
	if tname != "" {
		c.metadata(threadName, Args{"name": tname})
	}
}

// Discard forgets a context association created with NewPID or NewTID.
func Discard(marker interface{}) {
	lockContexts.Lock()
	defer lockContexts.Unlock()
	if contexts != nil {
		delete(contexts, marker)
	}
	// Do not affect tids, tids is effectively a (small) memory leak.
}

// Private stuff.

var (
	// Immutable.
	start          = time.Now().UTC()
	defaultContext = context{1, 1}

	// Mutable.
	lockContexts sync.Mutex
	contexts     map[interface{}]*context
	tids         map[int]int
	wg           sync.WaitGroup

	lockWriter sync.Mutex
	out        io.Writer
	encoder    *json.Encoder
	first      bool
)

// eventType is one of the supported event type by
// https://github.com/google/trace-viewer.
type eventType string

const (
	// Duration Events. Duration events provide a way to mark a duration of work
	// on a given thread. These can be nested in the same Tid.
	eventBegin eventType = "B"
	eventEnd   eventType = "E"

	// Complete Events. Each complete event logically combines a pair of duration
	// (B and E) events.
	eventComplete eventType = "X"

	// Instant Events. Thread/process/global instantaneous event. The instant
	// events correspond to something that happens but has no time associated
	// with it. For example, vblank events are considered instant events. Using
	// Scope.
	eventInstant eventType = "i"

	// Counter Events. The counter events can track a value or multiple values as
	// they change over time. Used to count values via Args.
	eventCounter eventType = "C"

	// Async Events. Events that flows multiple threads, referenced by ID instead
	// of relying on Tid. It's not useful here since there's no thread ID in Go!
	eventNestableStart   eventType = "b"
	eventNestableEnd     eventType = "e"
	eventNestableInstant eventType = "n"

	// Flow Events. Essentially an arrow between two Duration events.
	eventFlowStart eventType = "s"
	eventFlowEnd   eventType = "f"
	eventFlowStep  eventType = "t"

	// Sample Events. Sample events provide a way of adding sampling-profiler
	// based results in a trace.
	eventSample eventType = "P"

	// Object Events. Used to track object lifetime.
	eventCreated   eventType = "N"
	eventSnapshot  eventType = "O"
	eventDestroyed eventType = "D"

	// Metadata Events. Metadata events are used to associate extra information
	// with the events in the trace file. Using metadataType.
	eventMetadata eventType = "M"

	// Memory Dump Events
	//
	// Global memory dump events, which contain system memory information such as
	// the size of RAM.
	eventGlobal eventType = "V"
	// Process memory dump events, which contain information about a single
	// process’s memory usage (e.g. total allocated memory).
	eventProcess eventType = "v"
)

// metadataType is used with Metadata events.
type metadataType string

const (
	// Sets the display name for the provided pid. The name is provided in a name
	// argument.
	processName metadataType = "process_name"
	// Sets the extra process labels for the provided pid. The label is provided
	// in a labels argument.
	processLabels metadataType = "process_labels"
	// Sets the process sort order position. The sort index is provided in a
	// sort_index argument.
	processSortIndex metadataType = "process_sort_index"
	// Sets the name for the given tid. The name is provided in a name argument.
	threadName metadataType = "thread_name"
	// Sets the thread sort order position. The sort index is provided in a
	// sort_index argument.
	threadSortIndex metadataType = "thread_sort_index"
)

// event is a single trace line.
//
// See format description at
// https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU/edit
type event struct {
	Pid       int          `json:"pid"`            // Required. Process ID.
	Tid       int          `json:"tid"`            // Required. Thread ID. It is implicitly used to set start/end.
	Timestamp microseconds `json:"ts"`             // From process start.
	Type      eventType    `json:"ph"`             // Required. The event type. This is a single character which changes depending on the type of event being output.
	Category  string       `json:"cat,omitempty"`  // Optional. The event categories. This is a comma separated list of categories for the event. The categories can be used to hide events in the Trace Viewer UI.
	Name      string       `json:"name,omitempty"` // Optional. The name of the event, as displayed in Trace Viewer.
	Args      Args         `json:"args,omitempty"` // Optional. Cannot be used with Object. Any arguments provided for the event. Some of the event types have required argument fields, otherwise, you can put any information you wish in here. The arguments are displayed in Trace Viewer when you view an event in the analysis section.
	Duration  microseconds `json:"dur,omitempty"`  // Optional. Only for Complete.
	Scope     Scope        `json:"s,omitempty"`    // Optional. Only for Instant. Defaults to ScopeThread.
	/* TODO(maruel): Add these if ever used, commented out for performance.
	StackID         int          `json:"sf,omitempty"`     // Optional. Stack ID found in stackFrames section.
	Stack           []string     `json:"stack,omitempty"`  // Optional. Raw stack.
	EndStackID      int          `json:"esf,omitempty"`    // Optional. Only for Complete for end stack. Stack ID found in stackFrames section.
	EndStack        []string     `json:"estack,omitempty"` // Optional. Only for Complete for end stack. Raw stack.
	ID              int          `json:"id,omitempty"`     // Optional. Only for Async or Object.
	ThreadTimestamp microseconds `json:"tts,omitempty"`    // Undocumented.
	ThreadDuration  microseconds `json:"tdur,omitempty"`   // Undocumented.
	*/
}

// stackFrame is used in 'stackFrames' section.
// TODO(maruel): Use it.
type stackFrame struct {
	Parent   int    `json:"parent,omitempty"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// microseconds is used to convert from time.Duration.
type microseconds float64

// fromDuration converts a time in nanosecond to time in µs.
func fromDuration(t time.Duration) microseconds {
	return microseconds(float64(t) / float64(time.Microsecond))
}

// context embeds a pseudo thread id for this context. It's useful to keep
// context, as runtime doesn't expose the goroutine id.
type context struct {
	pid int
	tid int
}

// getContext returns a context if tracing is enabled. If the marker is
// unknown, the default {1, 1} context is returned.
func getContext(marker interface{}) *context {
	lockContexts.Lock()
	defer lockContexts.Unlock()
	if contexts == nil {
		return nil
	}
	c, ok := contexts[marker]
	if !ok {
		return &defaultContext
	}
	return c
}

// emit asynchronously emits a trace event.
func (c *context) emit(e *event) {
	if e.Timestamp == 0 {
		e.Timestamp = fromDuration(time.Since(start))
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		e.Pid = c.pid
		e.Tid = c.tid
		lockWriter.Lock()
		defer lockWriter.Unlock()
		if out != nil {
			if first {
				first = false
			} else {
				if _, err := out.Write([]byte(",")); err != nil {
					log.Printf("failed writing to trace: %s", err)
					go Stop()
					return
				}
			}
			if err := encoder.Encode(e); err != nil {
				log.Printf("failed writing to trace: %s", err)
				go Stop()
			}
		}
	}()
}

// metadata registers metadata in the trace. For example putting a name on the
// current pseudo process id or pseudo thread id.
func (c *context) metadata(m metadataType, args Args) {
	c.emit(&event{Type: eventMetadata, Name: string(m), Args: args})
}

func dummy(Args) {
}
