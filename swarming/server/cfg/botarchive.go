// Copyright 2024 The LUCI Authors.
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

package cfg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/klauspost/compress/zip"

	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/common/errors"
)

// botArchiveConfig is embedded into the bot archive as config/config.json.
type botArchiveConfig struct {
	// Server is Swarming server's URL e.g. "https://somehost.appspot.com".
	Server string `json:"server"`
	// BotCIPDInstanceID is CIPD instance ID of the base bot package.
	BotCIPDInstanceID string `json:"bot_cipd_instance_id"`

	// ServerVersion is deprecated and should not be used.
	ServerVersion string `json:"server_version"`
	// EnableTSMonitoring is deprecated and should not be used.
	EnableTSMonitoring bool `json:"enable_ts_monitoring"`
}

// buildBotArchive takes the base bot CIPD package, adds a couple of generated
// files to it, builds the resulting zip archive and calculates the SHA256 hash
// of the content.
//
// `botConfigPy`, if not nil, will be stored as `config/bot_config.py`.
// `configJSON` will be serialized and stored as `config/config.json`.
func buildBotArchive(base pkg.Instance, botConfigPy []byte, configJSON botArchiveConfig) (out []byte, digest string, err error) {
	// Get the base package files as a map. Skip internal CIPD package guts.
	files := base.Files()
	filesByName := make(map[string]archivedFile, len(files))
	for _, f := range files {
		if !strings.HasPrefix(f.Name(), ".cipdpkg/") {
			filesByName[f.Name()] = f
		}
	}

	// Override files in the base with the generated copies.
	if botConfigPy != nil {
		filesByName["config/bot_config.py"] = blobFile(botConfigPy)
	}
	configJSONBlob, err := json.MarshalIndent(&configJSON, "", "  ")
	if err != nil {
		return nil, "", errors.Annotate(err, "failed to marshal config.json").Err()
	}
	filesByName["config/config.json"] = blobFile(configJSONBlob)

	var outBytes bytes.Buffer
	zipWriter := zip.NewWriter(&outBytes)
	defer func() { _ = zipWriter.Close() }() // cleanup for early exits

	hasher := sha256.New()

	names := make([]string, 0, len(filesByName))
	for name := range filesByName {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		f := filesByName[name]
		expectedSize := f.Size()
		src, err := f.Open()
		if err != nil {
			return nil, "", errors.Annotate(err, "failed to open %q for reading", name).Err()
		}

		// This hashing logic should match what the bot code itself is doing when
		// calculating its own version (see generate_version in bot_main.py).
		//
		// Note that writes to a hasher are non-fallible.
		_, _ = fmt.Fprintf(hasher, "%d", len(name))
		_, _ = hasher.Write([]byte(name))
		_, _ = fmt.Fprintf(hasher, "%d", expectedSize)

		fh := zip.FileHeader{Name: name, Method: zip.Deflate}
		fh.SetMode(0400) // read only
		dst, err := zipWriter.CreateHeader(&fh)
		if err != nil {
			_ = src.Close()
			return nil, "", errors.Annotate(err, "writing zip entry header for %q", name).Err()
		}

		// This should write exactly `expectedSize` bytes into both the hasher and
		// the destination zip file. We'll double check.
		wrote, err := io.Copy(io.MultiWriter(hasher, dst), src)
		_ = src.Close()
		if err != nil {
			return nil, "", errors.Annotate(err, "failed to write %q", name).Err()
		}
		if uint64(wrote) != expectedSize {
			return nil, "", errors.Annotate(err, "when writing %q wrote %d bytes, while should have %d", name, wrote, expectedSize).Err()
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, "", errors.Annotate(err, "finalizing the zip archive").Err()
	}

	return outBytes.Bytes(), hex.EncodeToString(hasher.Sum(nil)), nil
}

// archivedFile is a subset of fs.File we use.
type archivedFile interface {
	Size() uint64
	Open() (io.ReadCloser, error)
}

// blobFile implements archivedFile on top of a []byte buffer.
type blobFile []byte

func (b blobFile) Size() uint64                 { return uint64(len(b)) }
func (b blobFile) Open() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }
