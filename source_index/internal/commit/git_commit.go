// Copyright 2024 The LUCI Authors.
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

package commit

import (
	"regexp"
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/git/footer"
	"go.chromium.org/luci/common/proto/git"
)

var (
	// ErrInvalidPositionFooter is returned when there is matching position footer
	// key, but its value doesn't match expected format.
	ErrInvalidPositionFooter = errors.New("invalid position footer format")
)

type GitCommit struct {
	// key is the primary key of the commit in the Commits table.
	key Key
	// message is the commit message.
	message string
}

// NewGitCommit creates a new GitCommit.
func NewGitCommit(host, repository string, commit *git.Commit) (GitCommit, error) {
	key, err := NewKey(host, repository, commit.Id)
	if err != nil {
		return GitCommit{}, errors.Fmt("invalid commit key: %w", err)
	}

	return GitCommit{
		key:     key,
		message: commit.Message,
	}, nil
}

// Key returns the key of the commit.
func (c GitCommit) Key() Key {
	return c.key
}

// Position extracts the commit position from the commit message.
func (c GitCommit) Position() (*Position, error) {
	pos, err := extractPosition(c.message)
	return pos, errors.WrapIf(err, "extract commit position from commit message")
}

var (
	crCommitPositionFooterName = "Cr-Commit-Position"
	crCommitPositionFormat     = regexp.MustCompile(`(?P<name>.*)@{#(?P<number>\d+)}`)

	svnCommitPositionFooterName = "Git-Svn-Id"
	svnCommitPositionFormat     = regexp.MustCompile(`(?P<name>.*)@(?P<number>\d+)`)
)

// extractPosition extracts the commit position from the commit message footers.
func extractPosition(commitMessage string) (*Position, error) {
	footerMap := footer.ParseMessage(commitMessage)

	// Prefer Cr-Commit-Position based footer.
	if footers, ok := footerMap[crCommitPositionFooterName]; ok {
		// Note that the footers list has reversed order.
		// The first item is the last footer in the commit message.
		pos, err := extractPositionWithRe(footers[0], crCommitPositionFormat)
		if err != nil {
			return nil, errors.Fmt("invalid %s footer: %w", crCommitPositionFooterName, err)
		}
		return pos, nil
	}

	// Fallback to Git-Svn-Id based footer.
	if footers, ok := footerMap[svnCommitPositionFooterName]; ok {
		// Note that the footers list has reversed order.
		// The first item is the last footer in the commit message.
		pos, err := extractPositionWithRe(footers[0], svnCommitPositionFormat)
		if err != nil {
			return nil, errors.Fmt("invalid %s footer: %w", svnCommitPositionFooterName, err)
		}
		return pos, nil
	}

	return nil, nil
}

// extractPositionWithRe extracts the commit position from the commit message
// footers with the specified regex.
func extractPositionWithRe(footer string, re *regexp.Regexp) (*Position, error) {
	match := re.FindStringSubmatch(footer)
	if len(match) == 0 {
		return nil, errors.Fmt("does not match pattern %q: %w", re, ErrInvalidPositionFooter)
	}

	cp := Position{}
	for i, name := range re.SubexpNames() {
		switch name {
		case "name":
			cp.Ref = match[i]
		case "number":
			positionNumber, err := strconv.ParseInt(match[i], 10, 64)
			if err != nil {
				return nil, err
			}
			cp.Number = positionNumber
		}
	}
	return &cp, nil
}
