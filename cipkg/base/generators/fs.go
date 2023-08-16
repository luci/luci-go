// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generators

import (
	"archive/tar"
	"fmt"
	"hash"
	"io"
	"io/fs"
)

func getHashFromFile(name string, src fs.File, h hash.Hash) error {
	// Tar is used for calculating hash from files - including metadata - in a
	// simple way.
	tw := tar.NewWriter(h)
	defer tw.Close()
	return hashFile(name, src, tw)
}

func getHashFromFS(src fs.FS, h hash.Hash) error {
	// Tar is used for calculating hash from files - including metadata - in a
	// simple way.
	tw := tar.NewWriter(h)
	defer tw.Close()

	return fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		var info fs.FileInfo
		// Resolve symlink
		if d.Type() == fs.ModeSymlink {
			f, err := src.Open(name)
			if err != nil {
				return fmt.Errorf("failed to open file: %s: %w", name, err)
			}
			defer f.Close()
			if info, err = f.Stat(); err != nil {
				return fmt.Errorf("failed to stat file: %s: %w", name, err)
			}
		} else {
			if info, err = fs.Stat(src, name); err != nil {
				return fmt.Errorf("failed to stat file: %s: %w", name, err)
			}
		}

		switch info.Mode().Type() {
		case fs.ModeSymlink:
			return fmt.Errorf("unexpected symlink: %s", name)
		case fs.ModeDir:
			if err := tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     name,
				Mode:     int64(info.Mode()),
			}); err != nil {
				return fmt.Errorf("failed to write header: %s: %w", name, err)
			}
		default: // Regular File
			f, err := src.Open(name)
			if err != nil {
				return fmt.Errorf("failed to open file: %s: %w", name, err)
			}
			defer f.Close()
			if err := hashFile(name, f, tw); err != nil {
				return err
			}
		}
		return nil
	})
}

func hashFile(name string, src fs.File, tw *tar.Writer) error {
	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %s: %w", name, err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Mode:     int64(info.Mode()),
		Size:     info.Size(),
	}); err != nil {
		return fmt.Errorf("failed to write header: %s: %w", name, err)
	}
	if _, err := io.Copy(tw, src); err != nil {
		return fmt.Errorf("failed to write file: %s: %w", name, err)
	}
	return nil
}
