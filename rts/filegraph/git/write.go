// Copyright 2020 The LUCI Authors.
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

package git

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// magicHeader is the first token when writing/reading a graph.
const magicHeader = 54

// Write writes the graph to w.
// It is the opposite of (*Graph).Read().
//
// Spec:
//  graph = header version git-commit-hash root total-number-of-edges root-edges
//  header = 54
//  version = 0
//
//  root = node
//  node = number-of-commits number-of-children children-sorted-by-base-name
//  children-sorted-by-base-name = child*
//  child = base-name node
//
//  root-edges = node-edges
//  node-edges = number-of-edges edge*
//  edge =
//    index-of-the-adjacent-node-as-found-in-the-file
//    number-of-common-commits
//    edges-of-children-sorted-by-base-name
//  edges-of-children-sorted-by-base-name = edge*
//
//  where
//   all integer types are encoded as uvarint
//   all strings are encoded as length-prefixed utf8
//   `*` means "0 or more"
func (g *Graph) Write(w io.Writer) error {
	g.ensureInitialized()
	return (&writer{w: w}).writeGraph(g)
}

type writer struct {
	w io.Writer
	// textMode means tokens are encoded as utf-8 strings and appear on separate
	// lines.
	textMode bool

	varintBuf  [binary.MaxVarintLen64]byte
	indices    map[*node]int
	totalEdges int
}

func (w *writer) writeGraph(g *Graph) error {
	// Write the header.
	if err := w.writeInt(magicHeader); err != nil {
		return err
	}

	// Write version.
	if err := w.writeInt(0); err != nil {
		return err
	}

	// Write commit.
	if err := w.writeString(g.Commit); err != nil {
		return err
	}

	// Write nodes.
	w.indices = map[*node]int{}
	if err := w.writeNode(&g.root); err != nil {
		return err
	}

	// Write the total number of edges.
	if err := w.writeInt(w.totalEdges); err != nil {
		return err
	}

	// Write edges.
	return w.writeEdges(&g.root)
}

func (w *writer) writeNode(n *node) error {
	w.indices[n] = len(w.indices)
	w.totalEdges += len(n.edges)

	// Write the number of commits.
	if err := w.writeInt(n.commits); err != nil {
		return err
	}

	// Write the number of direct children.
	if err := w.writeInt(len(n.children)); err != nil {
		return err
	}

	// Write the descendants.
	for _, key := range n.sortedChildKeys() {
		if err := w.writeString(key); err != nil {
			return err
		}
		if err := w.writeNode(n.children[key]); err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) writeEdges(n *node) error {
	// TODO(nodir): consider changing writing edges only for
	// nodes that have them. Note that only files have edges,
	// unlike directories.
	// Then we don't have to sort keys.

	// Write the edges.
	if err := w.writeInt(len(n.edges)); err != nil {
		return err
	}
	for _, e := range n.edges {
		if err := w.writeInt(w.indices[e.to]); err != nil {
			return err
		}
		if err := w.writeInt(e.commonCommits); err != nil {
			return err
		}
	}

	// Write the edges of descendants.
	for _, key := range n.sortedChildKeys() {
		if err := w.writeEdges(n.children[key]); err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) writeString(s string) error {
	if w.textMode {
		if strings.Contains(s, "\n") {
			return errors.Reason("linebreak is not supported in text mode").Err()
		}
		_, err := fmt.Fprintln(w.w, s)
		return err
	}

	if err := w.writeInt(len(s)); err != nil {
		return err
	}
	_, err := io.WriteString(w.w, s)
	return err
}

func (w *writer) writeInt(n int) error {
	if w.textMode {
		_, err := fmt.Fprintln(w.w, n)
		return err
	}

	length := binary.PutVarint(w.varintBuf[:], int64(n))
	_, err := w.w.Write(w.varintBuf[:length])
	return err
}
