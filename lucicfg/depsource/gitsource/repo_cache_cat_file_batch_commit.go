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

package gitsource

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/common/exec"
)

type Commit struct {
	Tree          string
	Parents       []string
	Author        string
	AuthorTime    time.Time
	Committer     string
	CommitterTime time.Time
	MessageLines  []string
	Trailers      map[string][]string
}

var userTimeRe = regexp.MustCompile(`^(.*) (\d+) ([+-]\d+)$`)

func parseUserTime(value string) (user string, ts time.Time) {
	groups := userTimeRe.FindStringSubmatch(value)
	if len(groups) == 0 {
		user = value
		return
	}

	unixTs, err := strconv.ParseInt(groups[2], 10, 64)
	if err != nil {
		user = value
		return
	}

	tLoc, err := time.Parse("-0700", groups[3])
	if err != nil {
		user = value
		return
	}

	user = groups[1]
	ts = time.Unix(unixTs, 0).In(tLoc.Location())
	return
}

type parserFn func(ctx context.Context, c *Commit, lines []string) ([]string, error)

var parsers = []parserFn{
	func(ctx context.Context, c *Commit, lines []string) ([]string, error) {
		treeVal, ok := strings.CutPrefix(lines[0], "tree ")
		if !ok {
			return nil, fmt.Errorf("expected tree")
		}
		c.Tree = treeVal
		return lines[1:], nil
	},
	func(ctx context.Context, c *Commit, lines []string) ([]string, error) {
		for len(lines) > 0 {
			parentVal, ok := strings.CutPrefix(lines[0], "parent ")
			if !ok {
				break
			}
			c.Parents = append(c.Parents, parentVal)
			lines = lines[1:]
		}
		return lines, nil
	},
	func(ctx context.Context, c *Commit, lines []string) ([]string, error) {
		authorVal, ok := strings.CutPrefix(lines[0], "author ")
		if !ok {
			return nil, fmt.Errorf("expected author")
		}
		c.Author, c.AuthorTime = parseUserTime(authorVal)
		return lines[1:], nil
	},
	func(ctx context.Context, c *Commit, lines []string) ([]string, error) {
		committerVal, ok := strings.CutPrefix(lines[0], "committer ")
		if !ok {
			return nil, fmt.Errorf("expected committer")
		}
		c.Committer, c.CommitterTime = parseUserTime(committerVal)
		return lines[1:], nil
	},
	func(ctx context.Context, c *Commit, lines []string) ([]string, error) {
		if lines[0] != "" {
			return nil, fmt.Errorf("expected empty line")
		}
		return lines[1:], nil
	},
	func(ctx context.Context, c *Commit, lines []string) ([]string, error) {
		lastBlockIdx := -1
		for i, line := range slices.Backward(lines) {
			if strings.TrimSpace(line) == "" {
				lastBlockIdx = i
				break
			}
		}
		if lastBlockIdx == -1 {
			return lines, nil
		}

		cmd := exec.CommandContext(ctx, "git", "interpret-trailers", "--parse")
		trailersBuf := bytes.Buffer{}
		for _, line := range lines[lastBlockIdx:] { // include empty line
			fmt.Fprintln(&trailersBuf, line)
		}
		cmd.Stdin = &trailersBuf
		parsed, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		if len(parsed) == 0 {
			return lines, nil
		}
		trailers := map[string][]string{}
		for _, line := range strings.Split(string(parsed), "\n") {
			if len(line) == 0 {
				continue
			}
			toks := strings.SplitN(line, ": ", 2)
			if len(toks) != 2 {
				return lines, nil
			}
			key := toks[0]
			trailers[key] = append(trailers[key], toks[1])
		}
		c.Trailers = trailers
		return lines[:lastBlockIdx], nil
	},
}

func (c *Commit) parse(ctx context.Context, lines []string) (err error) {
	if lastIdx := len(lines) - 1; lines[lastIdx] == "" {
		lines = lines[:lastIdx]
	}
	for _, fn := range parsers {
		lines, err = fn(ctx, c, lines)
		if err != nil {
			return err
		}
	}
	c.MessageLines = lines
	return nil
}

func (b *batchProc) catFileCommit(ctx context.Context, commit string) (ret Commit, err error) {
	kind, data, err := b.catFile(ctx, commit)
	if err != nil {
		return
	}
	if kind != CommitKind {
		err = fmt.Errorf("catFileCommit(%q): expected %s got %s", commit, CommitKind, kind)
		return
	}
	err = ret.parse(ctx, strings.Split(string(data), "\n"))
	return
}
