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

package ui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

var (
	configCtxKey        = "lucicfg.internal.ui.Config"
	activityGroupCtxKey = "lucicfg.internal.ui.ActivityGroup"
)

// Config lives in the context and defines how activities are displayed.
type Config struct {
	Fancy bool     // if true, use fancy terminal output instead of plain logging
	Term  *os.File // a terminal for fancy output (unused if Fancy == false)
}

// WithConfig configures how activities will be displayed.
func WithConfig(ctx context.Context, cfg Config) context.Context {
	return context.WithValue(ctx, &configCtxKey, cfg)
}

// activityGroup is a group of related activities running at the same time.
//
// All activities in an activityGroup are displayed together in a nicely
// formatted coordinated way.
type activityGroup struct {
	cfg     Config
	title   string
	visTime time.Time // when to start displaying the activity

	wake chan struct{} // wakes up/stops the rendering goroutine
	done chan struct{} // closed when the rendering goroutine exits

	mu        sync.Mutex
	active    []*Activity
	maxPkgLen int  // to align columns, can only increase
	started   bool // true if already started
	stopped   bool // true if already stopped (and cannot be started again)
	seen      bool // true if displayed it at least once already
}

// NewActivityGroup returns a new context with a new activity group.
//
// Call the returned context.CancelFunc to finalize the activity group when all
// activities are finished and no new activities are expected. This will also
// cancel the context (to make sure no new activities can run in the finalized
// group).
func NewActivityGroup(ctx context.Context, title string) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)

	// If no fancy UI is needed, just do nothing. NewActivity(...) etc will
	// recognize this as the non-fancy mode and will regress to simple logging.
	cfg, _ := ctx.Value(&configCtxKey).(Config)
	if !cfg.Fancy {
		return ctx, cancel
	}

	group := &activityGroup{
		cfg:     cfg,
		title:   title,
		visTime: time.Now().Add(time.Second), // do not show super-fast activity groups
		wake:    make(chan struct{}, 1000),
		done:    make(chan struct{}),
	}
	ctx = context.WithValue(ctx, &activityGroupCtxKey, group)

	return ctx, func() {
		group.stop()
		cancel()
	}
}

// ensureRunningLocked launches a goroutine that constantly re-renders
// the activity group state.
func (ag *activityGroup) ensureRunningLocked() {
	if ag.started {
		return
	}
	ag.started = true
	go func() {
		defer close(ag.done)
		var renderState renderState
		for {
			select {
			case <-time.After(50 * time.Millisecond):
				ag.refresh(true, &renderState)
			case _, open := <-ag.wake:
				ag.refresh(false, &renderState)
				if !open {
					return
				}
			}
		}
	}()
}

// stop finishes running the goroutine that updates the activity group state.
func (ag *activityGroup) stop() {
	ag.mu.Lock()
	started := ag.started
	stopped := ag.stopped
	ag.stopped = true
	ag.mu.Unlock()
	if started && !stopped {
		close(ag.wake)
		<-ag.done
	}
}

// addActivity adds a new activity to the list of activities.
func (ag *activityGroup) addActivity(a *Activity) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	if !ag.stopped {
		ag.active = append(ag.active, a)
		if len(a.info.Package) > ag.maxPkgLen {
			ag.maxPkgLen = len(a.info.Package)
		}
		ag.wakeUpLocked()
	}
}

// updateActivity calls the callback under the lock if the activity group is
// still running.
func (ag *activityGroup) updateActivity(cb func()) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	if !ag.stopped {
		cb()
		ag.wakeUpLocked()
	}
}

// wakeUpLocked wakes up internal rendering goroutine.
//
// Called under the lock, with ag.stopped set to false.
func (ag *activityGroup) wakeUpLocked() {
	ag.ensureRunningLocked()
	select {
	case ag.wake <- struct{}{}:
	default:
	}
}

// makeVisible checks if it is OK to display the activity group now and prints
// the title if so.
func (ag *activityGroup) makeVisible() bool {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	if !ag.seen && time.Now().After(ag.visTime) {
		ag.seen = true
		fmt.Fprintf(ag.cfg.Term, "%s\n", ag.title)
	}
	return ag.seen
}

////////////////////////////////////////////////////////////////////////////////

var spinnerChars = []rune("⣾⣽⣻⢿⡿⣟⣯⣷")

// renderState is updated exclusively from the inner rendering goroutine.
type renderState struct {
	linesToRedraw int
	termWidth     int
	termCheck     time.Time
}

// refresh spins the spinners and rewrites the terminal.
func (ag *activityGroup) refresh(advance bool, rs *renderState) {
	if !ag.makeVisible() {
		return
	}

	// Throttle how often terminal.GetSize is called since it's not clear how
	// heavy it is (but it is clear that the terminal's width doesn't change
	// often).
	if rs.termCheck.IsZero() || time.Since(rs.termCheck) > time.Second {
		rs.termCheck = time.Now()
		rs.termWidth, _, _ = term.GetSize(int(ag.cfg.Term.Fd()))
		if rs.termWidth == 0 {
			rs.termWidth = 100
		}
	}

	// Get the snapshot of the new UI state to render.
	lines, activeNum := ag.render(advance, rs.termWidth)

	// Move the cursor up to start overwriting previously written lines.
	if rs.linesToRedraw > 0 {
		_, _ = fmt.Fprintf(ag.cfg.Term, "\033[%dA", rs.linesToRedraw)
	}
	// Render all lines, overwriting what was there previously.
	for _, line := range lines {
		_, _ = fmt.Fprintf(ag.cfg.Term, "\r\033[K%s\n", line)
	}

	// Tell the next refresh(...) call how many lines it should overwrite. We
	// won't be overwriting inactive lines anymore.
	rs.linesToRedraw = activeNum
}

// render asks activities to update their spinners and render themselves.
func (ag *activityGroup) render(advance bool, termWidth int) (lines []string, activeNum int) {
	ag.mu.Lock()
	defer ag.mu.Unlock()

	// Move all finished activities to the top. This rendering pass is the last
	// pass where we write their status. After that they'll just stay static in
	// the terminal history forever as finished.
	sort.SliceStable(ag.active, func(i, j int) bool {
		return ag.active[i].state.isFinished() && !ag.active[j].state.isFinished()
	})

	// Render all activities (active and no longer active).
	lines = make([]string, len(ag.active))
	for i, a := range ag.active {
		lines[i] = renderToLine(a, advance, ag.maxPkgLen, termWidth)
	}

	// Keep tracking only active activities.
	active := ag.active[:0]
	for _, a := range ag.active {
		if !a.state.isFinished() {
			active = append(active, a)
		}
	}
	ag.active = active

	return lines, len(active)
}

// renderToLine is called from the inner rendering goroutine under the activity
// group lock.
//
// It returns a single line representing this activity in the UI.
func renderToLine(a *Activity, advance bool, maxPkgLen, termWidth int) string {
	if a.state == activityRunning && advance {
		a.spinner++
	}

	var buf strings.Builder

	// Keep track of how many runes we still can fit into the line. Note that we
	// can't just use buf.Len() to calculate it since it returns number of bytes,
	// not runes.
	spaceLeft := termWidth

	// Writes an "indivisible" string. It will either will all fit or all will be
	// cut if the line is too long. If it is cut, the line will be considered
	// fully filled in.
	writeSymbols := func(s ...rune) {
		if len(s) < spaceLeft {
			buf.WriteString(string(s))
			spaceLeft -= len(s)
		} else {
			spaceLeft = 0
		}
	}

	// Writes a string that can be shortened if the line is too long.
	writeString := func(s string) {
		runes := utf8.RuneCountInString(s)
		if runes > spaceLeft {
			truncated := make([]rune, 0, spaceLeft)
			for _, r := range s {
				truncated = append(truncated, r)
				if spaceLeft--; spaceLeft == 1 { // leave room for '…'
					break
				}
			}
			buf.WriteString(string(append(truncated, '…')))
			spaceLeft = 0
		} else {
			buf.WriteString(s)
			spaceLeft -= runes
		}
	}

	var sym rune
	switch a.state {
	case activityPending:
		sym = '?'
	case activityRunning:
		sym = spinnerChars[a.spinner%len(spinnerChars)]
	case activityDone:
		sym = '•'
	default:
		sym = '!'
	}

	// The line layout is:
	// <spinner> <package> <padding> <version> [<message>]
	writeSymbols(' ', sym, ' ')
	writeString(padRight(a.info.Package, maxPkgLen))
	if a.info.Version != "" {
		writeSymbols(' ')
		writeString(padRight(a.info.Version, 40))
	}
	if a.message != "" {
		writeSymbols(' ')
		writeString(a.message)
	}
	return buf.String()
}

// padRight pads an ASCII string with spaces on the right to make it l bytes long.
func padRight(s string, l int) string {
	if len(s) < l {
		return s + strings.Repeat(" ", l-len(s))
	}
	return s
}
