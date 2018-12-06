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
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"

	"github.com/dustin/go-humanize"
)

// Downloader is a high level interface to an isolatedclient.Client.
//
// Downloader provides functionality to download full isolated trees.
type Downloader struct {
	// TODO(iannucci,maruel): remove "Canceler" in favor of Context.Cancel().
	common.Canceler

	// Immutable variables.
	ctx context.Context

	// fileCb is called back once with every downloaded file name.
	fileCb func(string, isolated.File)

	// FileStatsCb is called back
	fileStatsCb func(FileStats, time.Duration)

	c         *isolatedclient.Client
	maxJobs   int
	rootHash  isolated.HexDigest
	outputDir string

	// Mutable variables.
	mu       sync.Mutex
	started  bool
	finished bool

	err errors.MultiError

	interval    time.Duration
	maxInterval time.Duration
	isoMap      map[isolated.HexDigest]*isolated.Isolated

	fileStats       FileStats
	lastFileStatsCb time.Time

	// dirCache is a cache of known existing directories which is extended
	// and read from by ensureDir.
	muCache  sync.RWMutex
	dirCache stringset.Set

	// pool is a goroutine priority pool which manages jobs to download
	// isolated trees and files.
	//
	// TODO(iannucci,maruel): migrate priority features into
	// `go.chromium.org/luci/common/sync/parallel` and remove
	// `go.chromium.org/luci/client/internal/common`
	pool common.GoroutinePriorityPool
}

// New returns a Downloader instance, good to download one isolated once.
//
// You can set tracking callbacks on the Downloader
//
// ctx will be used for logging and clock.
func New(ctx context.Context, c *isolatedclient.Client, hash isolated.HexDigest,
	outputDir string) *Downloader {
	ret := &Downloader{
		Canceler:  common.NewCanceler(),
		ctx:       ctx,
		c:         c,
		maxJobs:   8,
		dirCache:  stringset.New(0),
		rootHash:  hash,
		outputDir: outputDir,
		isoMap:    map[isolated.HexDigest]*isolated.Isolated{},
	}
	ret.SetFileStatsMaxInterval(time.Second * 5)
	return ret
}

// SetFileCallback allows you to set a callback function that will be called
// with every file name which is extracted to disk by the Downloader.
//
// This callback should execute quickly (e.g. push to channel, append to list),
// as it will partially block the process of the download.
//
// Tarball archives behave a bit differently. The callback will be called for
// individual files in the tarball, but the 'Digest' field will be empty. The
// Size and Mode fields will be populated, however. The callback will ALSO be
// called for the tarfile as a whole.
//
// Calling after the Downloader has started will return an error.
func (d *Downloader) SetFileCallback(cb func(name string, f isolated.File)) error {
	if err := d.assertNotStarted(); err != nil {
		return errors.Annotate(err, "cannot call SetFileCallback").Err()
	}
	d.fileCb = cb
	return nil
}

// SetFileStatsCallback allows you to set a callback function that will be
// called at "interval" intervals with relevant statistics.
//
// This callback should execute quickly (e.g. push to channel, append to list,
// log to console), as it will partially block the process of the download.
// However, since it's only called once every "interval" amount of time, being
// a bit slow here (e.g. doing console IO) isn't the worst.
//
// To allow this callback to actuate meaningfully for small downloads, this will
// be called more frequently at the beginning of the download, and will taper
// off to the maxInterval set by SetFileStatsMaxInterval.
//
// Calling after the Downloader has started will return an error.
func (d *Downloader) SetFileStatsCallback(cb func(FileStats, time.Duration)) error {
	if err := d.assertNotStarted(); err != nil {
		return errors.Annotate(err, "cannot call SetFileStatsCallback").Err()
	}
	d.fileStatsCb = cb
	return nil
}

// SetFileStatsMaxInterval changes the maximum interval that the file stats
// callback will be called at. By default this is 5 seconds.
//
// At the beginning of the download the interval is 100ms, but it will ramp up
// to the provided maxInterval. If you specify a maxInterval smaller than 100ms,
// there will be no ramp up, just a fixed interval at the one you specify here.
//
// Calling after the Downloader has started will return an error.
func (d *Downloader) SetFileStatsMaxInterval(maxInterval time.Duration) error {
	if err := d.assertNotStarted(); err != nil {
		return errors.Annotate(err, "cannot call SetFileStatsMaxInterval").Err()
	}
	d.interval = 100 * time.Millisecond
	if d.interval > maxInterval {
		d.interval = maxInterval
	}
	d.maxInterval = maxInterval
	return nil
}

// SetMaxConcurrentJobs sets the depth of the parallel worker pool.
//
// By default this is 8.
//
// Calling after the Downloader has started will return an error.
func (d *Downloader) SetMaxConcurrentJobs(jobs int) error {
	if err := d.assertNotStarted(); err != nil {
		return errors.Annotate(err, "cannot call SetMaxConcurrentJobs").Err()
	}
	d.maxJobs = jobs
	return nil
}

// Start begins downloading the isolated.
func (d *Downloader) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return
	}
	d.started = true

	d.pool = common.NewGoroutinePriorityPool(d.maxJobs, d.Canceler)

	if err := d.ensureDir(d.outputDir); err != nil {
		d.addError(isolatedType, "<isolated setup>", err)
		d.Canceler.Close()
		return
	}

	// Start downloading the isolated tree in the work pool.
	d.scheduleIsolatedJob(d.rootHash)
}

// Wait waits for the completion of the download, and returns either `nil` if
// no errors occured during the operation, or an `errors.MultiError` otherwise.
//
// This will Start() the Downloader, if it hasn't been started already.
//
// Calling this many times is safe (and will always return the same thing).
func (d *Downloader) Wait() error {
	d.Start()
	_ = d.pool.Wait()
	_ = d.Canceler.Close()
	d.updateFileStats(func(s *FileStats) bool {
		ret := d.finished == false
		d.finished = true
		return ret
	})
	if d.err != nil {
		return d.err
	}
	return nil
}

// CmdAndCwd returns the effective command and relative_cwd entries
// from the fetched isolated.
//
// Must be called after the Downloader is completed.
//
// Note that new uses of isolated should NOT use cmd or relative_cwd!
func (d *Downloader) CmdAndCwd() ([]string, string, error) {
	d.mu.Lock()
	finished := d.finished
	d.mu.Unlock()
	if !finished {
		return nil, "", errors.New(
			"can only call CumulativeIsolated on a finished Downloader")
	}

	if d.err != nil {
		return nil, "", d.err
	}

	queue := []*isolated.Isolated{d.isoMap[d.rootHash]}

	for len(queue) > 0 {
		iso := queue[0]

		if len(iso.Command) > 0 {
			return iso.Command, iso.RelativeCwd, nil
		}

		toPrepend := []*isolated.Isolated{}
		for _, inc := range iso.Includes {
			toPrepend = append(toPrepend, d.isoMap[inc])
		}
		queue = append(toPrepend, queue[1:]...)
	}

	return nil, "", nil
}

func (d *Downloader) assertNotStarted() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.started {
		return errors.New("download already started")
	}
	return nil
}

func (d *Downloader) addError(ty downloadType, name string, err error) {
	err = errors.Annotate(err, "%s %s", ty, name).Err()
	d.mu.Lock()
	d.err = append(d.err, err)
	d.mu.Unlock()
}

func (d *Downloader) startFile(size *int64) {
	d.updateFileStats(func(s *FileStats) bool {
		s.CountScheduled++
		if size != nil && *size > 0 {
			s.BytesScheduled += uint64(*size)
		}
		return false
	})
}

func (d *Downloader) completeFile(name string, f *isolated.File) {
	d.updateFileStats(func(s *FileStats) bool {
		s.CountCompleted++
		return false
	})
	if d.fileCb != nil {
		d.fileCb(name, *f)
	}
}

func (d *Downloader) setIsolated(hash isolated.HexDigest, i isolated.Isolated) {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Omit files, they'll be called back with fileCb, and take up most of the
	// space.
	i.Files = nil
	d.isoMap[hash] = &i
}

func (d *Downloader) updateFileStats(lockedCb func(*FileStats) bool) {
	var stats FileStats
	var span time.Duration
	doCall := false

	func() {
		d.mu.Lock()
		defer d.mu.Unlock()

		maybeCall := lockedCb(&d.fileStats)

		if d.fileStatsCb != nil {
			doCall = maybeCall
			now := clock.Now(d.ctx)
			span = now.Sub(d.lastFileStatsCb)

			if span >= d.interval {
				if d.interval < d.maxInterval {
					d.interval *= 2
					if d.interval > d.maxInterval {
						d.interval = d.maxInterval
					}
				}
				doCall = true
			}

			if doCall {
				d.lastFileStatsCb = now
				stats = d.fileStats
			}
		}
	}()

	if doCall {
		d.fileStatsCb(stats, span)
	}
}

func (d *Downloader) track(w io.Writer) io.Writer {
	if d.fileStatsCb != nil {
		return &writeTracker{d, w}
	}
	return w
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
		if i == d.outputDir {
			break
		}
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

func (d *Downloader) processFile(name string, details isolated.File) {
	d.startFile(details.Size)

	// Get full local path for file.
	filename := filepath.Join(d.outputDir, name)

	if details.Link != nil {
		d.doSymlink(filename, name, &details)
	} else if details.Type == isolated.TarArchive {
		d.scheduleTarballJob(name, &details)
	} else {
		d.scheduleFileJob(filename, name, &details)
	}
}

func (d *Downloader) doSymlink(filename, name string, details *isolated.File) {
	// Ensure dir exists.
	if err := d.ensureDir(filepath.Dir(filename)); err != nil {
		d.addError(fileType, name, err)
		return
	}

	linkTarget := filepath.Join(d.outputDir, *details.Link)
	if err := os.Symlink(linkTarget, filename); err != nil {
		d.addError(fileType, name, err)
		return
	}
	d.completeFile(name, details)
}

func (d *Downloader) scheduleFileJob(filename, name string, details *isolated.File) {
	d.pool.Schedule(fileType.Priority(), func() {
		// Ensure dir exists.
		if err := d.ensureDir(filepath.Dir(filename)); err != nil {
			d.addError(fileType, name, err)
			return
		}

		mode := 0666
		if details.Mode != nil {
			mode = *details.Mode
		}
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, os.FileMode(mode))
		if err != nil {
			d.addError(fileType, name, err)
			return
		}
		defer f.Close()
		if err := d.c.Fetch(d.ctx, details.Digest, d.track(f)); err != nil {
			d.addError(fileType, name, err)
			return
		}
		d.completeFile(name, details)
	}, func() {
		d.addError(fileType, name, d.CancelationReason())
	})
}

func (d *Downloader) scheduleTarballJob(tarname string, details *isolated.File) {
	hash := details.Digest

	d.pool.Schedule(tarType.Priority(), func() {
		var buf bytes.Buffer
		if err := d.c.Fetch(d.ctx, hash, d.track(&buf)); err != nil {
			d.addError(tarType, string(hash), err)
			return
		}

		tf := tar.NewReader(&buf)
	loop:
		for {
			hdr, err := tf.Next()
			switch err {
			case io.EOF:
				// end of the tarball
				break loop
			case nil:

			default:
				d.addError(tarType, string(hash), err)
				continue
			}

			name := filepath.Clean(hdr.Name)

			// got a file to read
			if hdr.Typeflag != tar.TypeReg {
				d.addError(tarType, string(hash)+":"+name,
					errors.New("not a regular file"))
				continue
			}

			if filepath.IsAbs(name) {
				d.addError(tarType, string(hash)+":"+name,
					errors.New("absolute path"))
				continue
			}

			filename := filepath.Join(d.outputDir, name)
			mode := int(hdr.Mode)
			if err := d.ensureDir(filepath.Dir(filename)); err != nil {
				d.addError(tarType, string(hash)+":"+filename, err)
				continue
			}
			f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, os.FileMode(mode))
			if err != nil {
				d.addError(tarType, string(hash)+":"+filename, err)
				continue
			}
			defer f.Close()
			n, err := io.Copy(f, tf)
			if err != nil {
				d.addError(tarType, string(hash)+":"+filename, err)
				continue
			}
			if n != hdr.Size {
				d.addError(tarType, string(hash)+":"+filename,
					errors.New("failed to copy entire file"))
				continue
			}
			// Fake a File entry for the subfile. Also call startfile so that the
			// started/completed file numbers balance.
			d.startFile(nil)
			d.completeFile(name, &isolated.File{
				Mode: &mode,
				Size: &hdr.Size,
			})
		}

		// Also issue a callback for the overall tarball.
		d.completeFile(tarname, details)
	}, func() {
		d.addError(tarType, string(hash), d.CancelationReason())
	})
}

func (d *Downloader) scheduleIsolatedJob(hash isolated.HexDigest) {
	d.pool.Schedule(isolatedType.Priority(), func() {
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
		d.setIsolated(hash, root)
		for _, node := range root.Includes {
			d.scheduleIsolatedJob(node)
		}
		for name, details := range root.Files {
			d.processFile(name, details)
		}
	}, func() {
		d.addError(isolatedType, string(hash), d.CancelationReason())
	})
}

// FileStats is very basic statistics about the progress of
// a FetchIsolatedTracked operation.
type FileStats struct {
	// These cover the files that the isolated file says to fetch.
	CountScheduled uint64
	CountCompleted uint64

	// These cover the bytes of the files that the isolated file describes, not
	// the bytes of the isolated files themselves.
	//
	// Note that these are potentially served from the local cache, and so you
	// could observe speeds much faster than the network speed :).
	BytesScheduled uint64
	BytesCompleted uint64
}

// StatLine calculates a simple statistics line suitable for logging.
func (f *FileStats) StatLine(previous *FileStats, span time.Duration) string {
	var bytesDownloaded uint64
	if previous != nil {
		bytesDownloaded = f.BytesCompleted - previous.BytesCompleted
	}

	return fmt.Sprintf("Files (%d/%d) - %s / %s - %0.1f%% - %s/s",
		f.CountCompleted, f.CountScheduled,
		humanize.Bytes(f.BytesCompleted), humanize.Bytes(f.BytesScheduled),
		100*float64(f.BytesCompleted)/float64(f.BytesScheduled),
		humanize.Bytes(uint64(float64(bytesDownloaded)/span.Seconds())),
	)
}

type writeTracker struct {
	d *Downloader
	w io.Writer
}

func (w *writeTracker) Write(bs []byte) (n int, err error) {
	w.d.updateFileStats(func(s *FileStats) bool {
		s.BytesCompleted += uint64(len(bs))
		return false
	})
	return w.w.Write(bs)
}

type downloadType int8

const (
	fileType downloadType = iota
	tarType
	isolatedType
)

func (d downloadType) Priority() int64 {
	return int64(d)
}

func (d downloadType) String() string {
	switch d {
	case fileType:
		return "file"
	case tarType:
		return "tarball"
	case isolatedType:
		return "isolated"
	default:
		panic("invalid downloadType")
	}
}
