// Copyright 2016 The LUCI Authors.
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

// Package cryptorand implements a mockable source or crypto strong randomness.
//
// In real world scenario it is same source as provided by crypt/rand. In tests
// it is replaced with reproducible, not really random stream of bytes.
package cryptorand

import (
	"context"
	crypto "crypto/rand"
	"io"
	math "math/rand"
	"sync"
)

var key = "holds crypto.Reader for cryptorand"

// Get returns an io.Reader that emits random stream of bytes.
//
// Usually this returns crypto/rand.Reader, but unit tests may replace it with
// a mock by using 'MockForTest' function.
func Get(ctx context.Context) io.Reader {
	if r, _ := ctx.Value(&key).(io.Reader); r != nil {
		return r
	}
	return crypto.Reader
}

// Read is a helper that reads bytes from random source using io.ReadFull.
//
// On return, n == len(b) if and only if err == nil.
func Read(ctx context.Context, b []byte) (n int, err error) {
	return io.ReadFull(Get(ctx), b)
}

// MockForTest installs deterministic source of 'randomness' in the context.
//
// Must not be used outside of tests.
func MockForTest(ctx context.Context, seed int64) context.Context {
	return MockForTestWithIOReader(ctx, &notRandom{r: math.New(math.NewSource(seed))})
}

// notRandom is io.Reader that uses math/rand generator.
type notRandom struct {
	sync.Mutex
	r *math.Rand
}

func (r *notRandom) Read(p []byte) (n int, err error) {
	r.Lock()
	defer r.Unlock()

	for i := range p {
		p[i] = byte(r.r.Intn(256))
	}
	return len(p), nil
}

// MockForTestWithIOReader installs the provided io reader as the source of
// 'randomness' in the context.
//
// Must not be used outside of tests.
func MockForTestWithIOReader(ctx context.Context, r io.Reader) context.Context {
	return context.WithValue(ctx, &key, r)
}
