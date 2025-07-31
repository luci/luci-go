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

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/mgutz/ansi"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/text/color"
	"go.chromium.org/luci/common/data/text/indented"

	bb "go.chromium.org/luci/buildbucket"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

var (
	ansiWhiteBold      = ansi.ColorCode("white+b")
	ansiWhiteUnderline = ansi.ColorCode("white+u")
	ansiStatus         = map[pb.Status]string{
		pb.Status_SCHEDULED:     ansi.LightWhite,
		pb.Status_STARTED:       ansi.LightYellow,
		pb.Status_SUCCESS:       ansi.LightGreen,
		pb.Status_FAILURE:       ansi.LightRed,
		pb.Status_INFRA_FAILURE: ansi.LightMagenta,
	}
)

// printer can print a buildbucket build to a io.Writer in a human-friendly
// format.
//
// First time writing fails, the error is saved to Err.
// Further attempts to write are noop.
type printer struct {
	// Err is not nil if printing to the writer failed.
	// If it is not nil, methods are noop.
	Err error

	nowFn func() time.Time

	// used to indent text. printer.f always writes to this writer.
	indent indented.Writer
}

func newPrinter(w io.Writer, disableColor bool, nowFn func() time.Time) *printer {
	// Stack writers together.
	// w always points to the stack top.

	p := &printer{nowFn: nowFn}

	if disableColor {
		w = &color.StripWriter{Writer: w}
	}

	p.indent.Writer = w
	p.indent.UseSpaces = true
	return p
}

func newStdioPrinters(disableColor bool) (stdout, stderr *printer) {
	disableColor = disableColor || shouldDisableColors()
	stdout = newPrinter(os.Stdout, disableColor, time.Now)
	stderr = newPrinter(os.Stderr, disableColor, time.Now)
	return
}

// f prints a formatted message.
func (p *printer) f(format string, args ...any) {
	if p.Err != nil {
		return
	}
	if _, err := fmt.Fprintf(&p.indent, format, args...); err != nil && err != io.ErrShortWrite {
		p.Err = err
	}
}

// fw is like f, but appends whitespace such that the printed string takes at
// least minWidth.
// Appends at least one space.
func (p *printer) fw(minWidth int, format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	pad := minWidth - utf8.RuneCountInString(s)
	if pad < 1 {
		pad = 1
	}
	p.f("%s%s", s, strings.Repeat(" ", pad))
}

// JSONPB prints pb in either compact or indented JSON format according to the
// provided compact boolean
func (p *printer) JSONPB(pb proto.Message, compact bool) {
	m := &jsonpb.Marshaler{}
	buf := &bytes.Buffer{}
	if err := m.Marshal(buf, pb); err != nil {
		panic(fmt.Errorf("failed to marshal a message: %s", err))
	}

	out := &bytes.Buffer{}
	var err error
	if compact {
		err = json.Compact(out, buf.Bytes())
	} else {
		// Note: json.Marshal indents JSON more nicely than jsonpb.Marshaler.Indent.
		err = json.Indent(out, buf.Bytes(), "", "  ")
	}
	if err != nil {
		panic(err)
	}

	p.f("%s\n", out.Bytes())
}

// Build prints b. Panic when id, status or any fields under builder is missing
func (p *printer) Build(b *pb.Build, host string) {
	// Id, Status and Builder are explicitly added to field mask so they should
	// always be present. Doing defensive check here to avoid any unexpected
	// conditions to corrupt the printed build.
	if b.Id == 0 {
		panic(fmt.Errorf("expect non zero id present in the build"))
	}
	if b.Status == pb.Status_STATUS_UNSPECIFIED {
		panic(fmt.Errorf("expect non zero value status present in the build"))
	}
	if builder := b.Builder; builder == nil ||
		builder.Project == "" || builder.Bucket == "" || builder.Builder == "" {
		panic(fmt.Errorf("expect builder present in the build and all fields under builder should be non zero value. Got: %v", builder))
	}

	// Print the build URL bold, underline and a color matching the status.
	if host == "" {
		host = "cr-buildbucket.appspot.com"
	}
	p.f("%s%s%shttp://%s/build/%d", ansiWhiteBold, ansiWhiteUnderline, ansiStatus[b.Status], host, b.Id)
	// Undo underline.
	p.f("%s%s%s ", ansi.Reset, ansiWhiteBold, ansiStatus[b.Status])
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

	experiments := stringset.NewFromSlice(b.Input.GetExperiments()...)

	var systemTags []string
	if experiments.Has(bb.ExperimentNonProduction) {
		systemTags = append(systemTags, "Experimental")
	}
	if experiments.Has(bb.ExperimentBBCanarySoftware) {
		systemTags = append(systemTags, "Canary")
	}
	if len(systemTags) > 0 {
		for i, t := range systemTags {
			if i > 0 {
				p.f(" ")
			}
			p.keyword(t)
		}
		p.f("\n")
	}

	// Timing.
	if b.CreateTime != nil {
		p.buildTime(b)
		p.f("\n")
	}

	if b.CreatedBy != "" {
		p.attr("By")
		p.f("%s\n", b.CreatedBy)
	}

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
		p.JSONPB(props, false)
	}

	if props := b.Output.GetProperties(); props != nil {
		p.attr("Output properties")
		p.JSONPB(props, false)
	}

	// Experiments
	if exps := b.Input.GetExperiments(); len(exps) > 0 {
		p.attr("Experiments")
		p.f("\n")
		p.indent.Level += 2
		for _, exp := range exps {
			p.f("%s\n", exp)
		}
		p.indent.Level -= 2
	}

	// Steps
	p.steps(b.Steps)
}

// commit prints c.
func (p *printer) commit(c *pb.GitilesCommit) {
	if c.Id == "" {
		p.linkf("https://%s/%s/+/%s", c.Host, c.Project, c.Ref)
		return
	}

	switch c.Host {
	// This shamelessly hardcodes https://cr-rev.appspot.com/_ah/api/crrev/v1/projects response
	// TODO(nodir): make an RPC and cache on the file system.
	case
		"aomedia.googlesource.com",
		"boringssl.googlesource.com",
		"chromium.googlesource.com",
		"gerrit.googlesource.com",
		"webrtc.googlesource.com":
		p.linkf("https://crrev.com/%s", c.Id)
	default:
		p.linkf("https://%s/%s/+/%s", c.Host, c.Project, c.Id)
	}
	if c.Ref != "" {
		p.f(" on %s", c.Ref)
	}
}

// change prints cl.
func (p *printer) change(cl *pb.GerritChange) {
	switch {
	case cl.Host == "chromium-review.googlesource.com":
		p.linkf("https://crrev.com/c/%d/%d", cl.Change, cl.Patchset)
	case cl.Host == "chrome-internal-review.googlesource.com":
		p.linkf("https://crrev.com/i/%d/%d", cl.Change, cl.Patchset)
	default:
		p.linkf("https://%s/c/%s/+/%d/%d", cl.Host, cl.Project, cl.Change, cl.Patchset)
	}
}

// steps print steps.
func (p *printer) steps(steps []*pb.Step) {
	maxNameWidth := 0
	for _, s := range steps {
		if w := utf8.RuneCountInString(s.Name); w > maxNameWidth {
			maxNameWidth = w
		}
	}

	for _, s := range steps {
		p.f("%sStep ", ansiStatus[s.Status])
		p.fw(maxNameWidth+5, "%q", s.Name)
		p.fw(10, "%s", s.Status)

		// Print duration.
		durString := ""
		if start := s.StartTime.AsTime(); s.StartTime != nil {
			var stepDur time.Duration
			if end := s.EndTime.AsTime(); s.EndTime != nil {
				stepDur = end.Sub(start)
			} else {
				now := p.nowFn()
				stepDur = now.Sub(start.In(now.Location()))
			}
			durString = truncateDuration(stepDur).String()
		}
		p.fw(10, "%s", durString)

		// Print log names.
		// Do not print log URLs because they are very long and
		// bb has `log` subcommand.
		if len(s.Logs) > 0 {
			p.f("Logs: ")
			for i, l := range s.Logs {
				if i > 0 {
					p.f(", ")
				}
				p.f("%q", l.Name)
			}
		}

		p.f("%s\n", ansi.Reset)

		p.indent.Level += 2
		if s.SummaryMarkdown != "" {
			// TODO(nodir): transform lists of links to look like logs
			p.summary(s.SummaryMarkdown)
		}
		p.indent.Level -= 2
	}
}

func (p *printer) buildTime(b *pb.Build) {
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

	// Since the `fields` part of the request is partially controlled by the user,
	// the nil timestamp fields in fetched build aren't trustworthy.
	// So, check by `status` first,which is always set, and only then try to print
	// additional information if semantically fitting.

	if b.Status == pb.Status_SCHEDULED {
		if !p.isJustNow(created) {
			p.f(", ")
			p.keyword("waiting")
			p.f(" for %s, ", truncateDuration(now.Sub(created)))
		}
		return
	}

	if !started.IsZero() {
		p.f(", ")
		p.keyword("waited")
		p.f(" %s, ", truncateDuration(started.Sub(created)))
		p.keyword("started")
		p.f(" ")
		p.time(started)
	}

	if !protoutil.IsEnded(b.Status) {
		if !started.IsZero() {
			// running now
			p.f(", ")
			p.keyword("running")
			p.f(" for %s", truncateDuration(now.Sub(started)))
		}
	} else {
		// ended
		if !started.IsZero() {
			// started in the past
			p.f(", ")
			p.keyword("ran")
			p.f(" for %s", truncateDuration(ended.Sub(started)))
		}
		if !ended.IsZero() {
			p.f(", ")
			p.keyword("ended")
			p.f(" ")
			p.time(ended)
		}
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
	elapsed := now.Sub(t.In(now.Location()))
	return elapsed > 0 && elapsed < 10*time.Second
}

func (p *printer) attr(s string) {
	p.keyword(s)
	p.f(": ")
}

func (p *printer) keyword(s string) {
	p.f("%s%s%s", ansiWhiteBold, s, ansi.Reset)
}

func (p *printer) linkf(format string, args ...any) {
	p.f("%s", ansiWhiteUnderline)
	p.f(format, args...)
	p.f("%s", ansi.Reset)
}

// Error prints the err. If err is a gRPC error, then prints only the message
// without the code.
func (p *printer) Error(err error) {
	st, _ := status.FromError(err)
	p.f("%s", st.Message())
}

// readTimestamp converts ts to time.Time.
// Returns zero if ts is invalid.
func readTimestamp(ts *timestamppb.Timestamp) time.Time {
	if err := ts.CheckValid(); err != nil {
		return time.Time{}
	}
	t := ts.AsTime()
	return t
}

func (p *printer) isToday(t time.Time) bool {
	now := p.nowFn()
	nYear, nMonth, nDay := now.Date()
	tYear, tMonth, tDay := t.In(now.Location()).Date()
	return tYear == nYear && tMonth == nMonth && tDay == nDay
}

var durTransactions = []struct{ threshold, round time.Duration }{
	{time.Hour, time.Minute},
	{time.Minute, time.Second},
	{time.Second, time.Second / 10},
	{time.Millisecond, time.Millisecond},
}

// truncateDuration truncates d to make it more human-readable.
func truncateDuration(d time.Duration) time.Duration {
	for _, t := range durTransactions {
		if d > t.threshold {
			return d.Round(t.round)
		}
	}
	return d
}
