package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/logging"
)

type graphFile struct {
	path   string
	commit string
	graph
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

	hd := &Header{}
	if err := readMsg(hd); err != nil {
		return err
	}
	f.commit = hd.Commit

	var ordered []*node
	nodeMsg := &Node{}
	var readNode func(prefix Path, n *node) error
	readNode = func(prefix Path, n *node) error {
		ordered = append(ordered, n)
		if err := readMsg(nodeMsg); err != nil {
			return err
		}

		n.weight = float64(nodeMsg.Weight)
		if nodeMsg.Name != "" {
			n.path = make(Path, len(prefix)+1)
			copy(n.path, prefix)
			n.path[len(n.path)-1] = nodeMsg.Name
		}

		if nodeMsg.Children == 0 {
			return nil
		}

		n.children = make(map[string]*node, int(nodeMsg.Children))
		children := int(nodeMsg.Children)

		for i := 0; i < children; i++ {
			child := &node{}
			if err := readNode(n.path, child); err != nil {
				return err
			}
			childName := child.path[len(child.path)-1]
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

		n.adjacent = make([]*node, count)
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
	file, err := os.Create(f.path)
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
	hd := &Header{
		Edges:  int64(len(f.weight)),
		Commit: f.commit,
	}
	if err := writeMsg(hd); err != nil {
		return err
	}

	// Write nodes.
	nMsg := &Node{}
	indices := map[*node]int{}
	err = f.visit(func(n *node) error {
		indices[n] = len(indices)
		*nMsg = Node{
			Weight:   float32(n.weight),
			Children: int64(len(n.children)),
		}
		if n != &f.root {
			_, nMsg.Name = n.path.Split()
		}
		return writeMsg(nMsg)
	})
	if err != nil {
		return err
	}

	// Write edges.
	err = f.visit(func(n *node) error {
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
	return file.Close()
}

func loadGraphFromRepo(ctx context.Context, repoDir string) (*graph, error) {
	gitDir, err := absoluteGitPath(repoDir)
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
		if err := file.AddCommit(c); err != nil {
			return err
		}
		dirty = true

		processed++
		if processed%10000 == 0 {
			if err := file.Save(); err != nil {
				logging.Errorf(ctx, "failed to save cache: %s", err)
			}
			fmt.Printf("processed %d commits\n", processed)
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

	return &file.graph, nil
}
