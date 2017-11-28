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

	"go.chromium.org/luci/client/internal/common"
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
	common.Canceler

	// Immutable variables.
	ctx context.Context
	c   *isolatedclient.Client

	// Mutable variables.

	// err is the accumulation of all errors encountered when attemping
	// any of the jobs that have been scheduled.
	mu  sync.Mutex
	err errors.MultiError

	// pool is a goroutine priority pool which manages jobs to download
	// isolated trees and files.
	pool common.GoroutinePriorityPool
}

// New returns a Downloader instance.
//
// ctx will be used for logging.
func New(ctx context.Context, c *isolatedclient.Client, maxConcurrentJobs int) *Downloader {
	canceler := common.NewCanceler()
	pool := common.NewGoroutinePriorityPool(maxConcurrentJobs, canceler)
	return &Downloader{
		Canceler: canceler,
		ctx: ctx,
		c:   c,
		pool: pool,
	}
}

// FetchIsolated downloads an entire isolated tree into a specified output directory.
//
// `gen` is called in a thread-unsafe way, so any mutable state captured by `gen`
// should be locked or otherwise written to atomically to prevent data corruption.
//
// This method is non-blocking.
func (d *Downloader) FetchIsolated(hash isolated.HexDigest, gen WriterGenerator) {
	d.pool.Schedule(isolatedType.Priority(), func() {
		d.doIsolatedJob(hash, gen)
	}, func() {
		d.addError(isolatedType, hash, d.CancelationReason())
	})
}

// FetchFile downloads a single file from the isolateserver by hash and writes it to `out`.
//
// This method is non-blocking.
func (d *Downloader) FetchFile(hash isolated.HexDigest, out io.WriteCloser) {
	d.pool.Schedule(fileType.Priority(), func() {
		d.doFileJob(hash, out)
	}, func() {
		d.addError(fileType, hash, d.CancelationReason())
	})
}

// Close cleans up the downloader and blocks until all downloads complete.
//
// Close returns an error summarizing all errors that were encountered.
func (d *Downloader) Close() error {
	_ = d.pool.Wait()
	_ = d.Canceler.Close()
	if len(d.err) > 0 {
		return d.err
	}
	return nil
}

type downloadType int8

const (
	fileType     downloadType = 0
	isolatedType downloadType = 1
)

func (d downloadType) Priority() int64 {
	return int64(d)
}

func (d downloadType) String() string {
	switch d {
	case fileType:
		return "file"
	case isolatedType:
		return "isolated"
	default:
		panic("invalid downloadType")
	}
}

func (d *Downloader) addError(ty downloadType, hash isolated.HexDigest, err error) {
	err = errors.Annotate(err, "%s %s", ty, hash).Err()
	d.mu.Lock()
	d.err = append(d.err, err)
	d.mu.Unlock()
}

func (d *Downloader) doFileJob(hash isolated.HexDigest, out io.WriteCloser) {
	if err := d.c.Fetch(d.ctx, hash, out); err != nil {
		d.addError(fileType, hash, err)
	}
	if err := out.Close(); err != nil {
		d.addError(fileType, hash, err)
	}
}

func (d *Downloader) doIsolatedJob(hash isolated.HexDigest, gen WriterGenerator) {
	var buf bytes.Buffer
	if err := d.c.Fetch(d.ctx, hash, &buf); err != nil {
		d.addError(isolatedType, hash, err)
		return
	}
	var root isolated.Isolated
	if err := json.Unmarshal(buf.Bytes(), &root); err != nil {
		d.addError(isolatedType, hash, err)
		return
	}
	for _, node := range root.Includes {
		d.FetchIsolated(node, gen)
	}
	for name, details := range root.Files {
		f, err := gen(name)
		if err != nil {
			d.addError(isolatedType, hash, err)
			return
		}
		d.FetchFile(details.Digest, f)
	}
}
