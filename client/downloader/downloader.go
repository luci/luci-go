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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

// Downloader is a high level interface to an isolatedclient.Client.
//
// Downloader provides functionality to download full isolated trees.
type Downloader struct {
	common.Canceler

	// Immutable variables.
	ctx     context.Context
	c       *isolatedclient.Client
	maxJobs int

	// Mutable variables.

	// err is the accumulation of all errors encountered when attemping
	// any of the jobs that have been scheduled.
	//
	// files is the accumulation of all files encountered when attempting
	// an isolated job.
	//
	// Both fields are protected by mu.
	mu    sync.Mutex
	err   errors.MultiError
	files []string

	// dirCache is a cache of known existing directories which is extended
	// and read from by ensureDir.
	muCache  sync.RWMutex
	dirCache stringset.Set

	muFiles sync.Mutex

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
		ctx:      ctx,
		c:        c,
		pool:     pool,
		maxJobs:  maxConcurrentJobs,
		dirCache: stringset.New(0),
	}
}

// FetchIsolated downloads an entire isolated tree into a specified output directory.
//
// Returns a list of paths relative to outputDir for all downloaded files.
//
// Note that this method is not thread-safe and it does not flush the Downloader's directory cache.
func (d *Downloader) FetchIsolated(hash isolated.HexDigest, outputDir string) ([]string, error) {
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return nil, err
	}
	d.dirCache.Add(outputDir)
	d.files = make([]string, 0, 1024)

	// Start downloading the isolated tree in the work pool.
	d.pool.Schedule(isolatedType.Priority(), func() {
		d.doIsolatedJob(hash, outputDir)
	}, func() {
		d.addError(isolatedType, string(hash), d.CancelationReason())
	})

	// First wait for the work in the pool to finish, then wait for
	// the work in the files goroutine to finish.
	_ = d.pool.Wait()
	_ = d.Canceler.Close()

	// If any error occurred, return it and reset the err.
	if len(d.err) > 0 {
		err := d.err
		d.err = nil
		return nil, err
	}
	result := make([]string, len(d.files))
	copy(result, d.files)
	d.files = nil
	return result, nil
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

// ensureDir ensures that the directory dir exists.
func (d *Downloader) ensureDir(dir string) error {
	// Fast path: if the cache has the directory, we're done.
	d.muCache.RLock()
	cached := d.dirCache.Has(dir)
	d.muCache.RUnlock()
	if cached {
		return nil
	}

	// Slow path: collect the directory and its parents, then create
	// them and add them to the cache.
	d.muCache.Lock()
	defer d.muCache.Unlock()
	parents := make([]string, 0, 1)
	for i := dir; i != "" && !d.dirCache.Has(i); i = filepath.Dir(i) {
		parents = append(parents, i)
	}
	for i := len(parents) - 1; i >= 0; i-- {
		if err := os.Mkdir(parents[i], os.ModePerm); err != nil && !os.IsExist(err) {
			return err
		}
		d.dirCache.Add(parents[i])
	}
	return nil
}

func (d *Downloader) addError(ty downloadType, name string, err error) {
	err = errors.Annotate(err, "%s %s", ty, name).Err()
	d.mu.Lock()
	d.err = append(d.err, err)
	d.mu.Unlock()
}

func (d *Downloader) addFile(name string) {
	d.mu.Lock()
	d.files = append(d.files, name)
	d.mu.Unlock()
}

func (d *Downloader) doFileJob(name string, details *isolated.File, outputDir string) {
	// Get full local path for file.
	name = filepath.Clean(name)
	filename := filepath.Join(outputDir, name)

	// Ensure dir exists.
	if err := d.ensureDir(filepath.Dir(filename)); err != nil {
		d.addError(fileType, name, err)
		return
	}

	// Handle the file specially if it's a symlink.
	if details.Link != nil {
		linkTarget := filepath.Join(outputDir, *details.Link)
		if err := os.Symlink(linkTarget, filename); err != nil {
			d.addError(fileType, name, err)
			return
		}
		d.addFile(name)
		return
	}

	// Every other kind of file just fetch the bytes.
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, os.FileMode(details.Mode))
	if err != nil {
		d.addError(fileType, name, err)
		return
	}
	defer f.Close()
	if err := d.c.Fetch(d.ctx, details.Digest, f); err != nil {
		d.addError(fileType, name, err)
		return
	}
	d.addFile(name)
}

func (d *Downloader) doIsolatedJob(hash isolated.HexDigest, outputDir string) {
	var buf bytes.Buffer
	if err := d.c.Fetch(d.ctx, hash, &buf); err != nil {
		d.addError(isolatedType, string(hash), err)
		return
	}
	var root isolated.Isolated
	if err := json.Unmarshal(buf.Bytes(), &root); err != nil {
		d.addError(isolatedType, string(hash), err)
		return
	}
	for _, node := range root.Includes {
		node := node
		d.pool.Schedule(isolatedType.Priority(), func() {
			d.doIsolatedJob(node, outputDir)
		}, func() {
			d.addError(isolatedType, string(hash), d.CancelationReason())
		})
	}
	for name, details := range root.Files {
		name := name
		details := details
		d.pool.Schedule(fileType.Priority(), func() {
			d.doFileJob(name, &details, outputDir)
		}, func() {
			d.addError(fileType, name, d.CancelationReason())
		})
	}
}
