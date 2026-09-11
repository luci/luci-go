// Copyright 2026 The LUCI Authors.
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

package build

import (
	"fmt"
	"io"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// FormatOptions controls how a build is formatted for terminal display.
type FormatOptions struct {
	// AllSteps prints the full hierarchical step tree rather than just failed steps.
	AllSteps bool
	// FailedSteps forces printing only failed steps (default when build has failures).
	FailedSteps bool
	// Properties prints input and output properties.
	Properties bool
}

// FormatBuild formats a Build message into human-readable terminal output.
func FormatBuild(w io.Writer, b *pb.Build, opts FormatOptions) error {
	// 1. Builder and Number
	builderStr := ""
	if b.Builder != nil {
		builderStr = fmt.Sprintf("%s/%s/%s", b.Builder.Project, b.Builder.Bucket, b.Builder.Builder)
		if b.Number != 0 {
			builderStr = fmt.Sprintf("%s/%d", builderStr, b.Number)
		}
	}
	if builderStr != "" {
		if b.Number != 0 {
			fmt.Fprintf(w, "Build:         %s\n", builderStr)
		} else {
			fmt.Fprintf(w, "Builder:       %s\n", builderStr)
		}
	}

	// 2. Status
	statusStr := b.Status.String()
	if b.StatusDetails != nil {
		if b.StatusDetails.GetTimeout() != nil {
			statusStr += " (Timeout)"
		} else if b.StatusDetails.GetResourceExhaustion() != nil {
			statusStr += " (Resource Exhaustion)"
		}
	}
	fmt.Fprintf(w, "Status:        %s\n", statusStr)

	// 3. Build ID
	if b.Id != 0 {
		fmt.Fprintf(w, "Build ID:      %d\n", b.Id)
	}

	// 4. Timing
	if b.CreateTime != nil || b.StartTime != nil || b.EndTime != nil {
		timingStr := formatTiming(b)
		if timingStr != "" {
			fmt.Fprintf(w, "Timing:        %s\n", timingStr)
		}
	}

	// 5. Code context: Gitiles commit and Gerrit CLs
	if gc := b.Input.GetGitilesCommit(); gc != nil && (gc.Id != "" || gc.Ref != "") {
		target := gc.Id
		if target == "" {
			target = gc.Ref
		}
		fmt.Fprintf(w, "Commit:        https://%s/%s/+/%s\n", gc.Host, gc.Project, target)
	}
	for _, cl := range b.Input.GetGerritChanges() {
		clURL := ""
		switch cl.Host {
		case "chromium-review.googlesource.com":
			clURL = fmt.Sprintf("https://crrev.com/c/%d/%d", cl.Change, cl.Patchset)
		case "chrome-internal-review.googlesource.com":
			clURL = fmt.Sprintf("https://crrev.com/i/%d/%d", cl.Change, cl.Patchset)
		default:
			clURL = fmt.Sprintf("https://%s/c/%s/+/%d/%d", cl.Host, cl.Project, cl.Change, cl.Patchset)
		}
		fmt.Fprintf(w, "Gerrit CL:     %s\n", clURL)
	}

	if b.CreatedBy != "" {
		fmt.Fprintf(w, "Created By:    %s\n", b.CreatedBy)
	}

	// 6. Bot ID if available
	if sw := b.GetInfra().GetSwarming(); sw != nil {
		for _, dim := range sw.BotDimensions {
			if dim.Key == "id" && len(dim.Value) > 0 {
				fmt.Fprintf(w, "Bot ID:        %s\n", dim.Value)
				break
			}
		}
	}

	// 7. ResultDB Invocation
	inv := ""
	if rdb := b.GetInfra().GetResultdb(); rdb != nil && rdb.Invocation != "" {
		inv = rdb.Invocation
	} else if b.Id != 0 {
		inv = fmt.Sprintf("rootInvocations/build-%d", b.Id)
	}
	if inv != "" {
		if !strings.HasPrefix(inv, "rootInvocations/") {
			inv = "rootInvocations/" + strings.TrimPrefix(inv, "invocations/")
			inv = strings.Replace(inv, "build:", "build-", 1)
		}
		fmt.Fprintf(w, "Invocation:    %s\n", inv)
	}

	// 8. Summary Markdown
	if b.SummaryMarkdown != "" {
		fmt.Fprintf(w, "\nSummary:\n")
		for _, line := range strings.Split(strings.TrimSpace(b.SummaryMarkdown), "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}

	// 9. Steps
	hasFailures := false
	if len(b.Steps) > 0 {
		hasFailures = formatSteps(w, b.Steps, opts)
	}

	// 10. Input / Output Properties
	if opts.Properties {
		if props := b.Input.GetProperties(); props != nil {
			out, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(props)
			if err == nil {
				fmt.Fprintf(w, "\nInput Properties:\n%s\n", string(out))
			}
		}
		if props := b.Output.GetProperties(); props != nil {
			out, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(props)
			if err == nil {
				fmt.Fprintf(w, "\nOutput Properties:\n%s\n", string(out))
			}
		}
	}

	// 11. Helpful guidance tips
	if inv != "" && (hasFailures || b.Status == pb.Status_FAILURE || b.Status == pb.Status_INFRA_FAILURE) {
		cleanInv := strings.TrimPrefix(inv, "rootInvocations/")
		cleanInv = strings.TrimPrefix(cleanInv, "invocations/")
		fmt.Fprintf(w, "\n(Tip: To list failed test verdicts, run: luci verdict list -invocationid %s)\n", cleanInv)
	}
	if !opts.AllSteps && len(b.Steps) > 0 && hasFailures {
		if b.Id != 0 {
			fmt.Fprintf(w, "(Tip: To view all steps, run: luci build get -buildid %d -steps)\n", b.Id)
		}
	}

	return nil
}

func formatTiming(b *pb.Build) string {
	var created, started, ended time.Time
	if b.CreateTime != nil {
		created = b.CreateTime.AsTime().UTC()
	}
	if b.StartTime != nil {
		started = b.StartTime.AsTime().UTC()
	}
	if b.EndTime != nil {
		ended = b.EndTime.AsTime().UTC()
	}

	formatSubsequent := func(t, base time.Time) string {
		if !base.IsZero() && t.Year() == base.Year() && t.YearDay() == base.YearDay() {
			return t.Format("15:04:05")
		}
		return t.Format("2006-01-02 15:04:05")
	}

	switch {
	case !started.IsZero() && !ended.IsZero():
		dur := ended.Sub(started)
		var details []string
		if !created.IsZero() {
			details = append(details, fmt.Sprintf("Created: %s", created.Format("2006-01-02 15:04:05 UTC")))
			details = append(details, fmt.Sprintf("Started: %s", formatSubsequent(started, created)))
		} else {
			details = append(details, fmt.Sprintf("Started: %s UTC", started.Format("2006-01-02 15:04:05")))
		}
		details = append(details, fmt.Sprintf("Ended: %s", formatSubsequent(ended, started)))
		return fmt.Sprintf("%s (%s)", formatDuration(dur), strings.Join(details, ", "))
	case !started.IsZero() && ended.IsZero():
		dur := time.Since(started)
		var details []string
		if !created.IsZero() {
			details = append(details, fmt.Sprintf("Created: %s", created.Format("2006-01-02 15:04:05 UTC")))
			details = append(details, fmt.Sprintf("Started: %s", formatSubsequent(started, created)))
		} else {
			details = append(details, fmt.Sprintf("Started: %s UTC", started.Format("2006-01-02 15:04:05")))
		}
		return fmt.Sprintf("Running for %s (%s)", formatDuration(dur), strings.Join(details, ", "))
	case !created.IsZero():
		return fmt.Sprintf("Scheduled at %s", created.Format("2006-01-02 15:04:05 UTC"))
	default:
		return ""
	}
}

func formatSteps(w io.Writer, steps []*pb.Step, opts FormatOptions) bool {
	var passed, failed, infraFailed, started, canceled int
	var failedSteps []*pb.Step
	var startedSteps []*pb.Step

	for _, s := range steps {
		switch s.Status {
		case pb.Status_SUCCESS:
			passed++
		case pb.Status_FAILURE:
			failed++
			failedSteps = append(failedSteps, s)
		case pb.Status_INFRA_FAILURE:
			infraFailed++
			failedSteps = append(failedSteps, s)
		case pb.Status_STARTED:
			started++
			startedSteps = append(startedSteps, s)
		case pb.Status_CANCELED:
			canceled++
		}
	}

	totalFailures := failed + infraFailed

	if opts.AllSteps {
		fmt.Fprintf(w, "\nSteps (%d total: %d passed, %d failed, %d running):\n", len(steps), passed, totalFailures, started)
		for _, s := range steps {
			depth := strings.Count(s.Name, "|")
			indent := strings.Repeat("  ", depth)
			baseName := s.Name
			if idx := strings.LastIndex(s.Name, "|"); idx != -1 {
				baseName = s.Name[idx+1:]
			}

			icon := "[?]"
			switch s.Status {
			case pb.Status_SUCCESS:
				icon = "[✓]"
			case pb.Status_FAILURE, pb.Status_INFRA_FAILURE:
				icon = "[✕]"
			case pb.Status_STARTED:
				icon = "[●]"
			case pb.Status_CANCELED:
				icon = "[-]"
			}

			durStr := ""
			if s.StartTime != nil && s.EndTime != nil {
				durStr = ", " + formatDuration(s.EndTime.AsTime().Sub(s.StartTime.AsTime()))
			} else if s.StartTime != nil {
				durStr = ", running for " + formatDuration(time.Since(s.StartTime.AsTime()))
			}

			fmt.Fprintf(w, "%s%s %s (%s%s)\n", indent, icon, baseName, s.Status, durStr)
			if s.SummaryMarkdown != "" {
				for _, line := range strings.Split(s.SummaryMarkdown, "\n") {
					fmt.Fprintf(w, "%s    Summary: %s\n", indent, line)
				}
			}
			if len(s.Logs) > 0 {
				var logNames []string
				for _, l := range s.Logs {
					logNames = append(logNames, l.Name)
				}
				fmt.Fprintf(w, "%s    Logs: %s\n", indent, strings.Join(logNames, ", "))
			}
		}
		return totalFailures > 0
	}

	// Default view: highlight failures
	if totalFailures > 0 {
		fmt.Fprintf(w, "\nFailed Steps (%d failed, %d passed):\n", totalFailures, passed)
		for _, s := range failedSteps {
			durStr := ""
			if s.StartTime != nil && s.EndTime != nil {
				durStr = ", " + formatDuration(s.EndTime.AsTime().Sub(s.StartTime.AsTime()))
			}
			fmt.Fprintf(w, "  ✕ %s (%s%s)\n", s.Name, s.Status, durStr)
			if s.SummaryMarkdown != "" {
				for _, line := range strings.Split(s.SummaryMarkdown, "\n") {
					fmt.Fprintf(w, "    Summary: %s\n", line)
				}
			}
			if len(s.Logs) > 0 {
				var logNames []string
				for _, l := range s.Logs {
					if l.ViewUrl != "" {
						logNames = append(logNames, fmt.Sprintf("%s (%s)", l.Name, l.ViewUrl))
					} else {
						logNames = append(logNames, l.Name)
					}
				}
				fmt.Fprintf(w, "    Logs: %s\n", strings.Join(logNames, ", "))
			}
		}
		return true
	}

	if started > 0 {
		fmt.Fprintf(w, "\nSteps (%d running, %d passed):\n", started, passed)
		for _, s := range startedSteps {
			durStr := ""
			if s.StartTime != nil {
				durStr = ", running for " + formatDuration(time.Since(s.StartTime.AsTime()))
			}
			fmt.Fprintf(w, "  ● %s (STARTED%s)\n", s.Name, durStr)
		}
		return false
	}

	fmt.Fprintf(w, "Steps:         %d passed\n", passed)
	return false
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
