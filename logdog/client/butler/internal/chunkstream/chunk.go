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

// Chunk wraps a fixed-size byte buffer. It is the primary interface used by
// the chunk library.
//
// A Chunk reference should be released once the user is finished with it. After
// being released, it may no longer be accessed.
type Chunk interface {
	// Bytes returns the underlying byte slice contained by this Chunk.
	Bytes() []byte

	// Release releases the Chunk. After being released, a Chunk's methods must no
	// longer be used.
	//
	// It is a good idea to set your chunk variable to nil after releasing it.
	Release()
}
