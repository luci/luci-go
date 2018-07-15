// Copyright 2014 The LUCI Authors.
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

package local

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// VerificationMode defines whether to verify hash or not.
type VerificationMode int

const (
	// VerifyHash instructs OpenPackage to calculate hash of the package and
	// compare it to the given instanceID.
	VerifyHash VerificationMode = 0

	// SkipHashVerification instructs OpenPackage to skip the hash verification
	// and trust that the given instanceID matches the package.
	SkipHashVerification VerificationMode = 1
)

// ErrHashMismatch is an error when package hash doesn't match.
var ErrHashMismatch = errors.New("package hash mismatch")

// PackageInstance represents a binary CIPD package file (with manifest inside).
type PackageInstance interface {
	// Pin identifies package name and concreted instance ID of this package file.
	Pin() common.Pin

	// Files returns a list of files to deploy with the package.
	Files() []File

	// DataReader returns reader that reads raw package data.
	DataReader() io.ReadSeeker
}

// InstanceFile is an underlying data file for a PackageInstance.
type InstanceFile interface {
	io.ReadSeeker

	// Close is a bit non-standard, and can be used to indicate to the storage
	// (filesystem and/or cache) layer that this instance is actually bad. The
	// storage layer can then evict/revoke, etc. the bad file.
	Close(ctx context.Context, corrupt bool) error
}

// OpenInstance prepares the package for extraction.
//
// If instanceID is an empty string, OpenInstance will calculate the hash
// of the package and use it as InstanceID (regardless of verification mode).
//
// If instanceID is not empty and verification mode is VerifyHash,
// OpenInstance will check that package data matches the given instanceID. It
// skips this check if verification mode is SkipHashVerification.
func OpenInstance(ctx context.Context, r InstanceFile, instanceID string, v VerificationMode) (PackageInstance, error) {
	out := &packageInstance{data: r}
	if err := out.open(instanceID, v); err != nil {
		return nil, err
	}
	return out, nil
}

type dummyInstance struct {
	*os.File
}

func (d dummyInstance) Close(context.Context, bool) error { return d.File.Close() }

// OpenInstanceFile opens a package instance file on disk.
//
// The caller of this function must call closer() if err != nil to close the
// underlying file.
func OpenInstanceFile(ctx context.Context, path string, instanceID string, v VerificationMode) (inst PackageInstance, closer func() error, err error) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	inst, err = OpenInstance(ctx, dummyInstance{file}, instanceID, v)
	if err != nil {
		inst = nil
		file.Close()
	} else {
		closer = file.Close
	}
	return
}

// ExtractFiles extracts all given files into a destination, with a progress
// report.
func ExtractFiles(ctx context.Context, files []File, dest Destination) error {
	progress := newProgressReporter(ctx, files)

	extractManifestFile := func(f File) (err error) {
		defer progress.advance(f)
		manifest, err := readManifestFile(f)
		if err != nil {
			return err
		}
		manifest.Files = make([]FileInfo, 0, len(files))
		for _, file := range files {
			// Do not put info about service .cipdpkg files into the manifest,
			// otherwise it becomes recursive and "size" property of manifest file
			// itself is not correct.
			if strings.HasPrefix(file.Name(), packageServiceDir+"/") {
				continue
			}
			fi := FileInfo{
				Name:       file.Name(),
				Size:       file.Size(),
				Executable: file.Executable(),
				Writable:   file.Writable(),
				WinAttrs:   file.WinAttrs().String(),
			}
			if modTime := file.ModTime(); !modTime.IsZero() {
				fi.ModTime = modTime.Unix()
			}
			if file.Symlink() {
				target, err := file.SymlinkTarget()
				if err != nil {
					return err
				}
				fi.Symlink = target
			}
			manifest.Files = append(manifest.Files, fi)
		}
		out, err := dest.CreateFile(ctx, f.Name(), CreateFileOptions{})
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := out.Close(); err == nil {
				err = closeErr
			}
		}()
		return writeManifest(&manifest, out)
	}

	extractSymlinkFile := func(f File) error {
		defer progress.advance(f)
		target, err := f.SymlinkTarget()
		if err != nil {
			return err
		}
		return dest.CreateSymlink(ctx, f.Name(), target)
	}

	extractRegularFile := func(f File) (err error) {
		defer progress.advance(f)
		out, err := dest.CreateFile(ctx, f.Name(), CreateFileOptions{
			Executable: f.Executable(),
			Writable:   f.Writable(),
			ModTime:    f.ModTime(),
			WinAttrs:   f.WinAttrs(),
		})
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := out.Close(); err == nil {
				err = closeErr
			}
		}()
		in, err := f.Open()
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(out, in)
		return err
	}

	var manifest File
	var err error
	for _, f := range files {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			break

		default:
			switch {
			case f.Name() == manifestName:
				// We delay writing the extended manifest until the very end because it
				// contains values of 'SymlinkTarget' fields of all extracted files.
				// Fetching 'SymlinkTarget' in general involves seeking inside the zip,
				// and we prefer not to do that now. Upon exit from the loop, all
				// 'SymlinkTarget' values will be already cached in memory, and writing
				// the manifest will be cheaper.
				manifest = f
			case f.Symlink():
				err = extractSymlinkFile(f)
			default:
				err = extractRegularFile(f)
			}
		}
		if err != nil {
			break
		}
	}

	if err != nil {
		return err
	}

	// Finally extract the extended manifest, now that we have read (and cached)
	// all 'SymlinkTarget' values.
	if manifest == nil {
		return fmt.Errorf("no %s file, this is bad", manifestName)
	}
	return extractManifestFile(manifest)
}

// ExtractFilesTxn is like ExtractFiles, but it also opens and closes
// the transaction over TransactionalDestination object.
//
// It guarantees that if extraction fails for some reason, there'll be no
// garbage layout around.
func ExtractFilesTxn(ctx context.Context, files []File, dest TransactionalDestination) (err error) {
	if err := dest.Begin(ctx); err != nil {
		return err
	}

	// Cleanup the garbage even on panics.
	defer func() {
		endErr := dest.End(ctx, err == nil)
		if err == nil {
			err = endErr
		} else if endErr != nil {
			// Log endErr, but return the original error from ExtractFiles.
			logging.Warningf(ctx, "Failed to cleanup the destination after failed extraction - %s", endErr)
		}
	}()

	return ExtractFiles(ctx, files, dest)
}

// progressReporter periodically logs progress of the extraction.
//
// Can be shared by multiple goroutines.
type progressReporter struct {
	sync.Mutex

	ctx context.Context

	totalCount     uint64    // total number of files to extract
	totalSize      uint64    // total expected uncompressed size of files
	extractedCount uint64    // number of files extract so far
	extractedSize  uint64    // bytes uncompressed so far
	prevReport     time.Time // time when we did the last progress report
}

func newProgressReporter(ctx context.Context, files []File) *progressReporter {
	r := &progressReporter{ctx: ctx, totalCount: uint64(len(files))}
	for _, f := range files {
		if !f.Symlink() {
			r.totalSize += f.Size()
		}
	}
	if r.totalCount != 0 {
		logging.Infof(
			r.ctx, "cipd: about to extract %.1f MB (%d files)",
			float64(r.totalSize)/1000.0/1000.0, r.totalCount)
	}
	return r
}

// advance moves the progress indicator, occasionally logging it.
func (r *progressReporter) advance(f File) {
	if r.totalCount == 0 {
		return
	}

	now := clock.Now(r.ctx)
	reportNow := false
	progress := 0

	// We don't count size of the symlinks toward total.
	var size uint64
	if !f.Symlink() {
		size = f.Size()
	}

	// Report progress on first and last 'advance' calls and each 2 sec.
	r.Lock()
	r.extractedSize += size
	r.extractedCount++
	if r.extractedCount == 1 || r.extractedCount == r.totalCount || now.Sub(r.prevReport) > 2*time.Second {
		reportNow = true
		if r.totalSize != 0 {
			progress = int(float64(r.extractedSize) * 100 / float64(r.totalSize))
		} else {
			progress = int(float64(r.extractedCount) * 100 / float64(r.totalCount))
		}
		r.prevReport = now
	}
	r.Unlock()

	if reportNow {
		logging.Infof(r.ctx, "cipd: extracting - %d%%", progress)
	}
}

////////////////////////////////////////////////////////////////////////////////
// PackageInstance implementation.

type packageInstance struct {
	data       InstanceFile
	instanceID string
	zip        *zip.Reader
	files      []File
	manifest   Manifest
}

// open reads the package data, verifies SHA1 hash and reads manifest.
//
// It doesn't check for corruption, but the caller must do so.
func (inst *packageInstance) open(instanceID string, v VerificationMode) error {
	var dataSize int64
	var err error

	switch {
	case instanceID == "":
		// Calculate the default hash and use it as instance ID, regardless of
		// the verification mode.
		h := DefaultHash()
		dataSize, err = getHashAndSize(inst.data, h)
		if err != nil {
			return err
		}
		instanceID = InstanceIDFromHash(h)

	case v == VerifyHash:
		var h hash.Hash
		h, err = HashForInstanceID(instanceID)
		if err != nil {
			return err
		}
		dataSize, err = getHashAndSize(inst.data, h)
		if err != nil {
			return err
		}
		if InstanceIDFromHash(h) != instanceID {
			return ErrHashMismatch
		}

	case v == SkipHashVerification:
		dataSize, err = inst.data.Seek(0, os.SEEK_END)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid verification mode %q", v)
	}

	// Assert it is well-formated. This is important for SkipHashVerification
	// mode, where the user can pass whatever.
	if err = common.ValidateInstanceID(instanceID); err != nil {
		return err
	}
	inst.instanceID = instanceID

	// Zip reader needs an io.ReaderAt. Try to sniff it from our io.ReadSeeker
	// before falling back to a generic (potentially slower) implementation. This
	// works if inst.data is actually an os.File (which happens quite often).
	reader, ok := inst.data.(io.ReaderAt)
	if !ok {
		reader = &readerAt{r: inst.data}
	}

	// List files and package manifest.
	inst.zip, err = zip.NewReader(reader, dataSize)
	if err != nil {
		return err
	}
	inst.files = make([]File, len(inst.zip.File))
	for i, zf := range inst.zip.File {
		fiz := &fileInZip{z: zf}
		if fiz.Name() == manifestName {
			// The manifest is later read again when extracting, keep it in memory.
			if err = fiz.prefetch(); err != nil {
				return err
			}
			if inst.manifest, err = readManifestFile(fiz); err != nil {
				return err
			}
		}
		inst.files[i] = fiz
	}

	// Version "1" (legacy format) used to set the writable mode bit (0200) in
	// zipped files, and then ignored it when unpacking. Newer versions respect
	// the writable mode bit. Strip it off for the version "1", to preserve
	// the old semantics.
	if inst.manifest.FormatVersion == "1" {
		for _, f := range inst.files {
			zf, ok := f.(*fileInZip)
			if !ok {
				return fmt.Errorf("file %s is not a fileInZip type", f.Name())
			}
			// SetMode overrides lower 16 bits of ExternalAttrs that we use to store
			// Windows specific attributes (such as "hidden" and "system file"). Carry
			// them over manually. Note that if we manually clear only upper 16
			// bit that correspond to +w Unix flag, we won't clear 'msdosReadOnly'
			// flag that is stored in lower 16 bits. SetMode does it for us. So we use
			// SetMode to deal with 'msdosReadOnly' correctly, and then reapply other
			// Windows attributes.
			winAttrs := zf.z.ExternalAttrs & uint32(WinAttrsAll)
			zf.z.SetMode(zf.z.Mode() &^ 0222)
			zf.z.ExternalAttrs |= winAttrs
		}
	}

	// Generate version_file if needed.
	if inst.manifest.VersionFile != "" {
		vf, err := makeVersionFile(inst.manifest.VersionFile, VersionFile{
			PackageName: inst.manifest.PackageName,
			InstanceID:  inst.instanceID,
		})
		if err != nil {
			return err
		}
		inst.files = append(inst.files, vf)
	}

	return nil
}

func (inst *packageInstance) Pin() common.Pin {
	return common.Pin{
		PackageName: inst.manifest.PackageName,
		InstanceID:  inst.instanceID,
	}
}

func (inst *packageInstance) Files() []File             { return inst.files }
func (inst *packageInstance) DataReader() io.ReadSeeker { return inst.data }

// IsCorruptionError returns true iff err indicates corruption.
func IsCorruptionError(err error) bool {
	switch err {
	case io.ErrUnexpectedEOF, zip.ErrFormat, zip.ErrChecksum, zip.ErrAlgorithm, ErrHashMismatch:
		return true
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////
// Utilities.

// getHashAndSize rereads the entire file, passing it through the digester.
//
// Returns file length.
func getHashAndSize(r io.ReadSeeker, h hash.Hash) (int64, error) {
	if _, err := r.Seek(0, os.SEEK_SET); err != nil {
		return 0, err
	}
	if _, err := io.Copy(h, r); err != nil {
		return 0, err
	}
	return r.Seek(0, os.SEEK_CUR)
}

// readManifestFile decodes manifest file zipped inside the package.
func readManifestFile(f File) (Manifest, error) {
	r, err := f.Open()
	if err != nil {
		return Manifest{}, err
	}
	defer r.Close()
	return readManifest(r)
}

// makeVersionFile returns File representing a JSON blob with info about package
// version. It's what's deployed at path specified in 'version_file' stanza in
// package definition YAML.
func makeVersionFile(relPath string, versionFile VersionFile) (File, error) {
	if !isCleanSlashPath(relPath) {
		return nil, fmt.Errorf("invalid version_file: %s", relPath)
	}
	blob, err := json.MarshalIndent(versionFile, "", "  ")
	if err != nil {
		return nil, err
	}
	return &blobFile{
		name: relPath,
		blob: blob,
	}, nil
}

// blobFile implements File on top of byte array with file data.
type blobFile struct {
	name string
	blob []byte
}

func (b *blobFile) Name() string                   { return b.name }
func (b *blobFile) Size() uint64                   { return uint64(len(b.blob)) }
func (b *blobFile) Executable() bool               { return false }
func (b *blobFile) Writable() bool                 { return false }
func (b *blobFile) ModTime() time.Time             { return time.Time{} }
func (b *blobFile) Symlink() bool                  { return false }
func (b *blobFile) SymlinkTarget() (string, error) { return "", nil }
func (b *blobFile) WinAttrs() WinAttrs             { return 0 }

func (b *blobFile) Open() (io.ReadCloser, error) {
	return ioutil.NopCloser(bytes.NewReader(b.blob)), nil
}

////////////////////////////////////////////////////////////////////////////////
// File interface implementation via zip.File.

type fileInZip struct {
	z    *zip.File
	body []byte // if not nil, uncompressed body of the file
}

// prefetch loads the body of file into memory to speed up later calls.
func (f *fileInZip) prefetch() error {
	if f.body != nil {
		return nil
	}
	r, err := f.z.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	f.body, err = ioutil.ReadAll(r)
	return err
}

func (f *fileInZip) Name() string  { return f.z.Name }
func (f *fileInZip) Symlink() bool { return (f.z.Mode() & os.ModeSymlink) != 0 }
func (f *fileInZip) WinAttrs() WinAttrs {
	return WinAttrs(f.z.ExternalAttrs) & WinAttrsAll
}

func (f *fileInZip) Executable() bool {
	if f.Symlink() {
		return false
	}
	return (f.z.Mode() & 0100) != 0
}

func (f *fileInZip) Writable() bool {
	return (f.z.Mode() & 0200) != 0
}

func (f *fileInZip) ModTime() time.Time {
	var t time.Time
	if f.z.ModifiedTime != 0 || f.z.ModifiedDate != 0 {
		t = f.z.ModTime()
	}
	return t
}

func (f *fileInZip) Size() uint64 {
	if f.Symlink() {
		return 0
	}
	return f.z.UncompressedSize64
}

func (f *fileInZip) SymlinkTarget() (string, error) {
	if !f.Symlink() {
		return "", fmt.Errorf("not a symlink: %s", f.Name())
	}
	// Symlink is small, read it once and keep in memory. This is important
	// because 'SymlinkTarget' method looks like metadata getter, callers
	// don't expect it to do any IO each time (e.g. seeking inside the zip file).
	if err := f.prefetch(); err != nil {
		return "", err
	}
	return string(f.body), nil
}

func (f *fileInZip) Open() (io.ReadCloser, error) {
	if f.Symlink() {
		return nil, fmt.Errorf("opening a symlink is not allowed: %s", f.Name())
	}
	if f.body != nil {
		return ioutil.NopCloser(bytes.NewReader(f.body)), nil
	}
	return f.z.Open()
}

////////////////////////////////////////////////////////////////////////////////
// ReaderAt implementation via ReadSeeker. Not concurrency safe, moves file
// pointer around without any locking. Works OK in the context of OpenInstance
// function though (where OpenInstance takes sole ownership of io.ReadSeeker).

type readerAt struct {
	r io.ReadSeeker
}

func (r *readerAt) ReadAt(data []byte, off int64) (int, error) {
	_, err := r.r.Seek(off, os.SEEK_SET)
	if err != nil {
		return 0, err
	}
	return r.r.Read(data)
}
