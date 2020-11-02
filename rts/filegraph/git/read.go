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
	"io"
)

func (g *Graph) Read(r interface {
	io.Reader
	io.ByteReader
}) error {
	return (&reader{r: r}).readGraph(g)
}

type reader struct {
	r interface {
		io.ByteReader
		io.Reader
	}
	nodes []*node
	buf   []byte
}

func (r *reader) readGraph(g *Graph) error {
	var err error
	if g.commit, err = r.readString(); err != nil {
		return err
	}

	r.nodes = r.nodes[:0]
	if err := r.readNode(&g.root); err != nil {
		return err
	}

	for _, n := range r.nodes {
		if err := r.readEdges(n); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) readNode(n *node) error {
	r.nodes = append(r.nodes, n)

	var err error
	if n.commits, err = r.readInt(); err != nil {
		return err
	}

	childCount, err := r.readInt()
	switch {
	case err != nil:
		return err
	case childCount == 0:
		return nil
	}

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
	count, err := r.readInt()
	if err != nil {
		return err
	}

	n.edges = make([]edge, count)
	for i := range n.edges {
		index, err := r.readInt()
		if err != nil {
			return err
		}
		n.edges[i].other = r.nodes[index]

		if n.edges[i].commonCommits, err = r.readInt(); err != nil {
			return err
		}
	}
	return nil
}

func (r *reader) readString() (string, error) {
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
	n, err := binary.ReadVarint(r.r)
	return int(n), err
}
