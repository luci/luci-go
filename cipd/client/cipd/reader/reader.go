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

// Package reader implements reading contents of a CIPD package.
package reader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/zip"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/ui"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// VerificationMode defines whether to verify hash or not.
type VerificationMode int

const (
	// VerifyHash instructs OpenPackage to calculate the hash of the package file
	// and compare it to the given InstanceID.
	VerifyHash VerificationMode = iota

	// SkipHashVerification instructs OpenPackage to skip the hash verification
	// and trust that the given InstanceID matches the package.
	SkipHashVerification

	// CalculateHash instructs OpenPackage to calculate the hash of the package
	// file using the given hash algo, and use it as a new instance ID.
	CalculateHash
)

func (v VerificationMode) String() string {
	switch v {
	case VerifyHash:
		return "VerifyHash"
	case SkipHashVerification:
		return "SkipHashVerification"
	case CalculateHash:
		return "CalculateHash"
	}
	return fmt.Sprintf("Unknown VerificationMode(%d)", int(v))
}

// ErrHashMismatch is an error when package hash doesn't match.
var ErrHashMismatch = cipderr.HashMismatch.Apply(
	errors.New("package hash mismatch"))

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
	// as an instance ID of the new pkg.Instance object. InstanceID is ignored
	// in this case and should be "".
	VerificationMode VerificationMode

	// InstanceID encodes the expected hash of the instance file.
	//
	// May be empty. See the comment for VerificationMode for more details.
	InstanceID string

	// HashAlgo specifies what hashing algorithm to use for computing instance ID.
	//
	// May be empty. See the comment for VerificationMode for more details.
	HashAlgo caspb.HashAlgo
}

// OpenInstance opens a package instance by reading it from the given source.
//
// The caller is responsible for closing the instance when done with it.
//
// On success it takes ownership of the source, closing it when the instance
// itself is closed. On errors the source is left open. It's a responsibility of
// the caller to close it in this case.
func OpenInstance(ctx context.Context, r pkg.Source, opts OpenInstanceOpts) (pkg.Instance, error) {
	out := &packageInstance{data: r}
	if err := out.open(opts); err != nil {
		return nil, err
	}
	return out, nil
}

// OpenInstanceFile opens a package instance by reading it from a file on disk.
//
// The caller is responsible for closing the instance when done with it. This
// will close the underlying file too.
func OpenInstanceFile(ctx context.Context, path string, opts OpenInstanceOpts) (pkg.Instance, error) {
	file, err := pkg.NewFileSource(path)
	if err != nil {
		return nil, cipderr.IO.Apply(errors.Fmt("opening instance file: %w", err))
	}
	inst, err := OpenInstance(ctx, file, opts)
	if err != nil {
		file.Close(ctx, false)
		return nil, err
	}
	return inst, nil
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
// placed into the destination. Note that it will *not* be included into
// 'extracted' slice.
//
// If withManifest is WithoutManifest, the function will fail if the manifest is
// among 'files' (as a precaution against unintended override of manifests).
func ExtractFiles(ctx context.Context, files []fs.File, dest fs.Destination, maxThreads int, withManifest pkg.ManifestMode, overrideInstallMode pkg.InstallMode) (extracted []pkg.FileInfo, err error) {
	if !withManifest {
		for _, f := range files {
			if f.Name() == pkg.ManifestName {
				return nil, cipderr.BadArgument.Apply(errors.New("refusing to extract the manifest, it is unexpected here"))
			}
		}
	}

	type fileToExtract struct {
		fs.File     // the actual file to extract
		index   int // its index in the `extracted` list
	}

	// recordExtracted writes into a correct slot in `extracted`.
	recordExtracted := func(f fileToExtract, symlink, hash string) {
		fi := pkg.FileInfo{
			Name:       f.Name(),
			Size:       f.Size(),
			Executable: f.Executable(),
			Writable:   f.Writable(),
			WinAttrs:   f.WinAttrs().String(),
			Symlink:    symlink,
			Hash:       hash,
		}
		if modTime := f.ModTime(); !modTime.IsZero() {
			fi.ModTime = modTime.Unix()
		}
		extracted[f.index] = fi
	}

	extractSymlinkFile := func(f fileToExtract) error {
		target, err := f.SymlinkTarget()
		if err != nil {
			return err
		}
		if err := dest.CreateSymlink(ctx, f.Name(), target); err != nil {
			return err
		}
		recordExtracted(f, target, "")
		return nil
	}

	extractRegularFile := func(f fileToExtract) (err error) {
		out, err := dest.CreateFile(ctx, f.Name(), fs.CreateFileOptions{
			Executable: f.Executable(),
			Writable:   f.Writable(),
			ModTime:    f.ModTime(),
			WinAttrs:   f.WinAttrs(),
		})
		if err != nil {
			return err
		}
		h := common.MustNewHash(common.DefaultHashAlgo)
		defer func() {
			if closeErr := out.Close(); err == nil {
				err = closeErr
			}
			if err == nil {
				recordExtracted(f, "", common.ObjectRefToInstanceID(common.ObjectRefFromHash(h)))
			}
		}()
		in, err := f.Open()
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(io.MultiWriter(out, h), in)
		return err
	}

	// Estimate of the work to do for the progress reporter.
	var progressTotalCount uint64
	var progressTotalSize uint64

	// Will be handled at the very end.
	var manifestFile fs.File

	filesToExtract := make([]fileToExtract, 0, len(files))
	for _, f := range files {
		// Extract everything except files under .cipdpkg dir (they are special CIPD
		// guts that are interpreted by CIPD itself and thus don't need to be
		// blindly extracted with everything else). Grab the manifest from them
		// though. It will be extended with []pkg.FileInfo of extracted files and
		// dropped to disk too, to represent the now unpacked package.
		name := f.Name()
		if name != pkg.ManifestName && strings.HasPrefix(name, pkg.ServiceDir+"/") {
			continue
		}

		// Will going to extract this file.
		progressTotalCount++
		progressTotalSize += f.Size()

		if name == pkg.ManifestName {
			// We delay writing the extended manifest until the very end because we
			// need to know hashes of all extract files (to put them inside the
			// manifest), so we need to extract them all first.
			manifestFile = f
		} else {
			// Extract through worker threads.
			filesToExtract = append(filesToExtract, fileToExtract{
				File:  f,
				index: len(filesToExtract), // to know what `extracted` slot to update
			})
		}
	}

	// `extracted` is updated by recordExtracted. Each item in `filesToExtract`
	// holds an index of a slot in `extracted` it corresponds. This is used to
	// preserve the original order of pkg.FileInfo entries even after we sort
	// `filesToExtract` below
	extracted = make([]pkg.FileInfo, len(filesToExtract))

	// Figure out how many worker threads to run. We assume `ExtractFiles` itself
	// is not called concurrently with other CPU-intensive operations.
	if maxThreads <= 0 {
		maxThreads = runtime.NumCPU()
	}
	workerCount := len(filesToExtract)
	if workerCount > maxThreads {
		workerCount = maxThreads
	}

	// If using multiple threads, sort the files by size (descending) so that
	// large files are processed first. This helps distribute processing more
	// evenly between worker threads.
	if workerCount > 1 {
		sort.Slice(filesToExtract, func(i, j int) bool {
			return filesToExtract[i].Size() > filesToExtract[j].Size()
		})
	}

	// We now know how much work needs to be done.
	progress := newProgressReporter(ctx, progressTotalCount, progressTotalSize)

	// Spawn worker threads to do the CPU-intensive extraction in parallel.
	err = parallel.WorkPool(workerCount, func(tasks chan<- func() error) {
		for _, f := range filesToExtract {
			task := func() error {
				defer progress.advance(f.Size())
				if f.Symlink() {
					return extractSymlinkFile(f)
				}
				return extractRegularFile(f)
			}
			select {
			case tasks <- task:
			case <-ctx.Done():
				return
			}
		}
	})

	switch {
	case ctx.Err() != nil:
		return extracted, ctx.Err()
	case err != nil || bool(!withManifest):
		return extracted, err
	case manifestFile == nil:
		return extracted, cipderr.BadArgument.Apply(errors.
			Fmt("bad CIPD package: no %s file", pkg.ManifestName))
	}

	// Now grab the original manifest.json from inside the package and extend it
	// with information about extracted files collected above while we were
	// extracting them.
	manifest, err := readManifestFile(manifestFile)
	if err != nil {
		return extracted, err
	}
	manifest.Files = extracted
	if overrideInstallMode != "" {
		manifest.ActualInstallMode = overrideInstallMode
	} else {
		manifest.ActualInstallMode = manifest.InstallMode
	}

	// And place it into the destination.
	out, err := dest.CreateFile(ctx, pkg.ManifestName, fs.CreateFileOptions{})
	if err != nil {
		return extracted, err
	}
	defer func() {
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
	}()
	err = pkg.WriteManifest(&manifest, out)
	progress.advance(manifestFile.Size())
	return extracted, err
}

// ExtractFilesTxn is like ExtractFiles, but it also opens and closes
// the transaction over fs.TransactionalDestination object.
//
// It guarantees that if extraction fails for some reason, there'll be no
// garbage laying around.
func ExtractFilesTxn(ctx context.Context, files []fs.File, dest fs.TransactionalDestination, maxThreads int, withManifest pkg.ManifestMode, overrideInstallMode pkg.InstallMode) (extracted []pkg.FileInfo, err error) {
	if err := dest.Begin(ctx); err != nil {
		return nil, err
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

	return ExtractFiles(ctx, files, dest, maxThreads, withManifest, overrideInstallMode)
}

// progressReporter logs progress of the extraction.
//
// Can be shared by multiple goroutines.
type progressReporter struct {
	ctx        context.Context
	activity   ui.Activity
	totalCount uint64 // total number of files to extract
	totalSize  uint64 // total expected uncompressed size of files
	formatStr  string // a format string for the title with proper padding

	m              sync.Mutex
	extractedCount uint64 // number of files extract so far
	extractedSize  uint64 // bytes uncompressed so far
}

func newProgressReporter(ctx context.Context, totalCount, totalSize uint64) *progressReporter {
	r := &progressReporter{
		ctx:        ctx,
		activity:   ui.CurrentActivity(ctx),
		totalCount: totalCount,
		totalSize:  totalSize,
	}

	// Construct a string like "Extracting (%5d files left)".
	pad := len(fmt.Sprintf("%d", r.totalCount))
	r.formatStr = fmt.Sprintf("Extracting (%%%dd files left)", pad)

	if r.totalCount != 0 {
		r.activity.Progress(ctx,
			fmt.Sprintf(r.formatStr, r.totalCount),
			ui.UnitBytes, 0, int64(r.totalSize),
		)
	}

	return r
}

// advance moves the progress indicator, occasionally logging it.
func (r *progressReporter) advance(size uint64) {
	if r.totalCount == 0 {
		return
	}

	r.m.Lock()
	defer r.m.Unlock()

	r.extractedSize += size
	r.extractedCount++

	// Need to report activity under the lock since otherwise the activity
	// progress tracker may see occasional "roll backs" of the progress if two
	// `advance` calls are racing.
	r.activity.Progress(r.ctx,
		fmt.Sprintf(r.formatStr, r.totalCount-r.extractedCount),
		ui.UnitBytes, int64(r.extractedSize), int64(r.totalSize),
	)
}

////////////////////////////////////////////////////////////////////////////////
// pkg.Instance implementation.

type packageInstance struct {
	data       pkg.Source
	instanceID string
	zip        *zip.Reader
	files      []fs.File
	manifest   pkg.Manifest
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
			return cipderr.BadArgument.Apply(errors.New("InstanceID is required with VerifyHash and SkipHashVerification modes"))
		case opts.HashAlgo != caspb.HashAlgo_HASH_ALGO_UNSPECIFIED:
			return cipderr.BadArgument.Apply(errors.New("HashAlgo must not be used with VerifyHash or SkipHashVerification modes"))
		}
	case CalculateHash:
		switch {
		case opts.InstanceID != "":
			return cipderr.BadArgument.Apply(errors.New("InstanceID must not be used with CalculateHash mode"))
		case opts.HashAlgo == caspb.HashAlgo_HASH_ALGO_UNSPECIFIED:
			return cipderr.BadArgument.Apply(errors.New("HashAlgo is required with CalculateHash mode"))
		}
	default:
		return cipderr.BadArgument.Apply(errors.
			Fmt("invalid verification mode %q", opts.VerificationMode))
	}

	// Assert instanceID is well-formated and uses a hash known to us, if given.
	// This is important for SkipHashVerification mode, where the user can pass
	// whatever, and for VerifyHash that parses the instance ID.
	if opts.InstanceID != "" {
		if err := common.ValidateInstanceID(opts.InstanceID, common.KnownHash); err != nil {
			return err
		}
	}

	switch opts.VerificationMode {
	case CalculateHash:
		h, err := common.NewHash(opts.HashAlgo)
		if err != nil {
			return err
		}
		if err := calculateHash(inst.data, h); err != nil {
			return err
		}
		inst.instanceID = common.ObjectRefToInstanceID(&caspb.ObjectRef{
			HashAlgo:  opts.HashAlgo,
			HexDigest: common.HexDigest(h),
		})

	case VerifyHash:
		obj := common.InstanceIDToObjectRef(opts.InstanceID)
		h := common.MustNewHash(obj.HashAlgo)
		if err := calculateHash(inst.data, h); err != nil {
			return err
		}
		if common.HexDigest(h) != obj.HexDigest {
			return ErrHashMismatch
		}
		inst.instanceID = opts.InstanceID // validated to match the data!

	case SkipHashVerification:
		inst.instanceID = opts.InstanceID // just trust
	}

	// List files and package manifest.
	var err error
	if inst.zip, err = zip.NewReader(inst.data, inst.data.Size()); err != nil {
		return cipderr.IO.Apply(errors.Fmt("reading instance file zip header: %w", err))
	}
	inst.files = make([]fs.File, len(inst.zip.File))
	for i, zf := range inst.zip.File {
		fiz := &fileInZip{z: zf}
		if fiz.Name() == pkg.ManifestName {
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
				return cipderr.BadArgument.Apply(errors.Fmt("file %s is not a fileInZip type", f.Name()))
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
		vf, err := makeVersionFile(inst.manifest.VersionFile, pkg.VersionFile{
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

func (inst *packageInstance) Files() []fs.File   { return inst.files }
func (inst *packageInstance) Source() pkg.Source { return inst.data }
func (inst *packageInstance) Close(ctx context.Context, corrupt bool) error {
	return inst.data.Close(ctx, corrupt)
}

// IsCorruptionError returns true iff err indicates corruption.
func IsCorruptionError(err error) bool {
	return errors.Any(err, func(err error) bool {
		switch err {
		case io.ErrUnexpectedEOF, zip.ErrFormat, zip.ErrChecksum, zip.ErrAlgorithm, ErrHashMismatch:
			return true
		default:
			_, flateCorrupt := err.(flate.CorruptInputError)
			return flateCorrupt
		}
	})
}

////////////////////////////////////////////////////////////////////////////////
// Utilities.

// calculateHash reads the entire file, passing it through the digester.
func calculateHash(r pkg.Source, h hash.Hash) error {
	_, err := io.Copy(h, io.NewSectionReader(r, 0, r.Size()))
	if err != nil {
		return cipderr.IO.Apply(errors.Fmt("calculating instance file hash: %w", err))
	}
	return nil
}

// readManifestFile decodes manifest file zipped inside the package.
func readManifestFile(f fs.File) (pkg.Manifest, error) {
	r, err := f.Open()
	if err != nil {
		return pkg.Manifest{}, cipderr.IO.Apply(errors.Fmt("opening manifest file: %w", err))
	}
	defer func() { _ = r.Close() }()
	return pkg.ReadManifest(r)
}

// makeVersionFile returns File representing a JSON blob with info about package
// version. It's what's deployed at path specified in 'version_file' stanza in
// package definition YAML.
func makeVersionFile(relPath string, versionFile pkg.VersionFile) (fs.File, error) {
	if !fs.IsCleanSlashPath(relPath) {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("invalid version_file: %s", relPath))
	}
	blob, err := json.MarshalIndent(versionFile, "", "  ")
	if err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("bad version file: %w", err))
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
	return io.NopCloser(bytes.NewReader(b.blob)), nil
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
		return cipderr.IO.Apply(errors.Fmt("prefetching %q: %w", f.z.Name, err))
	}
	defer r.Close()
	f.body, err = io.ReadAll(r)
	if err != nil {
		return cipderr.IO.Apply(errors.Fmt("prefetching %q: %w", f.z.Name, err))
	}
	return nil
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
		return "", cipderr.IO.Apply(errors.Fmt("%q: not a symlink", f.Name()))
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
		return nil, cipderr.IO.Apply(errors.Fmt("%q: opening a symlink is not allowed", f.Name()))
	}
	if f.body != nil {
		return io.NopCloser(bytes.NewReader(f.body)), nil
	}
	r, err := f.z.Open()
	if err != nil {
		return nil, cipderr.IO.Apply(errors.Fmt("opening %q for extraction: %w", f.Name(), err))
	}
	return r, nil
}
