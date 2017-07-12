// Copyright 2015 The LUCI Authors.
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

package chunkstream

import (
	"sync"
)

var chunkNodePool = sync.Pool{
	New: func() interface{} {
		return &chunkNode{}
	},
}

func newChunkNode(c Chunk) *chunkNode {
	n := chunkNodePool.Get().(*chunkNode)
	n.Chunk = c
	return n
}

// chunkNode wraps a Chunk interface in a linked-list node.
type chunkNode struct {
	Chunk

	next *chunkNode
}

func (n *chunkNode) release() {
	n.Chunk.Release()
	n.Chunk = nil
	n.next = nil
	chunkNodePool.Put(n)
}

func (n *chunkNode) length() int {
	return len(n.Bytes())
}
