// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Group int
type Section int

// Progress outputs information about the progress of a long task.
//
// It's implementation must be thread safe.
type Progress interface {
	io.Closer
	// Update increases the count of a column.
	Update(group Group, section Section, count int)
}

// NewProgress returns an initialized thread-safe Progress implementation.
//
// columns is the number of stages each item must go through, then with a set
// of numbers for each state, which will be displayed as a number in each box.
//
// For:
//   columns = [][]string{{"found"}, {"to hash", "hashed"}}
// It'll print:
//   [found] [to hash/hashed]
func NewProgress(columns [][]string, out io.Writer) Progress {
	p := &progress{
		start:    time.Now().UTC(),
		columns:  columns,
		interval: time.Second,
		EOL:      "\n",
		out:      out,
		values:   make([][]int, len(columns)),
	}
	if IsTerminal(out) {
		p.interval = 50 * time.Millisecond
		p.EOL = "\r"
	}
	for i, c := range columns {
		p.values[i] = make([]int, len(c))
	}
	if out != nil {
		go p.printLoop()
	}
	return p
}

// Private stuff.

type progress struct {
	// Immutable.
	start    time.Time
	columns  [][]string
	interval time.Duration
	EOL      string
	out      io.Writer

	// Mutable.
	lock         sync.Mutex
	values       [][]int
	valueChanged bool
}

func (p *progress) Update(group Group, section Section, count int) {
	go func() {
		p.lock.Lock()
		defer p.lock.Unlock()
		p.values[group][section] += count
		p.valueChanged = true
	}()
}

func (p *progress) Close() error {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.out = nil
	return nil
}

func (p *progress) printLoop() {
	line := renderGroups(p.columns) + "\n"
	p.lock.Lock()
	out := p.out
	p.lock.Unlock()
	io.WriteString(out, line)

	for {
		p.lock.Lock()
		out, line := p.printStep()
		p.lock.Unlock()
		if out == nil {
			return
		}
		if line != "" {
			if _, err := io.WriteString(out, line); err != nil {
				return
			}
		}
		time.Sleep(p.interval)
	}
}

func (p *progress) printStep() (io.Writer, string) {
	if p.out == nil || !p.valueChanged {
		return p.out, ""
	}
	p.valueChanged = false
	// Zap resolution at .1s level. We're slow anyway.
	duration := Round(time.Since(p.start), 100*time.Millisecond)
	return p.out, fmt.Sprintf("%s %s%s", renderGroupsInt(p.values), duration, p.EOL)
}

func renderSection(section []string) string {
	return "[" + strings.Join(section, "/") + "]"
}

func renderGroups(c [][]string) string {
	sections := make([]string, len(c))
	for i, section := range c {
		sections[i] = renderSection(section)
	}
	return strings.Join(sections, " ")
}

func renderSectionInt(section []int) string {
	parts := make([]string, len(section))
	for i, part := range section {
		parts[i] = strconv.Itoa(part)
	}
	return "[" + strings.Join(parts, "/") + "]"
}

func renderGroupsInt(c [][]int) string {
	sections := make([]string, len(c))
	for i, section := range c {
		sections[i] = renderSectionInt(section)
	}
	return strings.Join(sections, " ")
}
