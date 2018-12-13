package ui

import (
	"html/template"
	"strings"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// BuildPage represents a build page on Milo.
// The core of the build page is the underlying build proto, but can contain
// extra information depending on the context, for example a blamelist,
// and the user's display preferences.
type BuildPage struct {
	// Build is the underlying build proto for the build page.
	buildbucketpb.Build

	// Blame is a list of people and commits that is likely to be in relation to
	// the thing displayed on this page.
	Blame []*Commit

	// Mode to render the steps.
	StepDisplayPref StepDisplayPref
}

// StepDisplayPref is the display preference for the steps.
type StepDisplayPref string

const (
	// StepDisplayDefault means that all steps are visible, green steps are
	// collapsed.
	StepDisplayDefault StepDisplayPref = "default"
	// StepDisplayExpanded means that all steps are visible, nested steps are
	// expanded.
	StepDisplayExpanded StepDisplayPref = "expanded"
	// StepDisplayNonGreen means that only non-green steps are visible, nested
	// steps are expanded.
	StepDisplayNonGreen StepDisplayPref = "non-green"
)

// Commit represents a single commit to a repository, rendered as part of a blamelist.
type Commit struct {
	// Who made the commit?
	AuthorName string
	// Email of the committer.
	AuthorEmail string
	// Time of the commit.
	CommitTime time.Time
	// Full URL of the main source repository.
	Repo string
	// Branch of the repo.
	Branch string
	// Requested revision of the commit or base commit.
	RequestRevision *Link
	// Revision of the commit or base commit.
	Revision *Link
	// The commit message.
	Description string
	// Rietveld or Gerrit URL if the commit is a patch.
	Changelist *Link
	// Browsable URL of the commit.
	CommitURL string
	// List of changed filenames.
	File []string
}

// RevisionHTML returns a single rendered link for the revision, prioritizing
// Revision over RequestRevision.
func (c *Commit) RevisionHTML() template.HTML {
	switch {
	case c == nil:
		return ""
	case c.Revision != nil:
		return c.Revision.HTML()
	case c.RequestRevision != nil:
		return c.RequestRevision.HTML()
	default:
		return ""
	}
}

// Title is the first line of the commit message (Description).
func (c *Commit) Title() string {
	switch lines := strings.SplitN(c.Description, "\n", 2); len(lines) {
	case 0:
		return ""
	case 1:
		return c.Description
	default:
		return lines[0]
	}
}

// DescLines returns the description as a slice, one line per item.
func (c *Commit) DescLines() []string {
	return strings.Split(c.Description, "\n")
}
