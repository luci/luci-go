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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"
)

// VerificationMode defines whether to verify hash or not.
type VerificationMode int

const (
	// VerifyHash instructs OpenPackage to calculate the hash of the package file
	// and compare it to the given InstanceID.
	VerifyHash VerificationMode = 0

	// SkipHashVerification instructs OpenPackage to skip the hash verification
	// and trust that the given InstanceID matches the package.
	SkipHashVerification VerificationMode = 1

	// CalculateHash instructs OpenPackage to calculate the hash of the package
	// file using the given hash algo, and use it as a new instance ID.
	CalculateHash VerificationMode = 2
)

// ErrHashMismatch is an error when package hash doesn't match.
var ErrHashMismatch = errors.New("package hash mismatch")

// PackageInstance represents a binary CIPD package file (with manifest inside).
type PackageInstance interface {
	// Pin identifies package name and concreted instance ID of this package file.
	Pin() common.Pin

	// Files returns a list of files to deploy with the package.
	Files() []fs.File

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

// OpenInstanceOpts is passed to OpenInstance and OpenInstanceFile.
type OpenInstanceOpts struct {
	// VerificationMode specifies what to do with the hash of the instance file.
	//
	// Passing VerifyHash instructs OpenPackage to calculate the hash of the
	// instance file using the hash algorithm matching InstanceID, and then
	// compare the resulting digest to InstanceID, failing on mismatch with
	// ErrHashMismatch. HashAlgo is ignored in this case and should be 0.
	//
	// Passing SkipHashVerification instructs OpenPackage to unconditionally
	// trust the given InstanceID. HashAlgo is also ignored in this case and
	// should be 0.
	//
	// Passing CalculateHash instructs OpenPackage to calculate the hash of the
	// instance file using the given HashAlgo, and use the resulting digest
	// as an instance ID of the new PackageInstance object. InstanceID is ignored
	// in this case and should be "".
	VerificationMode VerificationMode

	// InstanceID encodes the expected hash of the instance file.
	//
	// May be empty. See the comment for VerificationMode for more details.
	InstanceID string

	// HashAlgo specifies what hashing algorithm to use for computing instance ID.
	//
	// May be empty. See the comment for VerificationMode for more details.
	HashAlgo api.HashAlgo
}

// OpenInstance prepares the package for extraction.
func OpenInstance(ctx context.Context, r InstanceFile, opts OpenInstanceOpts) (PackageInstance, error) {
	out := &packageInstance{data: r}
	if err := out.open(opts); err != nil {
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
func OpenInstanceFile(ctx context.Context, path string, opts OpenInstanceOpts) (inst PackageInstance, closer func() error, err error) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	inst, err = OpenInstance(ctx, dummyInstance{file}, opts)
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
//
// This function has intimate understanding of specifics of CIPD packages, such
// as manifest files and .cipdpkg/* directory, thus it should be used only when
// extracting files from CIPD packages.
//
// If withManifest is WithManifest, the manifest file is required to be among
// 'files'. It will be extended with information about extracted files and
// placed into the destination.
//
// If withManifest is WithoutManifest, the function will fail if the manifest is
// among 'files' (as a precaution against unintended override of manifests).
func ExtractFiles(ctx context.Context, files []fs.File, dest fs.Destination, withManifest ManifestMode) error {
	if !withManifest {
		for _, f := range files {
			if f.Name() == ManifestName {
				return fmt.Errorf("refusing to extract the manifest, it is unexpected here")
			}
		}
	}

	progress := newProgressReporter(ctx, files)

	extractManifestFile := func(f fs.File) (err error) {
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
			if strings.HasPrefix(file.Name(), PackageServiceDir+"/") {
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
		out, err := dest.CreateFile(ctx, f.Name(), fs.CreateFileOptions{})
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

	extractSymlinkFile := func(f fs.File) error {
		defer progress.advance(f)
		target, err := f.SymlinkTarget()
		if err != nil {
			return err
		}
		return dest.CreateSymlink(ctx, f.Name(), target)
	}

	extractRegularFile := func(f fs.File) (err error) {
		defer progress.advance(f)
		out, err := dest.CreateFile(ctx, f.Name(), fs.CreateFileOptions{
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

	var manifest fs.File
	var err error
	for _, f := range files {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			break

		default:
			switch {
			case f.Name() == ManifestName:
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
	if withManifest {
		if manifest == nil {
			return fmt.Errorf("no %s file, this is bad", ManifestName)
		}
		return extractManifestFile(manifest)
	}

	return nil
}

// ExtractFilesTxn is like ExtractFiles, but it also opens and closes
// the transaction over fs.TransactionalDestination object.
//
// It guarantees that if extraction fails for some reason, there'll be no
// garbage laying around.
func ExtractFilesTxn(ctx context.Context, files []fs.File, dest fs.TransactionalDestination, withManifest ManifestMode) (err error) {
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

	return ExtractFiles(ctx, files, dest, withManifest)
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

func newProgressReporter(ctx context.Context, files []fs.File) *progressReporter {
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
func (r *progressReporter) advance(f fs.File) {
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
	files      []fs.File
	manifest   Manifest
}

// open reads the package data, verifies the hash and reads manifest.
//
// It doesn't check for corruption, but the caller must do so.
func (inst *packageInstance) open(opts OpenInstanceOpts) error {
	// Enforce consistency of opts, to avoid situations when caller thinks that
	// InstanceID is used, while in fact it is not.
	switch opts.VerificationMode {
	case VerifyHash, SkipHashVerification:
		switch {
		case opts.InstanceID == "":
			return fmt.Errorf("InstanceID is required with VerifyHash and SkipHashVerification modes")
		case opts.HashAlgo != api.HashAlgo_HASH_ALGO_UNSPECIFIED:
			return fmt.Errorf("HashAlgo must not be used with VerifyHash or SkipHashVerification modes")
		}
	case CalculateHash:
		switch {
		case opts.InstanceID != "":
			return fmt.Errorf("InstanceID must not be used with CalculateHash mode")
		case opts.HashAlgo == api.HashAlgo_HASH_ALGO_UNSPECIFIED:
			return fmt.Errorf("HashAlgo is required with CalculateHash mode")
		}
	default:
		return fmt.Errorf("invalid verification mode %q", opts.VerificationMode)
	}

	// Assert instanceID is well-formated and uses a hash known to us, if given.
	// This is important for SkipHashVerification mode, where the user can pass
	// whatever, and for VerifyHash that parses the instance ID.
	if opts.InstanceID != "" {
		if err := common.ValidateInstanceID(opts.InstanceID, common.KnownHash); err != nil {
			return err
		}
	}

	var dataSize int64
	var err error

	switch opts.VerificationMode {
	case CalculateHash:
		var h hash.Hash
		if h, err = common.NewHash(opts.HashAlgo); err != nil {
			return err
		}
		if dataSize, err = getHashAndSize(inst.data, h); err != nil {
			return err
		}
		inst.instanceID = common.ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  opts.HashAlgo,
			HexDigest: common.HexDigest(h),
		})

	case VerifyHash:
		obj := common.InstanceIDToObjectRef(opts.InstanceID)
		h := common.MustNewHash(obj.HashAlgo)
		if dataSize, err = getHashAndSize(inst.data, h); err != nil {
			return err
		}
		if common.HexDigest(h) != obj.HexDigest {
			return ErrHashMismatch
		}
		inst.instanceID = opts.InstanceID // validated to match the data!

	case SkipHashVerification:
		if dataSize, err = inst.data.Seek(0, os.SEEK_END); err != nil {
			return err
		}
		inst.instanceID = opts.InstanceID // just trust
	}

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
	inst.files = make([]fs.File, len(inst.zip.File))
	for i, zf := range inst.zip.File {
		fiz := &fileInZip{z: zf}
		if fiz.Name() == ManifestName {
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
			winAttrs := zf.z.ExternalAttrs & uint32(fs.WinAttrsAll)
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

func (inst *packageInstance) Files() []fs.File          { return inst.files }
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
func readManifestFile(f fs.File) (Manifest, error) {
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
func makeVersionFile(relPath string, versionFile VersionFile) (fs.File, error) {
	if !fs.IsCleanSlashPath(relPath) {
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

// blobFile implements fs.File on top of byte array with file data.
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
func (b *blobFile) WinAttrs() fs.WinAttrs          { return 0 }

func (b *blobFile) Open() (io.ReadCloser, error) {
	return ioutil.NopCloser(bytes.NewReader(b.blob)), nil
}

////////////////////////////////////////////////////////////////////////////////
// fs.File interface implementation via zip.File.

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
func (f *fileInZip) WinAttrs() fs.WinAttrs {
	return fs.WinAttrs(f.z.ExternalAttrs) & fs.WinAttrsAll
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
