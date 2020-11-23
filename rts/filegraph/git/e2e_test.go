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
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/filegraph/internal/gitutil"
)

// BenchmarkE2E measures performance of this package end to end:
//  - Builds the graph from a git checkout
//  - Serializes it.
//  - Deserializes it.
//  - Runs queries from some files
//
// Uses the git checkout at path in the FILEGRAPH_BENCH_CHECKOUT environment
// variable. If it is not defined, then uses current repo (luci-go.git).
func BenchmarkE2E(b *testing.B) {
	ctx := context.Background()
	repoDir := benchRepoDir(b)

	var g *Graph

	// First, build the graph from scratch.
	b.Run("Build", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			g = &Graph{}
			if err := g.Update(ctx, repoDir, "refs/heads/master", UpdateOptions{}); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		printStats(g, b)
	})

	// Serialize it.
	buf := &bytes.Buffer{}
	b.Run("Write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf.Reset()
			if err := g.Write(buf); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Deserialize it.
	b.Run("Read", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			r := bufio.NewReader(bytes.NewReader(buf.Bytes()))
			if err := g.Read(r); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Run queries for each top-level file.
	for _, n := range g.root.children {
		n := n
		if len(n.children) > 0 {
			continue
		}
		b.Run("Query-"+strings.TrimPrefix(n.name, "//"), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				q := filegraph.Query{Sources: []filegraph.Node{n}, EdgeReader: &EdgeReader{}}
				q.Run(func(*filegraph.ShortestPath) bool {
					return true
				})
			}
		})
	}
}

func printStats(g *Graph, b *testing.B) {
	nodes := 0
	edges := 0
	g.root.visit(func(n *node) bool {
		nodes++
		edges += len(n.edges)
		return true
	})
	b.Logf("%d nodes, %d edges", nodes, edges)
}

func benchRepoDir(b *testing.B) string {
	if repoDir := os.Getenv("FILEGRAPH_BENCH_CHECKOUT"); repoDir != "" {
		return repoDir
	}

	cwd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	repoDir, err := gitutil.TopLevel(cwd)
	if err != nil {
		b.Fatal(err)
	}
	return repoDir
}
