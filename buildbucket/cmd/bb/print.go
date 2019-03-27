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
	"github.com/mgutz/ansi"

	"go.chromium.org/luci/common/data/text/color"
	"go.chromium.org/luci/common/data/text/indented"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

var (
	ansiWhiteUnderline = ansi.ColorCode("white+u")
	ansiStatus         = map[buildbucketpb.Status]string{
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
	tab    *tabwriter.Writer
	indent indented.Writer
	w      io.Writer
}

func newPrinter(w io.Writer, colored bool) *printer {
	p := &printer{}
	p.tab = tabwriter.NewWriter(w, 0, 1, 4, ' ', 0)
	p.w = p.tab

	if !colored {
		// Put StripWriter after tabwriter.
		// tabwriter does not like strip writer.
		p.w = &color.StripWriter{Writer: p.w}
	}

	p.indent.Writer = p.w
	p.indent.UseSpaces = true
	p.w = &p.indent
	return p
}

func newStdoutPrinter(colored bool) *printer {
	return newPrinter(os.Stdout, colored)
}

// f prints a formatted message. Panics if writing fails.
func (p *printer) f(format string, args ...interface{}) {
	if _, err := fmt.Fprintf(p.w, format, args...); err != nil && err != io.ErrShortWrite {
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

	p.headerf("Build %d", b.Id)

	// Builder and build number.
	p.attr("Builder")
	p.f("%s/%s/%s", b.Builder.Project, b.Builder.Bucket, b.Builder.Builder)
	if b.Number != 0 {
		p.f(" #%d", b.Number)
	}
	p.f("\n")

	// Build status and summary.
	p.attr("Status")
	p.status(b.Status)
	if b.SummaryMarkdown != "" {
		// TODO(nodir): color markdown.
		p.f(" %s", b.SummaryMarkdown)
	}
	p.f("\n")

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
	for i, s := range b.Steps {
		if i > 0 {
			p.f("\n")
		}
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
	p.keyword("Step")
	p.fw(35, " %q", s.Name)
	p.status(s.Status)
	p.f("%s", strings.Repeat(" ", 10-len(s.Status.String())))

	start, startErr := ptypes.Timestamp(s.StartTime)
	end, endErr := ptypes.Timestamp(s.EndTime)
	if startErr == nil && endErr == nil {
		p.f("%s", end.Sub(start))
	}
	p.f("\n")

	p.indent.Level += 2
	if s.SummaryMarkdown != "" {
		// TODO(nodir): color markdown.
		// TODO(nodir): transform lists of links to look like logs
		p.f("%s\n", s.SummaryMarkdown)
	}
	for _, l := range s.Logs {
		p.f("* %s\t", l.Name)
		p.linkf("%s", l.ViewUrl)
		p.f("\n")
	}
	p.indent.Level -= 2
}

func (p *printer) status(s buildbucketpb.Status) {
	p.f("%s%s%s", ansiStatus[s], s, ansi.Reset)
}

func (p *printer) buildTime(b *buildbucketpb.Build) {
	created := localTimestamp(b.CreateTime)
	if created.IsZero() {
		return
	}
	p.keyword("Created")
	p.f(" ")
	p.dateTime(created)

	started := localTimestamp(b.StartTime)
	if !started.IsZero() {
		p.f(", ")
		p.keyword("waited")
		p.f(" %s, ", started.Sub(created))
		p.keyword("started")
		p.f(" ")
		p.time(started)
	}

	ended := localTimestamp(b.EndTime)
	if !ended.IsZero() {
		if !started.IsZero() {
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

func (p *printer) dateTime(t time.Time) {
	p.date(t)
	p.f(" ")
	p.time(t)
}

func (p *printer) date(t time.Time) {
	if isToday(t) {
		p.f("today")
	} else {
		p.f("on %s", t.Format("2006-01-02"))
	}
}

func (p *printer) time(t time.Time) {
	if time.Now().Sub(t) < 10*time.Second {
		p.f("just now")
	} else {
		p.f("at %s", t.Format("15:04:05"))
	}
}

func (p *printer) attr(s string) {
	p.keyword(s)
	p.f(": ")
}

func (p *printer) keyword(s string) {
	p.f("%s%s%s", ansi.Blue, s, ansi.Reset)
}

func (p *printer) headerf(format string, args ...interface{}) {
	p.f("%s", ansi.Yellow)
	p.f(format, args...)
	p.f("%s\n", ansi.Reset)
}

func (p *printer) linkf(format string, args ...interface{}) {
	p.f("%s", ansiWhiteUnderline)
	p.f(format, args...)
	p.f("%s", ansi.Reset)
}

// localTimestamp converts ts to local time.Time.
// Returns zero if ts is invalid.
func localTimestamp(ts *timestamp.Timestamp) time.Time {
	t, err := ptypes.Timestamp(ts)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

func isToday(t time.Time) bool {
	tYear, tMonth, tDay := t.Local().Date()
	nYear, nMonth, nDay := time.Now().Date()
	return tYear == nYear && tMonth == nMonth && tDay == nDay
}
