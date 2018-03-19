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
	"compress/flate"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
)

// BuildInstanceOptions defines options for BuildInstance function.
type BuildInstanceOptions struct {
	// Input is a list of files to add to the package.
	Input []File

	// Output is where to write the package file to.
	Output io.Writer

	// PackageName is name of the package being built, e.g. 'infra/tools/cipd'.
	PackageName string

	// VersionFile is slash separated path where to drop JSON with version info.
	VersionFile string

	// InstallMode defines how to install the package: "copy" or "symlink".
	InstallMode InstallMode

	// CompressionLevel defines deflate compression level in range [0-9].
	CompressionLevel int

	// overrideFormatVersion, if set, will override the default format version put
	// into the manifest file.
	//
	// This is useful for testing.
	overrideFormatVersion string
}

// BuildInstance builds a new package instance.
//
// If build an instance of package named opts.PackageName by archiving input
// files (passed via opts.Input).
//
// The final binary is written to opts.Output. Some output may be written even
// if BuildInstance eventually returns an error.
func BuildInstance(ctx context.Context, opts BuildInstanceOptions) error {
	err := common.ValidatePackageName(opts.PackageName)
	if err != nil {
		return err
	}

	// Sanitize the Inputs.
	for _, f := range opts.Input {
		// Make sure no files are written to package service directory.
		if strings.HasPrefix(f.Name(), packageServiceDir+"/") {
			return fmt.Errorf("can't write to %s: %s", packageServiceDir, f.Name())
		}
		// Make sure no files are written to cipd's internal state directory.
		if strings.HasPrefix(f.Name(), SiteServiceDir+"/") {
			return fmt.Errorf("can't write to %s: %s", SiteServiceDir, f.Name())
		}
	}

	// Generate the manifest file, add to the list of input files.
	manifestFile, err := makeManifestFile(opts)
	if err != nil {
		return err
	}
	files := append(opts.Input, manifestFile)

	// Make sure filenames are unique.
	seenNames := make(map[string]struct{}, len(files))
	for _, f := range files {
		_, seen := seenNames[f.Name()]
		if seen {
			return fmt.Errorf("file %s is provided twice", f.Name())
		}
		seenNames[f.Name()] = struct{}{}
	}

	// Write the final zip file.
	return zipInputFiles(ctx, files, opts.Output, opts.CompressionLevel)
}

// zipInputFiles deterministically builds a zip archive out of input files and
// writes it to the writer. Files are written in the order given.
func zipInputFiles(ctx context.Context, files []File, w io.Writer, level int) error {
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
			return err
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

func zipRegularFile(dst io.Writer, f File) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	written, err := io.Copy(dst, src)
	if err != nil {
		return err
	}
	if uint64(written) != f.Size() {
		return fmt.Errorf("file %s changed midway", f.Name())
	}
	return nil
}

func zipSymlinkFile(dst io.Writer, f File) error {
	target, err := f.SymlinkTarget()
	if err != nil {
		return err
	}
	// Symlinks are zipped as text files with target path. os.ModeSymlink bit in
	// the header distinguishes them from regular files.
	_, err = dst.Write([]byte(target))
	return err
}

////////////////////////////////////////////////////////////////////////////////

// isLikelyAlreadyCompressed returns true for file format that use compression.
//
// It decides based only on the file name extension. For files that have an
// optional compression, we assume it is enabled.
func isLikelyAlreadyCompressed(f File) bool {
	// TODO(vadimsh): We can sniff MIME type based on the content, e.g via
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

func (m *manifestFile) Name() string       { return manifestName }
func (m *manifestFile) Size() uint64       { return uint64(len(*m)) }
func (m *manifestFile) Executable() bool   { return false }
func (m *manifestFile) Writable() bool     { return false }
func (m *manifestFile) ModTime() time.Time { return time.Time{} }
func (m *manifestFile) Symlink() bool      { return false }
func (m *manifestFile) WinAttrs() WinAttrs { return 0 }

func (m *manifestFile) SymlinkTarget() (string, error) {
	return "", fmt.Errorf("not a symlink: %s", m.Name())
}

func (m *manifestFile) Open() (io.ReadCloser, error) {
	return ioutil.NopCloser(bytes.NewReader(*m)), nil
}

// makeManifestFile generates a package manifest file and returns it as
// File interface.
func makeManifestFile(opts BuildInstanceOptions) (File, error) {
	if opts.VersionFile != "" && !isCleanSlashPath(opts.VersionFile) {
		return nil, fmt.Errorf("version file path should be a clean path relative to a package root: %s", opts.VersionFile)
	}
	if err := ValidateInstallMode(opts.InstallMode); err != nil {
		return nil, err
	}
	formatVer := manifestFormatVersion
	if opts.overrideFormatVersion != "" {
		formatVer = opts.overrideFormatVersion
	}
	buf := &bytes.Buffer{}
	err := writeManifest(&Manifest{
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
