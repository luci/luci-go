// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package bufferpool implements a pool of bytes.Buffer instances backed by
// a sync.Pool. The goal of using a buffer pool is that, in exchange for some
// locking overhead, the user can avoid iteratively reallocating buffers for
// frequently used purposes.
//
// Ideal usage of bufferpool is with like-purposed buffers in order to encourage
// the pool to contain buffers sized to that specific purpose. If this is
// correctly implemented, the buffers in the pool should generally come into
// existence, grow to the purpose's size need, and then remain there without
// further allocation.
//
// An overly-broad-purposed pool, on the other hand, will have its buffers grow
// to max(purpose...) size and consequently contain more large buffers than
// necessary.
package bufferpool

import (
	"bytes"
	"sync"
)

// P is a pool of buffers. The zero value is an initialized but empty pool.
//
// P must be passed around as reference, not value.
type P struct {
	pool sync.Pool
}

// Get returns a Buffer. When the caller is finished with the Buffer, they
// should call Release to return it to its pool.
func (p *P) Get() *Buffer {
	buf, ok := p.pool.Get().(*bytes.Buffer)
	if !ok {
		buf = &bytes.Buffer{}
	}

	return &Buffer{
		Buffer: buf,
		p:      p,
	}
}

// Buffer is a bytes.Buffer that is bound to a pool. It should not be used
// directly, but rather obtained through calling Get on a P instance.
type Buffer struct {
	*bytes.Buffer

	p *P
}

// Clone clones the contents of the buffer's Bytes, returning an indepdent
// duplicate []byte.
func (b *Buffer) Clone() []byte {
	return append([]byte(nil), b.Bytes()...)
}

// Release returns this Buffer to its pool.
//
// After calling Release, no accesses may be made to b or its internal data.
// If its data is to be retained, it must be cloned prior to Release (see
// Clone).
func (b *Buffer) Release() {
	p := b.p
	if p == nil {
		panic("double Release")
	}

	b.p = nil
	b.Reset()
	p.pool.Put(b.Buffer)
}
