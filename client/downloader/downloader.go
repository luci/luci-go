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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	humanize "github.com/dustin/go-humanize"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/cache"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

// Downloader is a high level interface to an isolatedclient.Client.
//
// Downloader provides functionality to download full isolated trees.
type Downloader struct {
	// Immutable variables.
	ctx    context.Context
	cancel func()

	c         *isolatedclient.Client
	rootHash  isolated.HexDigest
	outputDir string
	options   Options

	// Mutable variables.
	mu       sync.Mutex
	started  bool
	finished bool

	err errors.MultiError

	interval time.Duration
	isoMap   map[isolated.HexDigest]*isolated.Isolated

	fileStats       FileStats
	lastFileStatsCb time.Time

	// dirCache is a cache of known existing directories which is extended
	// and read from by ensureDir.
	muCache  sync.RWMutex
	dirCache stringset.Set

	// pool is a goroutine priority pool which manages jobs to download
	// isolated trees and files.
	pool *common.GoroutinePriorityPool
}

// Options are some optional bits you can pass to New.
type Options struct {
	// FileCallback allows you to set a callback function that will be called with
	// every file name and metadata which is extracted to disk by the Downloader.
	//
	// This callback should execute quickly (e.g. push to channel, append to list),
	// as it will partially block the process of the download.
	//
	// Tarball archives behave a bit differently. The callback will be called for
	// individual files in the tarball, but the 'Digest' field will be empty. The
	// Size and Mode fields will be populated, however. The callback will ALSO be
	// called for the tarfile as a whole (but the tarfile will not actually exist
	// on disk).
	FileCallback func(string, *isolated.File)

	// FileStatsCallback is a callback function that will be called at intervals
	// with relevant statistics (see MaxFileStatsInterval).
	//
	// This callback should execute quickly (e.g. push to channel, append to list,
	// etc.) as it will partially block the process of the download.  However,
	// since it's only called once every "interval" amount of time, being a bit
	// slow here (e.g. doing console IO) isn't the worst.
	//
	// To allow this callback to actuate meaningfully for small downloads, this
	// will be called more frequently at the beginning of the download, and will
	// taper off to MaxFileStatsInterval.
	FileStatsCallback func(FileStats, time.Duration)

	// MaxFileStatsInterval changes the maximum interval that the
	// FileStatsCallback will be called at.
	//
	// At the beginning of the download the interval is 100ms, but it will ramp up
	// to the provided maxInterval. If you specify a MaxFileStatsInterval
	// smaller than 100ms, there will be no ramp up, just a fixed interval at the
	// one you specify here.
	//
	// Default: 5 seconds
	MaxFileStatsInterval time.Duration

	// MaxConcurrentJobs is the number of parallel worker goroutines the
	// downloader will have.
	//
	// Default: 8
	MaxConcurrentJobs int

	// Cache is used to save/load isolated items to/from cache.
	Cache cache.Cache
}

// normalizePathSeparator returns path having os native path separator.
func normalizePathSeparator(p string) string {
	if filepath.Separator == '/' {
		return strings.Replace(p, `\`, string(filepath.Separator), -1)
	}
	return strings.Replace(p, "/", string(filepath.Separator), -1)
}

// New returns a Downloader instance, good to download one isolated.
//
// ctx will be used for logging and clock.
//
// The Client, hash and outputDir must be specified.
//
// If options is nil, this will use defaults as described in the Options struct.
func New(ctx context.Context, c *isolatedclient.Client, hash isolated.HexDigest,
	outputDir string, options *Options) *Downloader {

	var opt Options
	if options != nil {
		opt = *options
	}
	if opt.MaxConcurrentJobs == 0 {
		opt.MaxConcurrentJobs = 8
	}
	if opt.MaxFileStatsInterval == 0 {
		opt.MaxFileStatsInterval = time.Second * 5
	}

	interval := 100 * time.Millisecond
	if interval > opt.MaxFileStatsInterval {
		interval = opt.MaxFileStatsInterval
	}

	ctx2, cancel := context.WithCancel(ctx)
	ret := &Downloader{
		ctx:       ctx2,
		cancel:    cancel,
		c:         c,
		options:   opt,
		interval:  interval,
		dirCache:  stringset.New(0),
		rootHash:  hash,
		outputDir: normalizePathSeparator(outputDir),
		isoMap:    map[isolated.HexDigest]*isolated.Isolated{},
	}
	return ret
}

// Start begins downloading the isolated.
func (d *Downloader) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return
	}
	d.started = true

	d.pool = common.NewGoroutinePriorityPool(d.ctx, d.options.MaxConcurrentJobs)

	if err := d.ensureDir(d.outputDir); err != nil {
		d.addError(isolatedType, "<isolated setup>", err)
		d.cancel()
		return
	}

	// Start downloading the isolated tree in the work pool.
	d.scheduleIsolatedJob(d.rootHash)
}

// Wait waits for the completion of the download, and returns either `nil` if
// no errors occurred during the operation, or an `errors.MultiError` otherwise.
//
// This will Start() the Downloader, if it hasn't been started already.
//
// Calling this many times is safe (and will always return the same thing).
func (d *Downloader) Wait() error {
	d.Start()
	_ = d.pool.Wait()
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
	if d.options.FileCallback != nil {
		d.options.FileCallback(name, f)
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

		if d.options.FileStatsCallback != nil {
			doCall = maybeCall
			now := clock.Now(d.ctx)
			span = now.Sub(d.lastFileStatsCb)

			if span >= d.interval {
				if d.interval < d.options.MaxFileStatsInterval {
					d.interval *= 2
					if d.interval > d.options.MaxFileStatsInterval {
						d.interval = d.options.MaxFileStatsInterval
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
		d.options.FileStatsCallback(stats, span)
	}
}

func (d *Downloader) track(w io.Writer) io.Writer {
	if d.options.FileStatsCallback != nil {
		return &writeTracker{d, w}
	}
	return w
}

// ensureDir ensures that the directory dir exists.
func (d *Downloader) ensureDir(dir string) error {
	dir = normalizePathSeparator(dir)
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
	name = normalizePathSeparator(name)
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

		if d.options.Cache == nil {
			// no cache use case.
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
			return
		}

		err := d.options.Cache.Hardlink(details.Digest, filename, os.FileMode(mode))
		if err != nil {
			if !os.IsNotExist(err) {
				d.addError(fileType, name, errors.Annotate(err, "failed to link from cache").Err())
				return
			}
			// cache miss case
			pr, pw := io.Pipe()

			wg, ctx := errgroup.WithContext(d.ctx)
			wg.Go(func() error {
				err := d.c.Fetch(ctx, details.Digest, d.track(pw))
				if perr := pw.CloseWithError(err); perr != nil {
					return errors.Annotate(perr, "failed to close pipe writer").Err()
				}
				return err
			})

			wg.Go(func() error {
				err := d.options.Cache.Add(details.Digest, pr)
				if perr := pr.CloseWithError(err); perr != nil {
					return errors.Annotate(perr, "failed to close pipe reader").Err()
				}
				return err
			})

			if err = wg.Wait(); err != nil {
				d.addError(fileType, name, errors.Annotate(err, "failed to read from cache").Err())
				return
			}

			if err = d.options.Cache.Hardlink(details.Digest, filename, os.FileMode(mode)); err != nil {
				d.addError(fileType, name, errors.Annotate(err, "failed to link from cache").Err())
				return
			}
		}

		d.completeFile(name, details)
	}, func() {
		d.addError(fileType, name, d.ctx.Err())
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

			name := filepath.Clean(normalizePathSeparator(hdr.Name))
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
		d.addError(tarType, string(hash), d.ctx.Err())
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
		d.addError(isolatedType, string(hash), d.ctx.Err())
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

// Stats is stats for FetchAndMap
type Stats struct {
	Duration time.Duration `json:"duration"`

	ItemsCold string `json:"items_cold"`
	ItemsHot  string `json:"items_hot"`
}

func pack(array []int64) (string, error) {
	sort.Slice(array, func(i, j int) bool {
		return array[i] < array[j]
	})

	packed, err := isolated.Pack(array)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(packed), nil
}

// FetchAndMap fetches an isolated tree, create the tree and returns isolated tree.
func FetchAndMap(ctx context.Context, isolatedHash isolated.HexDigest, c *isolatedclient.Client, cache cache.Cache, outDir string) (map[string]*isolated.File, Stats, error) {
	start := time.Now()
	// TODO(tiktua): return IsolatedBundle
	isoMap := make(map[string]*isolated.File)
	d := New(ctx, c, isolatedHash, outDir, &Options{
		Cache: cache,
		FileCallback: func(name string, file *isolated.File) {
			isoMap[name] = file
		},
	})

	waitErr := d.Wait()
	// TODO(yyanagisawa): refactor this.
	added := cache.GetAdded()
	used := cache.GetUsed()

	itemsCold, err := pack(added)
	if err != nil {
		return nil, Stats{}, errors.Annotate(err, "failed to call Pack for cold items").Err()
	}

	hotCounter := make(map[int64]int)
	for _, v := range used {
		hotCounter[v]++
	}

	for _, v := range added {
		hotCounter[v]--
	}

	var hot []int64
	for k, v := range hotCounter {
		for i := 0; i < v; i++ {
			hot = append(hot, k)
		}
	}

	itemsHot, err := pack(hot)
	if err != nil {
		return nil, Stats{}, errors.Annotate(err, "failed to call Pack for hot items").Err()
	}

	return isoMap, Stats{
		Duration:  time.Now().Sub(start),
		ItemsCold: itemsCold,
		ItemsHot:  itemsHot,
	}, waitErr
}
