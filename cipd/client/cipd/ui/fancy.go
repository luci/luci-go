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
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh/terminal"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

var (
	spinnerChars = []rune("⣾⣽⣻⢿⡿⣟⣯⣷")
	progressRune = '█'
)

// FancyImplementation implements activities UI using terminal escape sequences.
type FancyImplementation struct {
	Out *os.File // where to write the output, must be a terminal

	m           sync.Mutex
	groups      int              // number of running ActivityGroups
	width       int              // the terminal width
	stop        chan struct{}    // closed to stop the rendering goroutine
	done        chan struct{}    // closed when the rendering goroutine exits
	activities  []*fancyActivity // started (and perhaps finished) activities
	activeLines int              // how many terminal lines we need to refresh
}

// NewActivityGroup is a part of Implementation interface.
func (impl *FancyImplementation) NewActivityGroup(ctx context.Context) ActivityGroup {
	impl.m.Lock()
	impl.groups++
	launch := impl.groups == 1
	impl.m.Unlock()

	if launch {
		impl.launchUI()
	}

	return &fancyActivityGroup{impl: impl}
}

func (impl *FancyImplementation) activityGroupClosed() {
	impl.m.Lock()
	impl.groups--
	stop := impl.groups == 0
	impl.m.Unlock()
	if stop {
		impl.stopUI()
	}
}

// launchUI is called to switch the terminal into VT100 mode and launch a
// goroutine that periodically renders the output.
func (impl *FancyImplementation) launchUI() {
	width, _, _ := terminal.GetSize(int(impl.Out.Fd()))
	if width == 0 {
		width = 100
	}

	stop := make(chan struct{})
	done := make(chan struct{})

	impl.m.Lock()
	impl.width = width
	impl.stop = stop
	impl.done = done
	impl.activeLines = 0
	impl.m.Unlock()

	go func() {
		defer close(done)
		for {
			impl.renderUI(true)
			select {
			case <-stop:
				impl.renderUI(false) // make sure the final state is displayed
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()
}

// stopUI is called to restore the previous terminal state.
func (impl *FancyImplementation) stopUI() {
	impl.m.Lock()
	stop := impl.stop
	done := impl.done
	impl.stop = nil
	impl.done = nil
	impl.m.Unlock()

	close(stop)
	<-done
}

// activityWokeUp is called when a new activity starts running.
//
// It is added to the list of displayed activities.
func (impl *FancyImplementation) activityWokeUp(a *fancyActivity) {
	impl.m.Lock()
	defer impl.m.Unlock()
	for _, existing := range impl.activities {
		if a == existing {
			return
		}
	}
	impl.activities = append(impl.activities, a)
}

// renderUI renders the UI and does some housekeeping.
func (impl *FancyImplementation) renderUI(advance bool) {
	impl.m.Lock()
	defer impl.m.Unlock()

	// Move the cursor up to start overwriting previously written lines.
	if impl.activeLines > 0 {
		fmt.Fprintf(impl.Out, "\033[%dA", impl.activeLines)
	}

	// How many stopped activities at the start of the list we can drop.
	isDropping := true
	droppingCount := 0

	for _, a := range impl.activities {
		line, active := a.renderLine(advance, impl.width)
		fmt.Fprintf(impl.Out, "\r\033[K%s\n", line) // overwrite the line
		if active {
			isDropping = false // can't drop this nor any following activity
		} else if isDropping {
			droppingCount++ // can stop rendering this activity in future passes
		}
	}

	// Stop updating activities that will never be changed.
	impl.activities = impl.activities[droppingCount:]

	// Tell the next renderUI call how many lines it should overwrite.
	impl.activeLines = len(impl.activities)
}

////////////////////////////////////////////////////////////////////////////////

type fancyActivityGroup struct {
	impl *FancyImplementation

	m   sync.RWMutex
	ids map[string]int
}

func (ag *fancyActivityGroup) NewActivity(ctx context.Context, kind string) Activity {
	ag.m.Lock()
	defer ag.m.Unlock()

	if ag.ids == nil {
		ag.ids = map[string]int{}
	}
	id := ag.ids[kind] + 1
	ag.ids[kind] = id

	return &fancyActivity{
		impl:   ag.impl,
		parent: ag,
		kind:   kind,
		id:     id,
	}
}

func (ag *fancyActivityGroup) Close() {
	ag.impl.activityGroupClosed()
}

func (ag *fancyActivityGroup) activityTitle(kind string, id int) string {
	ag.m.RLock()
	total := ag.ids[kind]
	ag.m.RUnlock()
	if total <= 1 {
		return kind
	}
	totalStr := fmt.Sprintf("%d", total)
	idStr := fmt.Sprintf("%d", id)
	return fmt.Sprintf("%s %s/%s", kind, padLeft(idStr, len(totalStr)), totalStr)
}

////////////////////////////////////////////////////////////////////////////////

type fancyActivity struct {
	impl   *FancyImplementation
	parent *fancyActivityGroup
	kind   string
	id     int

	m             sync.Mutex
	active        bool
	round         int
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
	wasInactive := !a.active
	a.active = true
	cb()
	a.m.Unlock()

	if wasInactive {
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

func (a *fancyActivity) Log(ctx context.Context, level logging.Level, calldepth int, f string, args []interface{}) {
	a.mutateLockedState(func() {
		a.logMessage = fmt.Sprintf(f, args...)
		a.logLevel = level
	})
}

func (a *fancyActivity) Done(ctx context.Context) {
	a.m.Lock()
	a.active = false
	a.m.Unlock()
}

// renderLine composes a status line of this activity.
//
// If `advance` is true, will "spin" the progress spinner.
func (a *fancyActivity) renderLine(advance bool, width int) (line string, active bool) {
	a.m.Lock()
	active = a.active
	round := a.round
	progressTitle := a.progressTitle
	progressCur := a.progressCur
	progressTotal := a.progressTotal
	logMessage := a.logMessage
	logLevel := a.logLevel
	speed := a.speed.speed
	if a.active && advance {
		a.round++
	}
	a.m.Unlock()

	// Construct the string with detailed progress stats.
	detailsStr := ""
	if progressTotal != 0 {
		totalStr := fmt.Sprintf("%.1f", float64(progressTotal)/1000000.0)
		curStr := fmt.Sprintf("%.1f", float64(progressCur)/1000000.0)
		speedStr := "??"
		if speed >= 0 {
			speedStr = fmt.Sprintf("%.1f", speed/1000000.0)
		}
		detailsStr = fmt.Sprintf(" %s/%s MB, %s MB/s",
			padLeft(curStr, len(totalStr)),
			totalStr,
			speedStr,
		)
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

	// The line layout is like a table row:
	// <spinner> <activityTitle> <progressTitle> <progressBar>

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

	// Write the activity title, e.g. "fetch 10/11: ".
	if a.kind != "" {
		writeString("[" + a.parent.activityTitle(a.kind, a.id) + "] ")
	}

	// If have a notice, show it instead of the progress bar.
	if logMessage != "" {
		// Need to avoid wrapping, it breaks the output.
		spaceLeft := width - runes - 1
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
		writeString(progressTitle + detailsStr + " ")

		// Too wide lines look ugly too.
		if width > 160 {
			width = 160
		}

		// Figure out how much space is left for the progress bar.
		progressWidth := width - runes - len("| 100%")
		if progressWidth < 5 {
			progressWidth = 5
		}

		// Render the progress bar.
		progressInt := 0
		if progressTotal != 0 {
			progressInt = int(progressCur * int64(progressWidth) / progressTotal)
		}
		for i := 0; i < progressInt; i++ {
			buf.WriteRune(progressRune)
		}
		for i := progressInt; i < progressWidth; i++ {
			buf.WriteByte(' ')
		}
		writeRune('◀')

		// Render percents as a number too. Mostly to have something visible on
		// the right so it is clear where the progress bar ends.
		if progressTotal > 0 {
			fmt.Fprintf(&buf, " %3.f%%", float64(progressCur)/float64(progressTotal)*100.0)
		}
	}

	return buf.String(), active
}

// padLeft pads an ASCII string with spaces on the left to make it l bytes long.
func padLeft(s string, l int) string {
	if len(s) < l {
		return strings.Repeat(" ", l-len(s)) + s
	}
	return s
}
