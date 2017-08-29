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

package resp

import (
	"bytes"
	"fmt"
	"html/template"

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
func (br BuilderRef) RenderHTML(buffer *bytes.Buffer, depth int, maxDepth int) {
	must(buffer.WriteString(`<div class="console-builder-column" style="flex-grow: 1">`))
	// Add spaces if we haven't hit maximum depth to keep the grid consistent.
	for i := 0; i < (maxDepth - depth); i++ {
		must(buffer.WriteString(`<div class="console-space"></div>`))
	}
	// Render builder link
	must(buffer.WriteString(`<div>`))
	must(fmt.Fprintf(
		buffer, `<a class="console-builder-item" href="/%s" title="%s">%s</a>`,
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
		panic("Rendering zero builds should have been caught earlier.")
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
			link = build.SelfLink
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
			link = build.SelfLink
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
func (c Category) RenderHTML(buffer *bytes.Buffer, depth int, maxDepth int) {
	must(buffer.WriteString(`<div class="console-column">`))
	must(fmt.Fprintf(buffer, `<div class="console-top-item">%s</div>
				  <div class="console-top-row">`,
		template.HTMLEscapeString(c.Name),
	))
	for _, child := range c.Children {
		child.RenderHTML(buffer, depth+1, maxDepth)
	}
	must(buffer.WriteString(`</div></div>`))
}
