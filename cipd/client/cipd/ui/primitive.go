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
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

const progressReportInterval = 500 * time.Millisecond

// primitiveActivity just logs its progress into the default logger.
//
// Always logs 0% and 100% progress reports and jumps back. For normal forward
// progress throttles messages per progressReportInterval.
//
// TODO(vadimsh): Only UnitBytes are supported currently.
type primitiveActivity struct {
	logger logging.Factory // wrapped logger

	parent *primitiveActivityGroup
	kind   string
	id     int

	m          sync.Mutex
	lastReport time.Time
	lastTitle  string
	lastUnits  Units
	lastCur    int64
	lastTotal  int64
	speed      speedGauge
}

func (a *primitiveActivity) Progress(ctx context.Context, title string, units Units, cur, total int64) {
	now := clock.Now(ctx)

	a.m.Lock()

	// True if made any progress since the last call.
	advanced := a.lastCur != cur
	// We log every title change for progress 0% and 100%, they are important.
	titleChange := title != a.lastTitle

	// Reset the speed gauge when the progress is restarted.
	reset := a.lastCur > cur || a.lastTotal != total || a.lastUnits != units
	if reset {
		a.speed.reset(now, cur)
	} else {
		a.speed.advance(now, cur)
	}
	speed := a.speed.speed

	a.lastTitle = title
	a.lastUnits = units
	a.lastCur = cur
	a.lastTotal = total

	// Report to log on "interesting" events and also periodically.
	reportNow := (cur == 0 && titleChange) ||
		(cur == total && (advanced || titleChange)) ||
		reset ||
		now.Sub(a.lastReport) > progressReportInterval
	if reportNow {
		a.lastReport = now
	}

	a.m.Unlock()

	if !reportNow {
		return
	}

	totalStr := fmt.Sprintf("%.1f", float64(total)/1000000.0)
	curStr := fmt.Sprintf("%.1f", float64(cur)/1000000.0)
	if len(curStr) < len(totalStr) {
		curStr = strings.Repeat(" ", len(totalStr)-len(curStr)) + curStr
	}

	var details []string
	if total != 0 {
		details = append(details, fmt.Sprintf("%3.f%%", float64(cur)/float64(total)*100.0))
	}
	if speed >= 0 {
		details = append(details, fmt.Sprintf("%.2f MB/s", speed/1000000.0))
	}
	detailsStr := strings.Join(details, ", ")
	if detailsStr != "" {
		detailsStr = " (" + detailsStr + ")"
	}

	// Use LogCall to pass non-default calldepth. Otherwise all logs appear as
	// coming from Progress function, which is not very useful.
	logging.Get(ctx).LogCall(logging.Info, 1, "%s: %s/%s MB%s",
		[]interface{}{title, curStr, totalStr, detailsStr},
	)
}

func (a *primitiveActivity) Log(ctx context.Context, level logging.Level, calldepth int, f string, args []interface{}) {
	a.logger(ctx).LogCall(level, calldepth+1, a.logPrefix()+f, args)
}

func (a *primitiveActivity) logPrefix() string {
	if a.parent != nil {
		return a.parent.logPrefix(a.kind, a.id)
	}
	return ""
}

// speedGauge measures speed using exponential averaging.
type speedGauge struct {
	speed   float64 // the last measured speed or -1 if not yet known
	prevTS  time.Time
	prevVal int64
	samples int
}

func (s *speedGauge) reset(ts time.Time, val int64) {
	s.speed = -1
	s.prevTS = ts
	s.prevVal = val
	s.samples = 0
}

func (s *speedGauge) advance(ts time.Time, val int64) {
	dt := ts.Sub(s.prevTS)
	if dt < 200*time.Millisecond {
		return // too soon
	}

	v := float64(val-s.prevVal) / dt.Seconds()

	s.prevTS = ts
	s.prevVal = val
	s.samples++

	// Apply exponential average. Take the first sample as a base.
	if s.samples == 1 {
		s.speed = v
	} else {
		s.speed = 0.07*v + 0.93*s.speed
	}
}

// primitiveActivityGroup just adds a unique log prefix to every activity.
type primitiveActivityGroup struct {
	m   sync.RWMutex
	ids map[string]int
}

func (p *primitiveActivityGroup) Close() {}

func (p *primitiveActivityGroup) NewActivity(ctx context.Context, kind string) Activity {
	p.m.Lock()
	defer p.m.Unlock()

	if p.ids == nil {
		p.ids = map[string]int{}
	}
	id := p.ids[kind] + 1
	p.ids[kind] = id

	return &primitiveActivity{
		logger: logging.GetFactory(ctx),
		parent: p,
		kind:   kind,
		id:     id,
	}
}

func (p *primitiveActivityGroup) logPrefix(kind string, id int) string {
	p.m.RLock()
	total := p.ids[kind]
	p.m.RUnlock()
	if total <= 1 {
		return fmt.Sprintf("[%s]", kind)
	}
	return fmt.Sprintf("[%s %d/%d] ", kind, id, total)
}
