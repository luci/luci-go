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
	"bytes"
	"fmt"
	"strings"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
)

// resolveTargetStep finds the requested step within a Build.
// If stepName is not an exact match, it falls back to matching a unique leaf step name
// (after the last '|') or the LogDog-sanitized step path from the step's log URLs.
func resolveTargetStep(b *pb.Build, stepName string) (*pb.Step, error) {
	var matchedStep *pb.Step
	for _, s := range b.Steps {
		if s.Name == stepName {
			matchedStep = s
			break
		}
	}

	if matchedStep == nil {
		suffix := "|" + stepName
		var suffixMatches []*pb.Step
		for _, s := range b.Steps {
			if strings.HasSuffix(s.Name, suffix) {
				suffixMatches = append(suffixMatches, s)
			}
		}
		if len(suffixMatches) == 1 {
			matchedStep = suffixMatches[0]
		} else if len(suffixMatches) > 1 {
			var names []string
			for _, s := range suffixMatches {
				names = append(names, "  "+s.Name)
			}
			return nil, errors.Fmt("step name %q is ambiguous in build %d. Matching steps:\n%s", stepName, b.Id, strings.Join(names, "\n"))
		}
	}

	if matchedStep == nil {
		wantedStream := strings.TrimPrefix(strings.ReplaceAll(stepName, "|", "/"), "u/")
		var streamMatches []*pb.Step
		for _, s := range b.Steps {
			if stepMatchesLogDogPath(s, wantedStream) {
				streamMatches = append(streamMatches, s)
			}
		}
		if len(streamMatches) == 1 {
			matchedStep = streamMatches[0]
		} else if len(streamMatches) > 1 {
			var names []string
			for _, s := range streamMatches {
				names = append(names, "  "+s.Name)
			}
			return nil, errors.Fmt("step name %q is ambiguous in build %d. Matching steps:\n%s", stepName, b.Id, strings.Join(names, "\n"))
		}
	}

	if matchedStep == nil {
		if len(b.Steps) == 0 {
			return nil, errors.Fmt("step %q not found: build %d has no steps", stepName, b.Id)
		}
		var available []string
		for _, s := range b.Steps {
			if s.Status == pb.Status_FAILURE || s.Status == pb.Status_INFRA_FAILURE {
				available = append(available, fmt.Sprintf("  %s (%s)", s.Name, s.Status))
			} else {
				available = append(available, "  "+s.Name)
			}
		}
		return nil, errors.Fmt("step %q not found in build %d. Available steps:\n%s", stepName, b.Id, strings.Join(available, "\n"))
	}

	return matchedStep, nil
}

// resolveTargetLog finds the requested step and log within a Build.
func resolveTargetLog(b *pb.Build, stepName, logName string) (*pb.Log, error) {
	if stepName == "" {
		buildLogs := b.GetOutput().GetLogs()
		if len(buildLogs) > 0 {
			return selectLog(buildLogs, logName, fmt.Sprintf("build %d", b.Id))
		}
		return nil, errors.Fmt("flag -step is required (run 'luci build get -buildid %d' to see steps and logs)", b.Id)
	}

	matchedStep, err := resolveTargetStep(b, stepName)
	if err != nil {
		return nil, err
	}

	if len(matchedStep.Logs) == 0 {
		return nil, errors.Fmt("step %q in build %d has no logs", matchedStep.Name, b.Id)
	}

	return selectLog(matchedStep.Logs, logName, fmt.Sprintf("step %q of build %d", matchedStep.Name, b.Id))
}

func stepMatchesLogDogPath(s *pb.Step, wantedStream string) bool {
	if wantedStream == "" {
		return false
	}
	for _, l := range s.Logs {
		for _, u := range []string{l.Url, l.ViewUrl} {
			stepPath, _ := logDogStreamParts(u)
			if stepPath != "" && (stepPath == wantedStream || strings.HasSuffix(stepPath, "/"+wantedStream)) {
				return true
			}
		}
	}
	return false
}

func logDogStreamParts(rawURL string) (stepStreamPath, logStreamName string) {
	idx := strings.Index(rawURL, "/+/")
	if idx == -1 {
		return "", ""
	}
	streamPath := strings.Trim(rawURL[idx+3:], "/")
	if streamPath == "" {
		return "", ""
	}
	if strings.HasPrefix(streamPath, "u/") {
		uRest := strings.TrimPrefix(streamPath, "u/")
		segs := strings.Split(uRest, "/")
		if len(segs) >= 2 {
			return strings.Join(segs[:len(segs)-1], "/"), segs[len(segs)-1]
		}
		if len(segs) == 1 {
			return "", segs[0]
		}
	}
	segs := strings.Split(streamPath, "/")
	return "", segs[len(segs)-1]
}

func selectLog(logs []*pb.Log, logName, scopeDesc string) (*pb.Log, error) {
	if logName != "" {
		for _, l := range logs {
			if l.Name == logName {
				return l, nil
			}
		}
		for _, l := range logs {
			for _, u := range []string{l.Url, l.ViewUrl} {
				if _, streamLogName := logDogStreamParts(u); streamLogName != "" && streamLogName == logName {
					return l, nil
				}
			}
		}
		var names []string
		for _, l := range logs {
			names = append(names, "  "+l.Name)
		}
		return nil, errors.Fmt("log %q not found in %s. Available logs:\n%s", logName, scopeDesc, strings.Join(names, "\n"))
	}

	// Default log selection priority when -log is omitted:
	// 1. "stdout"
	// 2. "failure_summary"
	// 3. Single available log
	for _, preferred := range []string{"stdout", "failure_summary"} {
		for _, l := range logs {
			if l.Name == preferred {
				return l, nil
			}
		}
	}
	if len(logs) == 1 {
		return logs[0], nil
	}

	var names []string
	for _, l := range logs {
		names = append(names, "  "+l.Name)
	}
	return nil, errors.Fmt("default log \"stdout\" not found in %s; specify -log with one of the available logs:\n%s", scopeDesc, strings.Join(names, "\n"))
}

func renderLogEntries(entries []*logpb.LogEntry, buf *bytes.Buffer) {
	for _, le := range entries {
		if le == nil {
			continue
		}
		if txt := le.GetText(); txt != nil {
			for _, line := range txt.GetLines() {
				buf.Write(line.GetValue())
				buf.WriteString(line.GetDelimiter())
			}
		} else if bin := le.GetBinary(); bin != nil {
			buf.Write(bin.GetData())
		}
	}
}

func validateStreamDescriptor(desc *logpb.LogStreamDescriptor) error {
	if desc != nil && desc.StreamType == logpb.StreamType_DATAGRAM {
		return errors.New("log is a datagram stream; only text and binary streams are supported")
	}
	return nil
}
