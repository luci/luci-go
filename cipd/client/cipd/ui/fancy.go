// Copyright 2021 The LUCI Authors.
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
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh/terminal"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

var spinnerChars = []rune("⣾⣽⣻⢿⡿⣟⣯⣷")

// FancyImplementation implements activities UI using terminal escape sequences.
type FancyImplementation struct {
	Out *os.File // where to write the output, should be a terminal

	// A session represents a "burst of activity" and a refresh goroutine.
	sessionM sync.Mutex
	session  *refreshSession

	// Info about the terminal dimensions.
	termM     sync.Mutex
	termWidth int
	termCheck time.Time
}

// NewActivity is a part of Implementation interface.
func (impl *FancyImplementation) NewActivity(ctx context.Context, group *ActivityGroup, kind string) Activity {
	a := &fancyActivity{
		impl:  impl,
		group: group,
		kind:  kind,
	}
	if group != nil && kind != "" {
		a.id = group.allocateID(kind)
	}
	return a
}

// terminalWidth returns the current terminal width.
//
// Throttles how often terminal.GetSize is called since it's not clear how heavy
// it is (but it is clear that the terminal's width doesn't change often).
func (impl *FancyImplementation) terminalWidth() int {
	impl.termM.Lock()
	defer impl.termM.Unlock()
	if impl.termCheck.IsZero() || time.Since(impl.termCheck) > time.Second {
		impl.termCheck = time.Now()
		impl.termWidth, _, _ = terminal.GetSize(int(impl.Out.Fd()))
		if impl.termWidth == 0 {
			impl.termWidth = 100
		}
	}
	return impl.termWidth
}

// activityWokeUp is called when a new activity starts running.
func (impl *FancyImplementation) activityWokeUp(a *fancyActivity) {
	impl.sessionM.Lock()
	defer impl.sessionM.Unlock()
	if impl.session == nil {
		impl.session = newRefreshSession(impl.Out, impl.terminalWidth)
	}
	impl.session.addActivity(a)
}

// someActivityDone is called when an activity finishes running.
func (impl *FancyImplementation) someActivityDone() {
	var sessionToStop *refreshSession

	impl.sessionM.Lock()
	if impl.session != nil && impl.session.allActivitiesDone() {
		sessionToStop = impl.session
		impl.session = nil
	}
	impl.sessionM.Unlock()

	if sessionToStop != nil {
		sessionToStop.stop()
	}
}

// refreshSession represents an active session of periodically
// refreshing the terminal with progress of activities.
type refreshSession struct {
	wake chan struct{} // wakes up/stops the rendering goroutine
	done chan struct{} // closed when the rendering goroutine exits

	m             sync.RWMutex
	activities    []*fancyActivity // started (and perhaps finished) activities
	linesToRedraw int              // how many terminal lines we need to redraw
}

// newRefreshSession launches a goroutine that updates the terminal output.
func newRefreshSession(out *os.File, width func() int) *refreshSession {
	s := &refreshSession{
		wake: make(chan struct{}, 10000),
		done: make(chan struct{}),
	}

	go func() {
		defer close(s.done)
		for {
			select {
			case <-time.After(50 * time.Millisecond):
				s.renderUI(out, true, width()) // time passed, spin spinners
			case _, open := <-s.wake:
				s.renderUI(out, false, width()) // woke up early, don't spin spinners
				if !open {
					return
				}
			}
		}
	}()

	return s
}

// stop stops the goroutine that updates the terminal.
func (s *refreshSession) stop() {
	close(s.wake)
	<-s.done
}

// addActivity is called when a new activity starts running.
//
// It is added to the list of displayed activities.
func (s *refreshSession) addActivity(a *fancyActivity) {
	s.m.Lock()
	defer s.m.Unlock()
	s.activities = append(s.activities, a)
	s.wake <- struct{}{}
}

// allActivitiesDone returns true if all registered activities are stopped.
func (s *refreshSession) allActivitiesDone() bool {
	s.m.RLock()
	defer s.m.RUnlock()
	for _, a := range s.activities {
		if a.isActive() {
			return false
		}
	}
	return true
}

// renderUI renders the UI and does some housekeeping.
func (s *refreshSession) renderUI(out io.Writer, advance bool, width int) {
	s.m.Lock()
	defer s.m.Unlock()

	// Move the cursor up to start overwriting previously written lines.
	if s.linesToRedraw > 0 {
		fmt.Fprintf(out, "\033[%dA", s.linesToRedraw)
	}

	// Render all progress lines and figure out which of them are still running.
	type activityLine struct {
		activity *fancyActivity
		line     string // rendered progress line
		active   bool   // false if will never ever change again
	}
	lines := make([]activityLine, len(s.activities))
	for i, activity := range s.activities {
		line, active := activity.renderLine(advance, width)
		lines[i] = activityLine{activity, line, active}
	}

	// Move all finished activities to the top. This rendering pass is the last
	// pass where we write their status. After that they'll just stay static in
	// the terminal history forever as finished.
	sort.SliceStable(lines, func(i, j int) bool {
		return !lines[i].active && lines[j].active
	})

	// Render all lines and reassemble s.activities keeping only active ones.
	s.activities = s.activities[:0]
	for _, l := range lines {
		fmt.Fprintf(out, "\r\033[K%s\n", l.line) // overwrite the line
		if l.active {
			s.activities = append(s.activities, l.activity)
		}
	}

	// Tell the next renderUI call how many lines it should overwrite. We won't
	// be overwritting inactive lines anymore.
	s.linesToRedraw = len(s.activities)
}

////////////////////////////////////////////////////////////////////////////////

type fancyActivityState int

const (
	stateDormant fancyActivityState = 0 // not doing anything yet
	stateActive  fancyActivityState = 1 // actually doing stuff
	stateDone    fancyActivityState = 2 // finished doing stuff
)

type fancyActivity struct {
	impl  *FancyImplementation
	group *ActivityGroup
	kind  string
	id    int

	m             sync.Mutex
	state         fancyActivityState
	round         int // what frame of the spinner animation to show
	progressTitle string
	progressUnits Units
	progressCur   int64
	progressTotal int64
	logMessage    string
	logLevel      logging.Level
	speed         speedGauge
}

func (a *fancyActivity) mutateLockedState(cb func()) {
	a.m.Lock()
	wokeUp := a.state == stateDormant
	if a.state == stateDormant {
		a.state = stateActive
	}
	if a.state == stateActive {
		cb()
	}
	a.m.Unlock()

	if wokeUp {
		a.impl.activityWokeUp(a)
	}
}

func (a *fancyActivity) Progress(ctx context.Context, title string, units Units, cur, total int64) {
	a.mutateLockedState(func() {
		reset := a.progressCur > cur || a.progressTotal != total || a.progressUnits != units
		if reset {
			a.speed.reset(clock.Now(ctx), cur)
		} else {
			a.speed.advance(clock.Now(ctx), cur)
		}

		a.progressTitle = title
		a.progressUnits = units
		a.progressCur = cur
		a.progressTotal = total

		// A progress report overrides any previous notices.
		a.logMessage = ""
		a.logLevel = logging.Info
	})
}

func (a *fancyActivity) Log(ctx context.Context, lc *logging.LogContext, level logging.Level, calldepth int, f string, args []any) {
	if level <= logging.Debug {
		return
	}
	a.mutateLockedState(func() {
		a.logMessage = fmt.Sprintf(f, args...)
		a.logLevel = level
	})
}

func (a *fancyActivity) Done(ctx context.Context) {
	a.m.Lock()
	wasActive := a.state == stateActive
	a.state = stateDone
	a.m.Unlock()

	if wasActive {
		a.impl.someActivityDone()
	}
}

func (a *fancyActivity) isActive() bool {
	a.m.Lock()
	defer a.m.Unlock()
	return a.state == stateActive
}

// renderLine composes a status line of this activity.
//
// If `advance` is true, will "spin" the progress spinner.
func (a *fancyActivity) renderLine(advance bool, width int) (line string, active bool) {
	a.m.Lock()
	active = a.state == stateActive
	round := a.round
	progressTitle := a.progressTitle
	progressCur := a.progressCur
	progressTotal := a.progressTotal
	logMessage := a.logMessage
	logLevel := a.logLevel
	speed := a.speed.speed
	if active && advance {
		a.round++
	}
	a.m.Unlock()

	// Construct strings with detailed progress and speed stats.
	progressStr := ""
	speedStr := ""
	if progressTotal != 0 {
		progressStr = fmt.Sprintf(" %.1f/%.1f MB",
			float64(progressCur)/1000000.0,
			float64(progressTotal)/1000000.0,
		)
		if speed >= 0 {
			speedStr = fmt.Sprintf("│ %.1f MB/s ", speed/1000000.0)
		} else {
			speedStr = "│ ?? MB/s "
		}
	}

	// The line being constructed.
	buf := strings.Builder{}

	// Keep track of how many runes we wrote. Note that we can't just use
	// buf.Len() since it returns a number of bytes, not runes.
	runes := 0

	writeRune := func(r rune) {
		buf.WriteRune(r)
		runes++
	}

	writeString := func(s string) {
		buf.WriteString(s)
		runes += utf8.RuneCountInString(s)
	}

	// The line layout is:
	// <spinner> <activityTitle> <progressTitle> <progressBar> <progressPercent>

	// Write the spinner section: " ⣾ ".
	writeRune(' ')
	if active {
		writeRune(spinnerChars[round%len(spinnerChars)])
	} else {
		if logLevel == logging.Info {
			writeRune('•') // indicates success
		} else {
			writeRune('!') // indicates something was wrong
		}
	}
	writeRune(' ')

	// Write the activity title, e.g. "[fetch 10/11]".
	if a.group != nil && a.kind != "" {
		writeString("[" + a.group.activityTitle(a.kind, a.id) + "] ")
	}

	// If have a notice, show it instead of the progress bar.
	if logMessage != "" {
		// Need to avoid line wrapping in the terminal, it breaks line overwrites.
		spaceLeft := width - 1 - runes // -1 is for "…""
		if utf8.RuneCountInString(logMessage) > spaceLeft {
			truncated := make([]rune, 0, spaceLeft)
			for _, r := range logMessage {
				truncated = append(truncated, r)
				if spaceLeft--; spaceLeft == 0 {
					break
				}
			}
			writeString(string(truncated))
			writeRune('…')
		} else {
			writeString(logMessage)
		}
	} else {
		// Write the progress title along with some stats.
		start := runes
		writeString(progressTitle + progressStr + " ")

		// Pad it with spaces on the right to align speed indicators across lines.
		if titleLen := runes - start; titleLen < 30 {
			writeString(strings.Repeat(" ", 30-titleLen))
		}
		writeString(padRight(speedStr, len("| 1000.0 MB/s ")))

		// Too wide progress lines look ugly.
		if width > 160 {
			width = 160
		}

		// Figure out how much space is left for the progress bar.
		progressWidth := width - runes - len("* 100%")
		if progressWidth < 1 {
			progressWidth = 1
		}

		// Render the progress bar.
		progressInt := 0
		if progressTotal != 0 {
			progressInt = int(progressCur * int64(progressWidth) / progressTotal)
		}
		for i := 0; i < progressInt; i++ {
			writeRune('█')
		}
		for i := progressInt; i < progressWidth; i++ {
			writeRune(' ')
		}
		writeRune('◀')

		// Render progress as a percent too. Mostly to have something dynamic on
		// the right so it is visually noticeable where the progress bar ends.
		if progressTotal > 0 {
			writeString(fmt.Sprintf(" %3.f%%", float64(progressCur)/float64(progressTotal)*100.0))
		}
	}

	return buf.String(), active
}
