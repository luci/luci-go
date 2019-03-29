// Copyright 2019 The LUCI Authors.
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

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	isatty "github.com/mattn/go-isatty"
	"github.com/mgutz/ansi"

	"go.chromium.org/luci/common/data/text/color"
	"go.chromium.org/luci/common/data/text/indented"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

var (
	ansiWhiteBold      = ansi.ColorCode("white+b")
	ansiWhiteUnderline = ansi.ColorCode("white+u")
	ansiStatus         = map[buildbucketpb.Status]string{
		buildbucketpb.Status_SCHEDULED:     ansi.LightWhite,
		buildbucketpb.Status_STARTED:       ansi.Yellow,
		buildbucketpb.Status_SUCCESS:       ansi.Green,
		buildbucketpb.Status_FAILURE:       ansi.Red,
		buildbucketpb.Status_INFRA_FAILURE: ansi.Magenta,
	}
)

// printer can print a buildbucket build to a io.Writer in a human-friendly
// format.
// Panics if writing fails.
type printer struct {
	nowFn func() time.Time

	// used to align lines that have tabs
	// tab.Flush() must be called before exiting public methods.
	tab *tabwriter.Writer
	// used to indent text. printer.f always writes to this writer.
	indent indented.Writer
}

func newPrinter(w io.Writer, disableColor bool, nowFn func() time.Time) *printer {
	// Stack writers together.
	// w always points to the stack top.

	p := &printer{nowFn: nowFn}
	p.tab = tabwriter.NewWriter(w, 0, 1, 4, ' ', 0)
	w = p.tab

	if disableColor {
		// note: tabwriter does not like stripwriter,
		// so strip writer must come after tabwriter.
		w = &color.StripWriter{Writer: w}
	}

	p.indent.Writer = w
	p.indent.UseSpaces = true
	return p
}

func newStdoutPrinter(disableColor bool) *printer {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		disableColor = true
	}
	return newPrinter(os.Stdout, disableColor, time.Now)
}

// f prints a formatted message. Panics if writing fails.
func (p *printer) f(format string, args ...interface{}) {
	if _, err := fmt.Fprintf(&p.indent, format, args...); err != nil && err != io.ErrShortWrite {
		panic(err)
	}
}

// fw is like f, but appends whitespace such that the printed string takes at
// least minWidth.
// Appends at least one space.
func (p *printer) fw(minWidth int, format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	pad := minWidth - utf8.RuneCountInString(s)
	if pad < 1 {
		pad = 1
	}
	p.f("%s%s", s, strings.Repeat(" ", pad))
}

// JSONPB prints pb in JSON format, indented.
func (p *printer) JSONPB(pb proto.Message) {
	m := &jsonpb.Marshaler{}
	buf := &bytes.Buffer{}
	if err := m.Marshal(buf, pb); err != nil {
		panic(fmt.Errorf("failed to marshal a message: %s", err))
	}

	// Note: json.Marshal indents JSON more nicely than jsonpb.Marshaler.Indent.
	indented := &bytes.Buffer{}
	if err := json.Indent(indented, buf.Bytes(), "", "  "); err != nil {
		panic(err)
	}
	p.f("%s\n", indented.Bytes())
}

// Build prints b.
func (p *printer) Build(b *buildbucketpb.Build) {
	defer p.tab.Flush()
	p.f("%sBuild %d ", ansiStatus[b.Status], b.Id)
	p.fw(10, "%s", b.Status)
	p.f("'%s/%s/%s", b.Builder.Project, b.Builder.Bucket, b.Builder.Builder)
	if b.Number != 0 {
		p.f("/%d", b.Number)
	}
	p.f("'%s\n", ansi.Reset)

	// Summary.
	if b.SummaryMarkdown != "" {
		p.attr("Summary")
		p.summary(b.SummaryMarkdown)
	}

	// Timing.
	p.buildTime(b)
	p.f("\n")

	// Commit, CLs and tags.
	if c := b.Input.GetGitilesCommit(); c != nil {
		p.attr("Commit")
		p.commit(c)
		p.f("\n")
	}
	for _, cl := range b.Input.GetGerritChanges() {
		p.attr("CL")
		p.change(cl)
		p.f("\n")
	}
	for _, t := range b.Tags {
		p.attr("Tag")
		p.f("%s:%s\n", t.Key, t.Value)
	}

	// Properties
	if props := b.Input.GetProperties(); props != nil {
		p.attr("Input properties")
		p.JSONPB(props)
	}

	if props := b.Output.GetProperties(); props != nil {
		p.attr("Output properties")
		p.JSONPB(props)
	}

	// Steps
	for _, s := range b.Steps {
		p.step(s)
	}
}

// commit prints c.
func (p *printer) commit(c *buildbucketpb.GitilesCommit) {
	if c.Id == "" {
		p.linkf("https://%s/%s/+/%s", c.Host, c.Project, c.Ref)
		return
	}

	p.linkf("https://%s/%s/+/%s", c.Host, c.Project, c.Id)
	if c.Ref != "" {
		p.f(" on %s", c.Ref)
	}
}

// change prints cl.
func (p *printer) change(cl *buildbucketpb.GerritChange) {
	p.linkf("https://%s/c/%s/+/%d/%d", cl.Host, cl.Project, cl.Change, cl.Patchset)
}

// step prints s.
func (p *printer) step(s *buildbucketpb.Step) {
	p.f("%s", ansiStatus[s.Status])
	p.fw(60, "Step %q", s.Name)
	p.fw(10, "%s", s.Status)

	start, startErr := ptypes.Timestamp(s.StartTime)
	end, endErr := ptypes.Timestamp(s.EndTime)
	if startErr == nil && endErr == nil {
		p.f("%s", end.Sub(start))
	}
	p.f("%s\n", ansi.Reset)

	p.indent.Level += 2
	if s.SummaryMarkdown != "" {
		// TODO(nodir): transform lists of links to look like logs
		p.summary(s.SummaryMarkdown)
	}
	for _, l := range s.Logs {
		p.f("* %s\t", l.Name)
		p.linkf("%s", l.ViewUrl)
		p.f("\n")
	}
	p.indent.Level -= 2
}

func (p *printer) buildTime(b *buildbucketpb.Build) {
	now := p.nowFn()
	created := readTimestamp(b.CreateTime).In(now.Location())
	started := readTimestamp(b.StartTime).In(now.Location())
	ended := readTimestamp(b.EndTime).In(now.Location())

	if created.IsZero() {
		return
	}
	p.keyword("Created")
	p.f(" ")
	p.dateTime(created)

	if started.IsZero() && ended.IsZero() {
		// did not start or end yet
		p.f(", ")
		p.keyword("waiting")
		p.f(" for %s, ", now.Sub(created))
		return
	}

	if !started.IsZero() {
		// did not start yet
		p.f(", ")
		p.keyword("waited")
		p.f(" %s, ", started.Sub(created))
		p.keyword("started")
		p.f(" ")
		p.time(started)
	}

	if ended.IsZero() {
		// did not end yet
		if !started.IsZero() {
			// running now
			p.f(", ")
			p.keyword("running")
			p.f(" for %s", now.Sub(started))
		}
	} else {
		// ended
		if !started.IsZero() {
			// started in the past
			p.f(", ")
			p.keyword("ran")
			p.f(" for %s", ended.Sub(started))
		}

		p.f(", ")
		p.keyword("ended")
		p.f(" ")
		p.time(ended)
	}
}

func (p *printer) summary(summaryMarkdown string) {
	// TODO(nodir): color markdown.
	p.f("%s\n", strings.TrimSpace(summaryMarkdown))
}

func (p *printer) dateTime(t time.Time) {
	if p.isJustNow(t) {
		p.f("just now")
	} else {
		p.date(t)
		p.f(" ")
		p.time(t)
	}
}

func (p *printer) date(t time.Time) {
	if p.isToday(t) {
		p.f("today")
	} else {
		p.f("on %s", t.Format("2006-01-02"))
	}
}

func (p *printer) time(t time.Time) {
	if p.isJustNow(t) {
		p.f("just now")
	} else {
		p.f("at %s", t.Format("15:04:05"))
	}
}

func (p *printer) isJustNow(t time.Time) bool {
	now := p.nowFn()
	ellapsed := now.Sub(t.In(now.Location()))
	return ellapsed > 0 && ellapsed < 10*time.Second
}

func (p *printer) attr(s string) {
	p.keyword(s)
	p.f(": ")
}

func (p *printer) keyword(s string) {
	p.f("%s%s%s", ansiWhiteBold, s, ansi.Reset)
}

func (p *printer) linkf(format string, args ...interface{}) {
	p.f("%s", ansiWhiteUnderline)
	p.f(format, args...)
	p.f("%s", ansi.Reset)
}

// readTimestamp converts ts to time.Time.
// Returns zero if ts is invalid.
func readTimestamp(ts *timestamp.Timestamp) time.Time {
	t, err := ptypes.Timestamp(ts)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (p *printer) isToday(t time.Time) bool {
	now := p.nowFn()
	nYear, nMonth, nDay := now.Date()
	tYear, tMonth, tDay := t.In(now.Location()).Date()
	return tYear == nYear && tMonth == nMonth && tDay == nDay
}
