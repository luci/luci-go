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

package builder

import (
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/zip"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// Options defines options for BuildInstance function.
type Options struct {
	// Input is a list of files to add to the package.
	Input []fs.File

	// Output is where to write the package file to.
	Output io.Writer

	// PackageName is name of the package being built, e.g. 'infra/tools/cipd'.
	PackageName string

	// VersionFile is slash separated path where to drop JSON with version info.
	VersionFile string

	// InstallMode defines how to install the package: "copy" or "symlink".
	InstallMode pkg.InstallMode

	// CompressionLevel defines deflate compression level in range [0-9].
	CompressionLevel int

	// HashAlgo specifies what hashing algorithm to use for computing instance ID.
	//
	// By default it is common.DefaultHashAlgo.
	HashAlgo api.HashAlgo

	// OverrideFormatVersion, if set, will override the default format version put
	// into the manifest file.
	//
	// This is useful for testing. Should not be normally used by other code.
	OverrideFormatVersion string
}

// BuildInstance builds a new package instance.
//
// It builds an instance of a package named opts.PackageName by archiving input
// files (passed via opts.Input) and writing the final binary to opts.Output.
//
// On success returns a pin of the built package which can later be used to
// register the package on CIPD backend.
//
// Some output may be written even if BuildInstance eventually returns an error.
func BuildInstance(ctx context.Context, opts Options) (common.Pin, error) {
	err := common.ValidatePackageName(opts.PackageName)
	if err != nil {
		return common.Pin{}, err
	}

	// Make sure hash algo is supported.
	if opts.HashAlgo == 0 {
		opts.HashAlgo = common.DefaultHashAlgo
	}
	hash, err := common.NewHash(opts.HashAlgo)
	if err != nil {
		return common.Pin{}, err
	}

	// Sanitize the Inputs.
	for _, f := range opts.Input {
		// Make sure no files are written to package service directory.
		if strings.HasPrefix(f.Name(), pkg.ServiceDir+"/") {
			return common.Pin{}, cipderr.BadArgument.Apply(errors.Fmt("can't write to %s: %s", pkg.ServiceDir, f.Name()))
		}
		// Make sure no files are written to cipd's internal state directory.
		if strings.HasPrefix(f.Name(), fs.SiteServiceDir+"/") {
			return common.Pin{}, cipderr.BadArgument.Apply(errors.Fmt("can't write to %s: %s", fs.SiteServiceDir, f.Name()))
		}
	}

	// Generate the manifest file, add to the list of input files.
	manifestFile, err := makeManifestFile(opts)
	if err != nil {
		return common.Pin{}, err
	}
	files := append(opts.Input, manifestFile)

	// Make sure filenames are unique.
	seenNames := make(map[string]struct{}, len(files))
	for _, f := range files {
		_, seen := seenNames[f.Name()]
		if seen {
			return common.Pin{}, cipderr.BadArgument.Apply(errors.Fmt("file %s is provided twice", f.Name()))
		}
		seenNames[f.Name()] = struct{}{}
	}

	// Write the final zip file, calculate its hash to use for instance ID.
	if err := zipInputFiles(ctx, files, io.MultiWriter(opts.Output, hash), opts.CompressionLevel); err != nil {
		return common.Pin{}, err
	}
	return common.Pin{
		PackageName: opts.PackageName,
		InstanceID: common.ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  opts.HashAlgo,
			HexDigest: common.HexDigest(hash),
		}),
	}, nil
}

// zipInputFiles deterministically builds a zip archive out of input files and
// writes it to the writer. Files are written in the order given.
func zipInputFiles(ctx context.Context, files []fs.File, w io.Writer, level int) error {
	logging.Infof(ctx, "About to zip %d files with compression level %d", len(files), level)

	writer := zip.NewWriter(w)
	defer writer.Close()

	writer.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, level)
	})

	// Reports zipping progress to the log each second.
	lastReport := time.Time{}
	progress := func(count int) {
		if time.Since(lastReport) > time.Second {
			lastReport = time.Now()
			logging.Infof(ctx, "Zipping files: %d files left", len(files)-count)
		}
	}

	for i, in := range files {
		progress(i)

		// Bail out early if context is canceled.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Intentionally do not add file mode to make zip archive
		// deterministic. Timestamps sometimes need to be preserved, but normally
		// are zero valued. See also zip.FileInfoHeader() implementation.
		fh := zip.FileHeader{
			Name:   in.Name(),
			Method: zip.Deflate,
		}
		if level == 0 || in.Symlink() || isLikelyAlreadyCompressed(in) {
			fh.Method = zip.Store
		}

		mode := os.FileMode(0400)
		if in.Executable() {
			mode |= 0100
		}
		if in.Writable() {
			mode |= 0200
		}
		if in.Symlink() {
			mode |= os.ModeSymlink
		}
		fh.SetMode(mode)

		if !in.ModTime().IsZero() {
			fh.SetModTime(in.ModTime())
		}

		fh.ExternalAttrs |= uint32(in.WinAttrs())

		dst, err := writer.CreateHeader(&fh)
		if err != nil {
			return cipderr.IO.Apply(errors.Fmt("writing zip entry header: %w", err))
		}
		if in.Symlink() {
			err = zipSymlinkFile(dst, in)
		} else {
			err = zipRegularFile(dst, in)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func zipRegularFile(dst io.Writer, f fs.File) error {
	src, err := f.Open()
	if err != nil {
		return cipderr.IO.Apply(errors.Fmt("opening %q for zipping: %w", f.Name(), err))
	}
	defer src.Close()
	written, err := io.Copy(dst, src)
	if err != nil {
		return cipderr.IO.Apply(errors.Fmt("zipping %q: %w", f.Name(), err))
	}
	if uint64(written) != f.Size() {
		return cipderr.IO.Apply(errors.Fmt("file %q changed midway", f.Name()))
	}
	return nil
}

func zipSymlinkFile(dst io.Writer, f fs.File) error {
	target, err := f.SymlinkTarget()
	if err != nil {
		return cipderr.IO.Apply(errors.

			// Symlinks are zipped as text files with target path. os.ModeSymlink bit in
			// the header distinguishes them from regular files.
			Fmt("resolving symlink %q for zipping: %w", f.Name(), err))
	}

	if _, err = dst.Write([]byte(target)); err != nil {
		return cipderr.IO.Apply(errors.Fmt("zipping symlink %q: %w", f.Name(), err))
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

// isLikelyAlreadyCompressed returns true for file format that use compression.
//
// It decides based only on the file name extension. For files that have an
// optional compression, we assume it is enabled.
func isLikelyAlreadyCompressed(f fs.File) bool {
	// TODO(vadimsh): We can sniff MIME type based on the content, e.g. via
	// https://bitbucket.org/taruti/mimemagic/src or http.DetectContentType. Not
	// sure it's worth it.
	return compressedExt.Has(strings.ToLower(path.Ext(f.Name())))
}

var compressedExt = stringset.NewFromSlice(
	// Archives.
	".7z",
	".apk",
	".bz2",
	".cab",
	".dmg",
	".egg",
	".epub",
	".gz",
	".jar",
	".lz",
	".lzma",
	".lzo",
	".pea",
	".rar",
	".rz",
	".s7z",
	".tbz2",
	".tgz",
	".tlz",
	".war",
	".whl",
	".xpi",
	".xz",
	".z",
	".zip",
	".zipx",

	// Images with (possibly optional) compression, which we assume is enabled.
	".arw",
	".cr2",
	".dng",
	".gif",
	".jpeg",
	".jpg",
	".nef",
	".orf",
	".pef",
	".pgf",
	".png",
	".raf",
	".rw2",
	".srw",
	".tiff",
	".webp",

	// Containers with usually compressed video/audio.
	".aac",
	".alac",
	".avi",
	".flac",
	".gifv",
	".m4p",
	".m4v",
	".mka",
	".mkv",
	".mov",
	".mp3",
	".mp4",
	".mpg",
	".ogg",
	".qt",
	".vob",
	".webm",
	".wma",
	".wmv",
)

////////////////////////////////////////////////////////////////////////////////

type manifestFile []byte

func (m *manifestFile) Name() string          { return pkg.ManifestName }
func (m *manifestFile) Size() uint64          { return uint64(len(*m)) }
func (m *manifestFile) Executable() bool      { return false }
func (m *manifestFile) Writable() bool        { return false }
func (m *manifestFile) ModTime() time.Time    { return time.Time{} }
func (m *manifestFile) Symlink() bool         { return false }
func (m *manifestFile) WinAttrs() fs.WinAttrs { return 0 }

func (m *manifestFile) SymlinkTarget() (string, error) {
	return "", cipderr.IO.Apply(errors.Fmt("%q: not a symlink", m.Name()))
}

func (m *manifestFile) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(*m)), nil
}

// makeManifestFile generates a package manifest file and returns it as
// File interface.
func makeManifestFile(opts Options) (fs.File, error) {
	if opts.VersionFile != "" && !fs.IsCleanSlashPath(opts.VersionFile) {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("version file path should be a clean path relative to a package root: %s", opts.VersionFile))
	}
	if err := pkg.ValidateInstallMode(opts.InstallMode); err != nil {
		return nil, err
	}
	formatVer := pkg.ManifestFormatVersion
	if opts.OverrideFormatVersion != "" {
		formatVer = opts.OverrideFormatVersion
	}
	buf := &bytes.Buffer{}
	err := pkg.WriteManifest(&pkg.Manifest{
		FormatVersion: formatVer,
		PackageName:   opts.PackageName,
		VersionFile:   opts.VersionFile,
		InstallMode:   opts.InstallMode,
	}, buf)
	if err != nil {
		return nil, err
	}
	out := manifestFile(buf.Bytes())
	return &out, nil
}
