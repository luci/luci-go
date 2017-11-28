// Copyright 2017 The LUCI Authors.
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

package downloader

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

// WriterGenerator is a function that given some name, generates an io.Writer
// or return an error.
type WriterGenerator func(string) (io.WriteCloser, error)

// Downloader is an high level interface to an isolatedclient.Client.
//
// A Downloader asynchronously downloads files or isolated trees from
// isolateserver. It spins up a goroutine for every file that needs to be
// downloaded (that is, it uses as many goroutines as it needs).
type Downloader struct {
	// Immutable variables.
	ctx context.Context
	c   *isolatedclient.Client

	// Mutable variables.

	// wg is a wait group intended to signal the completion of all
	// in-flight jobs.
	wg sync.WaitGroup

	// merr is the accumulation of all errors encountered when attemping
	// any of the jobs that have been scheduled.
	merr    errors.MultiError
	errLock sync.Mutex
}

// New returns a Downloader instance.
//
// ctx will be used for logging.
func New(ctx context.Context, c *isolatedclient.Client) *Downloader {
	return &Downloader{
		ctx: ctx,
		c:   c,
	}
}

func (d *Downloader) addError(err error) {
	d.errLock.Lock()
	d.merr = append(d.merr, err)
	d.errLock.Unlock()
}

func (d *Downloader) doFileJob(hash isolated.HexDigest, out io.WriteCloser) {
	if err := d.c.Fetch(d.ctx, hash, out); err != nil {
		d.addError(err)
	}
	if err := out.Close(); err != nil {
		d.addError(err)
	}
	d.wg.Done()
}

func (d *Downloader) doIsolatedJob(hash isolated.HexDigest, gen WriterGenerator) {
	var rootBuffer bytes.Buffer
	if err := d.c.Fetch(d.ctx, hash, &rootBuffer); err != nil {
		d.addError(err)
		return
	}
	var root isolated.Isolated
	if err := json.Unmarshal(rootBuffer.Bytes(), &root); err != nil {
		d.addError(err)
		return
	}
	for _, node := range root.Includes {
		d.wg.Add(1)
		go d.doIsolatedJob(node, gen)
	}
	for name, details := range root.Files {
		f, err := gen(name)
		if err != nil {
			d.addError(err)
			return
		}
		d.wg.Add(1)
		go d.doFileJob(details.Digest, f)
	}
	d.wg.Done()
}

// FetchIsolated downloads an entire isolated tree into a specified output directory.
//
// `gen` is called in a thread-unsafe way, so any mutable state captured by `gen`
// should be locked or otherwise written to atomically to prevent data corruption.
//
// This method is non-blocking.
func (d *Downloader) FetchIsolated(hash isolated.HexDigest, gen WriterGenerator) {
	d.wg.Add(1)
	go d.doIsolatedJob(hash, gen)
}

// FetchFile downloads a single file from the isolateserver by hash and writes it to `out`.
//
// This method is non-blocking.
func (d *Downloader) FetchFile(hash isolated.HexDigest, out io.WriteCloser) {
	d.wg.Add(1)
	go d.doFileJob(hash, out)
}

// Close cleans up the downloader and blocks until all downloads complete.
//
// Close returns an error summarizing all errors that were encountered.
func (d *Downloader) Close() error {
	d.wg.Wait()
	return d.merr
}
