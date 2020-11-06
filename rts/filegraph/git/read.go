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
	"bufio"
	"encoding/binary"
	"io"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Read reads the graph from r.
// It is the opposite of (*Graph).Write().
func (g *Graph) Read(r *bufio.Reader) error {
	g.ensureInitialized()
	return (&reader{r: r}).readGraph(g)
}

type reader struct {
	r *bufio.Reader

	// textMode means tokens are encoded as utf-8 strings and appear on separate
	// lines.
	textMode bool

	// ordered are the nodes in the order as they appear in the reader.
	ordered []*node
	buf     []byte
}

func (r *reader) readGraph(g *Graph) error {
	// Verify header.
	switch header, err := r.readInt(); {
	case err != nil:
		return err
	case header != magicHeader:
		return errors.Reason("unexpected header").Err()
	}

	// Read version.
	switch ver, err := r.readInt(); {
	case err != nil:
		return err
	case ver != 0:
		return errors.Reason("unexpected version %d; expected 0", ver).Err()
	}

	// Read the commit.
	var err error
	if g.Commit, err = r.readString(); err != nil {
		return err
	}

	// Read the nodes.
	r.ordered = r.ordered[:0]
	if err := r.readNode(&g.root); err != nil {
		return errors.Annotate(err, "failed to read nodes").Err()
	}

	// Read the edges.
	for _, n := range r.ordered {
		if err := r.readEdges(n); err != nil {
			return errors.Annotate(err, "failed to read edges").Err()
		}
	}

	return nil
}

func (r *reader) readNode(n *node) error {
	r.ordered = append(r.ordered, n)

	// Read the number of commits.
	var err error
	if n.commits, err = r.readInt(); err != nil {
		return err
	}

	// Read the number of children.
	childCount, err := r.readInt()
	switch {
	case err != nil:
		return err
	case childCount == 0:
		return nil
	}

	// Read the children.
	n.children = make(map[string]*node, childCount)
	for i := 0; i < childCount; i++ {
		childBaseName, err := r.readString()
		if err != nil {
			return err
		}
		child := &node{}
		if n.name == "//" {
			child.name = n.name + childBaseName
		} else {
			child.name = n.name + "/" + childBaseName
		}
		n.children[childBaseName] = child
		if err := r.readNode(child); err != nil {
			return err
		}
	}

	return nil
}

func (r *reader) readEdges(n *node) error {
	// Read the number of edges.
	count, err := r.readInt()
	switch {
	case err != nil:
		return err
	case count == 0:
		return nil
	}

	// Read the edges.
	n.edges = make([]edge, count)
	for i := range n.edges {
		switch index, err := r.readInt(); {
		case err != nil:
			return err
		case index < 0 || index >= len(r.ordered):
			return errors.Reason("node index %d is out of bounds", index).Err()
		default:
			n.edges[i].to = r.ordered[index]
		}

		if n.edges[i].commonCommits, err = r.readInt(); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) readString() (string, error) {
	if r.textMode {
		return r.readLine()
	}

	length, err := r.readInt()
	if err != nil {
		return "", err
	}

	if cap(r.buf) < length {
		r.buf = make([]byte, length)
	}
	r.buf = r.buf[:length]
	if _, err := io.ReadFull(r.r, r.buf); err != nil {
		return "", err
	}
	return string(r.buf), nil
}

func (r *reader) readInt() (int, error) {
	if r.textMode {
		s, err := r.readLine()
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(s)
	}

	n, err := binary.ReadVarint(r.r)
	return int(n), err
}

func (r *reader) readLine() (string, error) {
	if !r.textMode {
		panic("not text mode")
	}
	s, err := r.r.ReadString('\n')
	return strings.TrimSuffix(s, "\n"), err
}
