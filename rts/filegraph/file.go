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

package filegraph

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/rts/filegraph/internal/git"
	filegraphpb "go.chromium.org/luci/rts/filegraph/proto"
)

type graphFile struct {
	path   string
	commit string
	Graph
}

func (f *graphFile) Load() error {
	file, err := os.Open(f.path)
	if err != nil {
		return err
	}
	defer file.Close()

	bufR := bufio.NewReader(file)
	var buf []byte

	readMsg := func(m proto.Message) error {
		sz, err := binary.ReadUvarint(bufR)
		if err != nil {
			return err
		}

		if cap(buf) < int(sz) {
			buf = make([]byte, sz)
		}
		buf = buf[:sz]
		if _, err := io.ReadFull(bufR, buf); err != nil {
			return err
		}
		return proto.Unmarshal(buf, m)
	}

	hd := &filegraphpb.Header{}
	if err := readMsg(hd); err != nil {
		return err
	}
	f.commit = hd.Commit

	var ordered []*Node
	nodeMsg := &filegraphpb.Node{}
	var readNode func(prefix Path, n *Node) error
	readNode = func(prefix Path, n *Node) error {
		ordered = append(ordered, n)
		if err := readMsg(nodeMsg); err != nil {
			return err
		}

		n.weight = float64(nodeMsg.Weight)
		if nodeMsg.Name != "" {
			n.Path = make(Path, len(prefix)+1)
			copy(n.Path, prefix)
			n.Path[len(n.Path)-1] = nodeMsg.Name
		}

		if nodeMsg.Children == 0 {
			return nil
		}

		children := int(nodeMsg.Children)
		n.children = make(map[string]*Node, children)
		for i := 0; i < children; i++ {
			child := &Node{}
			if err := readNode(n.Path, child); err != nil {
				return err
			}
			childName := child.Path[len(child.Path)-1]
			n.children[childName] = child
		}

		return nil
	}

	if err := readNode(nil, &f.root); err != nil {
		return err
	}

	f.weight = make(map[nodePair]float64, int(hd.Edges))
	for _, n := range ordered {
		count, err := binary.ReadUvarint(bufR)
		if err != nil {
			return err
		}

		n.adjacent = make([]*Node, count)
		for i := range n.adjacent {
			index, err := binary.ReadUvarint(bufR)
			if err != nil {
				return err
			}
			a := ordered[index]

			n.adjacent[i] = a

			weightBits, err := binary.ReadUvarint(bufR)
			if err != nil {
				return err
			}
			f.weight[nodePair{n, a}] = math.Float64frombits(weightBits)
		}
	}

	return nil
}

func (f *graphFile) Save() error {
	tmpPath := fmt.Sprintf("%s_tmp%d", f.path, rand.Int())
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer file.Close()

	bufW := bufio.NewWriter(file)
	pBuf := proto.Buffer{}

	uvarintBuf := make([]byte, 10)
	writeUvarint := func(x uint64) error {
		n := binary.PutUvarint(uvarintBuf, x)
		_, err := bufW.Write(uvarintBuf[:n])
		return err
	}

	writeMsg := func(m proto.Message) error {
		sz := proto.Size(m)
		if err := writeUvarint(uint64(sz)); err != nil {
			return err
		}

		pBuf.Reset()
		if err := pBuf.Marshal(m); err != nil {
			return err
		}

		msgBytes := pBuf.Bytes()[:sz]
		_, err := bufW.Write(msgBytes)
		return err
	}

	// Write header.
	hd := &filegraphpb.Header{
		Edges:  int64(len(f.weight)),
		Commit: f.commit,
	}
	if err := writeMsg(hd); err != nil {
		return err
	}

	// Write nodes.
	nMsg := &filegraphpb.Node{}
	indices := map[*Node]int{}
	err = f.root.visitDeterministicOrder(func(n *Node) error {
		indices[n] = len(indices)
		*nMsg = filegraphpb.Node{
			Weight:   float32(n.weight),
			Children: int64(len(n.children)),
		}
		if n != &f.root {
			_, nMsg.Name = n.Path.Split()
		}
		return writeMsg(nMsg)
	})
	if err != nil {
		return err
	}

	// Write edges.
	err = f.root.visitDeterministicOrder(func(n *Node) error {
		if err := writeUvarint(uint64(len(n.adjacent))); err != nil {
			return err
		}

		for _, a := range n.adjacent {
			if err := writeUvarint(uint64(indices[a])); err != nil {
				return err
			}
			w := f.weight[nodePair{n, a}]
			if err := writeUvarint(math.Float64bits(w)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err := bufW.Flush(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, f.path)
}

// LoadGraphFromRepo loads a filegrpah from a .git repository.
func LoadGraphFromRepo(ctx context.Context, repoDir string) (*Graph, error) {
	gitDir, err := git.AbsoluteGitPath(repoDir)
	if err != nil {
		return nil, err
	}

	file := graphFile{
		path: filepath.Join(gitDir, "filegraph"),
	}

	start := time.Now()
	switch err := file.Load(); {
	case os.IsNotExist(err):
		logging.Infof(ctx, "populating cache; this may take minutes...")
	case err != nil:
		logging.Warningf(ctx, "cache is corrupted; populating cache; this may take minutes...")
	default:
		logging.Infof(ctx, "loaded cache in %s", time.Since(start))
	}

	processed := 0
	dirty := false
	err = readCommits(ctx, repoDir, file.commit, func(c commit) error {
		if err := file.addCommit(c); err != nil {
			return err
		}
		dirty = true

		processed++
		if processed%10000 == 0 {
			start := time.Now()
			if err := file.Save(); err != nil {
				logging.Errorf(ctx, "failed to save cache: %s", err)
			}
			fmt.Printf("processed %d commits so far; saved in %s\n", processed, time.Since(start))
			dirty = false
		}
		file.commit = c.Hash
		return nil
	})
	if err != nil {
		return nil, err
	}

	if dirty {
		if err := file.Save(); err != nil {
			logging.Errorf(ctx, "failed to save cache: %s", err)
		}
	}

	return &file.Graph, nil
}
