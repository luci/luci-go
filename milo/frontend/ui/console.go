// Copyright 2016 The LUCI Authors.
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
	"bytes"
	"fmt"
	"html/template"
	"net/url"

	"go.chromium.org/luci/milo/common/model"
)

// This file contains the structures for defining a Console view.
// Console: The main entry point and the overall struct for a console page.
// Category: A column in the console representing a builder category that may contain
//   subcategories (other columns) as well as builders  References a commit with a list
//   of build summaries.
// BuilderRef: Used both as an input to request a builder and headers for the console.

// This file also contains an interface through which to render the console "tree."
// ConsoleElement: Represents a renderable unit of the console. In this case a unit refers
//   to either a builder (via a BuilderRef) or a category (via a Category).

// Console represents a console view.  Commit contains a list of commits to be displayed
// in this console. Table contains a tree of categories whose leaves are builders.
// The builders maintain information regarding the builds for the corresponding commits
// in Commit. MaxDepth is a useful piece of metadata for adding in empty rows in the
// console's header.
type Console struct {
	Name string

	// Project is the LUCI project for which this console is defined.
	Project string

	// Header is an optional header for the console which contains links, oncall info,
	// and summaries of other, presumably related consoles.
	//
	// This field may be nil, which simply indicates to the renderer to not render a
	// header.
	Header *ConsoleHeader

	// Commit is a list of commits representing the list of commits to the left of the
	// console.
	Commit []Commit

	// Table is a tree of builder categories used to generate the console's main table.
	//
	// Leaf nodes must always be of concrete type BuilderRef, interior nodes must
	// always be of type Category. The root node is a dummy node without a name
	// to simplify the implementation.
	Table Category

	// MaxDepth represents the maximum tree depth of Table.
	MaxDepth int

	// FaviconURL is the URL to the favicon for this console.
	FaviconURL string
}

// ConsoleSummary represents the summary of a console, including its name and the latest
// status of each of its builders.
type ConsoleSummary struct {
	// Name is a Link that contains the name of the console as well as a relative URL
	// to the console's page.
	Name *Link

	// Builders contains a list of builders for a given console and some data about
	// the latest state for each builder.
	Builders []*model.BuilderSummary
}

// TreeStatusState indicates the status of a tree.
type TreeStatusState string

const (
	// TreeMaintenance means the tree is under maintenance and is not open.
	// This has a color of purple.
	TreeMaintenance TreeStatusState = "maintenance"
	// TreeThrottled means the tree is backed up, and commits are throttled
	// to allow the tree to catch up.  This has a color of yellow.
	TreeThrottled = "throttled"
	// TreeClosed means the tree is broken, and commits other than reverts
	// or fixes are not accepted at the time.  This has a color of red.
	TreeClosed = "closed"
	// TreeOpen means the tree is not broken, and commits can happen freely.
	// This has a color of green.
	TreeOpen = "open"
)

// TreeStatus represents the very top bar of the console, above the header.
type TreeStatus struct {
	// Username is the name of the user who changed the status last.
	Username        string `json:"username"`
	CanCommitFreely bool   `json:"can_commit_freely"`
	// GeneralState is the general state of the tree, which also indicates
	// the color of the tree.
	GeneralState TreeStatusState `json:"general_state"`
	Key          int64           `json:"key"`
	// Date is when the tree was last updated.  The format is YYYY-mm-DD HH:MM:SS.ssssss
	// and implicitly UTC.  eg. "2017-11-10 14:29:21.804080"
	Date string `json:"date"`
	// Message is a human readable description of the tree.
	Message string `json:"message"`
	// URL is a link to the root status page.  This is generated on the Milo side,
	// not provided by the status app.
	URL *url.URL
}

// Oncall represents an oncall role with the current individuals in that role, represented
// by their email addresses.
type Oncall struct {
	// Name is the name of the oncall role.  This is set by Milo.
	Name string `json:"-"`

	// Primary is the username of the primary oncall.  This is used in lieu of emails.
	// This is filled in from the remote JSON.
	Primary string

	// Secondaries are the usernames of the secondary oncalls.  This is used in lieu of emails.
	// This is filled in from the remote JSON.
	Secondaries []string

	// Emails is a list of email addresses for the individuals who are currently in
	// that role.  This is loaded from the sheriffing json.
	Emails []string
}

// LinkGroup represents a set of links grouped together by some category.
type LinkGroup struct {
	// Name is the name of the category this group of links belongs to.
	Name *Link

	// Links is a list of links in this link group.
	Links []*Link
}

// ConsoleGroup represents a group of console summaries which may optionally be titled.
// Logically, it represents a group of consoles with some shared quality (e.g. tree closers).
type ConsoleGroup struct {
	// Title is the title for this group of consoles and may link to anywhere.
	Title *Link

	// Consoles is the list of console summaries contained without this group.
	Consoles []ConsoleSummary
}

// ConsoleHeader represents the header of a console view, containing a set of links,
// oncall details, as well as a set of console summaries for other, relevant consoles.
type ConsoleHeader struct {
	// Oncalls is a list of oncall roles and the current people who fill that role
	// that will be displayed in the header..
	Oncalls []Oncall

	// Links is a list of link groups to be displayed in the header.
	Links []LinkGroup

	// ConsoleGroups is a list of groups of console summaries to be displayed in
	// the header.
	//
	// A console group without a title will have all of its console summaries
	// appear "ungrouped" when rendered.
	ConsoleGroups []ConsoleGroup

	// TreeStatus indicates the status of the tree if it is not nil.
	TreeStatus *TreeStatus
}

// ConsoleElement represents a single renderable console element.
type ConsoleElement interface {
	// Writes HTML into the given byte buffer.
	//
	// The two integer parameters represent useful pieces of metadata in
	// rendering: current depth, and maximum depth.
	RenderHTML(*bytes.Buffer, int, int)
}

// Category represents an interior node in a category tree for builders.
//
// Implements ConsoleElement.
type Category struct {
	Name string

	// The node's children, which can be any console element.
	Children []ConsoleElement

	// The node's children in a map to simplify insertion.
	childrenMap map[string]ConsoleElement
}

// NewCategory allocates a new Category struct with no children.
func NewCategory(name string) *Category {
	return &Category{
		Name:        name,
		childrenMap: make(map[string]ConsoleElement),
		Children:    make([]ConsoleElement, 0),
	}
}

// AddBuilder inserts the builder into this Category tree.
//
// AddBuilder will create new subcategories as a chain of Category structs
// as needed until there are no categories remaining. The builder is then
// made a child of the deepest such Category.
func (c *Category) AddBuilder(categories []string, builder *BuilderRef) {
	current := c
	for _, category := range categories {
		if child, ok := current.childrenMap[category]; ok {
			original := child.(*Category)
			current = original
		} else {
			newChild := NewCategory(category)
			current.childrenMap[category] = ConsoleElement(newChild)
			current.Children = append(current.Children, ConsoleElement(newChild))
			current = newChild
		}
	}
	current.childrenMap[builder.Name] = ConsoleElement(builder)
	current.Children = append(current.Children, ConsoleElement(builder))
}

// BuilderRef is an unambiguous reference to a builder.
//
// It represents a single column of builds in the console view.
//
// Implements ConsoleElement.
type BuilderRef struct {
	// Name is the canonical reference to a specific builder.
	Name string
	// ShortName is a string of length 1-3 used to label the builder.
	ShortName string
	// The most recent build summaries for this builder.
	Build []*model.BuildSummary
	// The most recent builder summary for this builder.
	Builder *model.BuilderSummary
}

// Convenience function for writing to bytes.Buffer: in our case, the
// writes into the buffer should _never_ fail. It is a catastrophic error
// if it does.
func must(_ int, err error) {
	if err != nil {
		panic(err)
	}
}

// State machine states for rendering builds.
const (
	empty = iota
	top
	middle
	bottom
	cell
)

// RenderHTML renders a BuilderRef as HTML with its builds in a column.
// If maxDepth is negative, render the HTML as flat rather than nested.
func (br BuilderRef) RenderHTML(buffer *bytes.Buffer, depth int, maxDepth int) {
	// If render the HTML as flat rather than nested, we don't need to recurse at all and should just
	// return after rendering the BuilderSummary.
	if maxDepth < 0 {
		if br.Builder != nil && br.Builder.LastFinishedBuildID != "" {
			must(fmt.Fprintf(buffer, `<a class="console-builder-status" href="%s" title="%s">`,
				template.HTMLEscapeString(br.Builder.LastFinishedBuildIDLink()),
				template.HTMLEscapeString(br.Builder.LastFinishedBuildID),
			))
			must(fmt.Fprintf(buffer, `<div class="console-list-builder status-%s"></div>`,
				template.HTMLEscapeString(br.Builder.LastFinishedStatus.String()),
			))
		} else {
			must(fmt.Fprintf(buffer, `<a class="console-builder-status" href="/%s" title="%s">`,
				template.HTMLEscapeString(br.Name),
				template.HTMLEscapeString(br.Name),
			))
			must(buffer.WriteString(`<div class="console-list-builder"></div>`))
		}
		must(buffer.WriteString(`</a>`))
		return
	}

	must(buffer.WriteString(`<div class="console-builder-column" style="flex-grow: 1">`))
	// Add spaces if we haven't hit maximum depth to keep the grid consistent.
	for i := 0; i < (maxDepth - depth); i++ {
		must(buffer.WriteString(`<div class="console-space"></div>`))
	}
	must(buffer.WriteString(`<div>`))
	var extraStatus string
	if br.Builder != nil {
		extraStatus += fmt.Sprintf("console-%s", br.Builder.LastFinishedStatus)
	}
	must(fmt.Fprintf(
		buffer, `<span class="%s"><a class="console-builder-item" href="/%s" title="%s">%s</a></span>`,
		template.HTMLEscapeString(extraStatus),
		template.HTMLEscapeString(br.Name),
		template.HTMLEscapeString(br.Name),
		template.HTMLEscapeString(br.ShortName)))
	must(buffer.WriteString(`</div>`))

	must(buffer.WriteString(`<div class="console-build-column">`))

	status := "None"
	link := "#"
	class := ""

	// Below is a state machine for rendering a single builder's column.
	// In essence, the state machine takes 3 inputs: the current state, and
	// the if the next 2 builds exist. It uses this information to choose the
	// next state.
	//
	// Each iteration, the state machine writes out the state's corresponding element,
	// either a lone cell, the top of a long cell, the bottom of a long cell, the
	// middle of a long cell, or an empty space.
	//
	// The ultimate goal of this state machine is to visually extend a single
	// build down to the next known build for this builder by commit.

	// Initialize state machine state.
	//
	// Could equivalently be implemented using a "start" state, but
	// that requires a no-render special case which would make the
	// state machine less clean.
	var state int
	switch {
	case len(br.Build) == 1:
		switch {
		case br.Build[0] != nil:
			state = cell
		case br.Build[0] == nil:
			state = empty
		}
	case len(br.Build) > 1:
		switch {
		case br.Build[0] != nil && br.Build[1] != nil:
			state = cell
		case br.Build[0] != nil && br.Build[1] == nil:
			state = top
		case br.Build[0] == nil:
			state = empty
		}
	default:
		// This is probably a console preview.
	}
	// Execute state machine for determining cell type.
	for i, build := range br.Build {
		nextBuild := false
		if i < len(br.Build)-1 {
			nextBuild = br.Build[i+1] != nil
		}
		nextNextBuild := false
		if i < len(br.Build)-2 {
			nextNextBuild = br.Build[i+2] != nil
		}

		var nextState int
		switch state {
		case empty:
			class = "empty-cell"
			switch {
			case nextBuild && nextNextBuild:
				nextState = cell
			case nextBuild && !nextNextBuild:
				nextState = top
			case !nextBuild:
				nextState = empty
			}
		case top:
			class = "cell-top"
			status = build.Summary.Status.String()
			link = build.SelfLink()
			switch {
			case nextNextBuild:
				nextState = bottom
			case !nextNextBuild:
				nextState = middle
			}
		case middle:
			class = "cell-middle"
			switch {
			case nextNextBuild:
				nextState = bottom
			case !nextNextBuild:
				nextState = middle
			}
		case bottom:
			class = "cell-bottom"
			switch {
			case nextNextBuild:
				nextState = cell
			case !nextNextBuild:
				nextState = top
			}
		case cell:
			class = "cell"
			status = build.Summary.Status.String()
			link = build.SelfLink()
			switch {
			case nextNextBuild:
				nextState = cell
			case !nextNextBuild:
				nextState = top
			}
		default:
			panic("Unrecognized state")
		}
		// Write current state's information.
		must(fmt.Fprintf(buffer,
			`<div><a class="console-%s status-%s" href="%s"></a></div>`,
			class, status, link))

		// Update state.
		state = nextState
	}
	must(buffer.WriteString(`</div></div>`))
}

// RenderHTML renders the Category struct and its children as HTML into a buffer.
// If maxDepth is negative, skip the labels to render the HTML as flat rather than nested.
func (c Category) RenderHTML(buffer *bytes.Buffer, depth int, maxDepth int) {
	if maxDepth > 0 {
		must(buffer.WriteString(`<div class="console-column">`))
		must(fmt.Fprintf(buffer, `<div class="console-top-item">%s</div>
						<div class="console-top-row">`,
			template.HTMLEscapeString(c.Name),
		))
	} else if depth <= 1 {
		must(buffer.WriteString(`<div class="console-builder-summary">`))
	}

	for _, child := range c.Children {
		child.RenderHTML(buffer, depth+1, maxDepth)
	}

	if maxDepth > 0 {
		must(buffer.WriteString(`</div></div>`))
	} else {
		if depth <= 1 {
			must(buffer.WriteString(`</div>`))
		}
	}
}
