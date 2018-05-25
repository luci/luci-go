// Copyright 2018 The LUCI Authors.
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

package iotools

import (
	"container/list"
	"fmt"
	"io"
	"sync"
)

// block is a contiguous chunk of data from the original file.
type block struct {
	offset int64
	data   []byte
	last   bool
}

// blocksLRU is LRU of last read blocks, keyed by their offset in the file.
type blocksLRU struct {
	capacity int         // how many blocks we want to cache
	evicted  func(block) // called for each evicted block

	blocks map[int64]*list.Element
	ll     list.List // each element's Value is block{}
}

// init initializes LRU guts.
func (l *blocksLRU) init(capacity int) {
	l.capacity = capacity
	l.blocks = make(map[int64]*list.Element, capacity)
	l.ll.Init()
}

// get returns (block, true) if it's in the cache or (block{}, false) otherwise.
func (l *blocksLRU) get(offset int64) (b block, ok bool) {
	elem, ok := l.blocks[offset]
	if !ok {
		return block{}, false
	}
	l.ll.MoveToFront(elem)
	return elem.Value.(block), true
}

// prepareForAdd removes oldest item from the list if it is at the capacity.
//
// Does nothing if it's not yet full.
func (l *blocksLRU) prepareForAdd() {
	switch {
	case l.ll.Len() > l.capacity:
		panic("impossible")
	case l.ll.Len() == l.capacity:
		oldest := l.ll.Remove(l.ll.Back()).(block)
		delete(l.blocks, oldest.offset)
		l.evicted(oldest)
	}
}

// add adds a block to the cache (perhaps evicting the oldest block).
//
// The caller must verify there's no such block in the cache already using
// get(). Panics if it wasn't done.
func (l *blocksLRU) add(b block) {
	if _, ok := l.blocks[b.offset]; ok {
		panic(fmt.Sprintf("block with offset %d is already in the LRU", b.offset))
	}
	l.prepareForAdd()
	l.blocks[b.offset] = l.ll.PushFront(b)
}

type bufferingReaderAt struct {
	l sync.Mutex
	r io.ReaderAt

	blockSize int
	lru       blocksLRU

	// Available buffers of blockSize length. Note that docs warn that sync.Pool
	// is too heavy for this case and it's better to roll our own mini pool.
	pool [][]byte
}

// NewBufferingReaderAt returns an io.ReaderAt that reads data in blocks of
// configurable size and keeps LRU of recently read blocks.
//
// It is great for cases when data is read sequentially from an io.ReaderAt,
// (e.g. when extracting files using zip.Reader), since by setting large block
// size we can effectively do lookahead reads.
//
// For example, zip.Reader reads data in 4096 byte chunks by default. By setting
// block size to 512Kb and LRU size to 1 we reduce the number of read operations
// significantly (128x), in exchange for the modest amount of RAM.
//
// The reader is safe to user concurrently (just like any ReaderAt), but beware
// that the LRU is shared and all reads from the underlying reader happen under
// the lock, so multiple goroutines may end up slowing down each other.
func NewBufferingReaderAt(r io.ReaderAt, blockSize int, lruSize int) io.ReaderAt {
	if blockSize < 1 {
		panic(fmt.Sprintf("block size should be >= 1, not %d", blockSize))
	}
	if lruSize < 1 {
		panic(fmt.Sprintf("lru size should be >= 1, not %d", lruSize))
	}
	reader := &bufferingReaderAt{
		r:         r,
		blockSize: blockSize,
		// We actually pool at most 1 buffer, since buffers are grabbed from the
		// pool immediately after they are evicted from LRU.
		pool: make([][]byte, 0, 1),
	}
	reader.lru.init(lruSize)
	reader.lru.evicted = func(b block) { reader.recycleBuf(b.data) }
	return reader
}

// grabBuf returns a byte slice of blockSize size.
func (r *bufferingReaderAt) grabBuf() []byte {
	if len(r.pool) != 0 {
		b := r.pool[len(r.pool)-1]
		r.pool = r.pool[:len(r.pool)-1]
		return b
	}
	return make([]byte, r.blockSize)
}

// recycleBuf is called when the buffer is no longer needed to put it for reuse.
func (r *bufferingReaderAt) recycleBuf(b []byte) {
	if cap(b) != r.blockSize {
		panic("trying to return a buffer not initially requested via grabBuf")
	}
	if len(r.pool)+1 > cap(r.pool) {
		panic("unexpected growth of byte buffer pool beyond capacity")
	}
	r.pool = append(r.pool, b[:cap(b)])
}

// readBlock returns the block of the file (of blockSize size) at an offset.
//
// Assumes the caller does not retain the returned buffer (just reads from it
// and forgets it right away, all under the lock).
//
// Returns one of:
//   (full block, nil) on success
//   (partial or full block, io.EOF) when reading the final block
//   (partial block, err) on read errors
func (r *bufferingReaderAt) readBlock(offset int64) (data []byte, err error) {
	// Have it cached already?
	if b, ok := r.lru.get(offset); ok {
		data = b.data
		if b.last {
			err = io.EOF
		}
		return
	}

	// Kick out the oldest block (if any) to move its buffer to the free buffers
	// pool and then immediately grab this buffer.
	r.lru.prepareForAdd()
	data = r.grabBuf()

	// Read the block from the underlying reader.
	read, err := r.r.ReadAt(data, offset)
	data = data[:read]

	// ReadAt promises that it returns nil only if it read the full block. We rely
	// on this later, so double check.
	if err == nil && read != r.blockSize {
		panic(fmt.Sprintf("broken ReaderAt: should have read %d bytes, but read only %d", r.blockSize, read))
	}

	// Cache fully read blocks and the partially read last block, but skip blocks
	// that were read partially due to unexpected errors.
	if err == nil || err == io.EOF {
		r.lru.add(block{
			offset: offset,
			data:   data,
			last:   err == io.EOF,
		})
	} else {
		// Caller promises not to retain 'data', so we can return it right away.
		r.recycleBuf(data)
	}

	return data, err
}

// ReadAt implements io.ReaderAt interface.
func (r *bufferingReaderAt) ReadAt(p []byte, offset int64) (read int, err error) {
	if len(p) == 0 {
		return r.r.ReadAt(p, offset)
	}

	r.l.Lock()
	defer r.l.Unlock()

	bs := int64(r.blockSize)
	blockOff := int64((offset / bs) * bs) // block-aligned offset

	// Sequentially read blocks that intersect with the requested segment.
	for {
		// err here may be EOF or some other error. We consume all data first and
		// deal with errors later.
		data, err := r.readBlock(blockOff)

		// The first block may be read from the middle, since 'min' may be less than
		// 'offset'.
		if offset > blockOff {
			pos := offset - blockOff // position inside the block to read from
			if pos < int64(len(data)) {
				data = data[pos:] // grab the tail of the block
			} else {
				data = nil // we probably hit EOF before the requested offset
			}
		}

		// 'copy' copies min of len(data) and whatever space is left in 'p', so this
		// is always safe. The last block may be copied partially (if there's no
		// space left in 'p').
		read += copy(p[read:], data)

		switch {
		case read == len(p):
			// We managed to read everything we wanted, ignore the last error, if any.
			return read, nil
		case err != nil:
			return read, err
		}

		// The last read was successful. Per ReaderAt contract (that we double
		// checked in readBlock) it means it read ALL requested data (and we request
		// 'bs' bytes). So move on to the next block.
		blockOff += bs
	}
}
