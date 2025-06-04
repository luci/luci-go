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
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zip"

	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/swarming/proto/config"
)

const (
	// How long to keep unused bot archives in the datastore.
	botArchiveExpiry = 24 * time.Hour
	// How often to bump expiry time of in-use bot archives.
	botArchiveTouchPeriod = time.Hour
	// Maximum size of a single chunk of a bot archive in the datastore.
	botArchiveChunkSize = 300 * 1024
)

// botArchiverState contains the state of the process that builds bot archives.
//
// It is used only by ensureBotArchiveBuilt(...) implementation and is its
// private detail.
//
// Normally there are one or two actively used bot archives (canary and stable),
// but we want to keep more of them in the datastore to avoid dealing with
// race conditions when cleaning archives that might potentially still be in
// use by some lagging GAE process.
//
// Either way there should be relatively few archives and all metadata about
// them can easily fit in a single entity.
type botArchiverState struct {
	_ datastore.PropertyMap `gae:"-,extra"`

	// Key is botArchiverStateKey(...).
	Key *datastore.Key `gae:"$key"`
	// Archives describes all recently built bot archives.
	Archives []botArchive `gae:",noindex"`
}

// botArchiverStateKey is a key of the singleton botArchiverState entity.
func botArchiverStateKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "BotArchiverState", "", 1, nil)
}

// findByInputs finds an existing archive with inputs matching b's.
func (s *botArchiverState) findByInputs(b *botArchive) *botArchive {
	for i := range s.Archives {
		if s.Archives[i].sameInputs(b) {
			return &s.Archives[i]
		}
	}
	return nil
}

// botArchive describes inputs to a bot archive builder and what output it
// produced (i.e. the bot archive digest and its chunks in the datastore).
type botArchive struct {
	// Inputs.

	// PackageInstanceID is the CIPD package instance ID with the base bot files.
	PackageInstanceID string `gae:",noindex"`
	// BotConfigHash is SHA256 of the bot_config.py embedded into the bot archive.
	BotConfigHash string `gae:",noindex"`
	// EmbeddedBotSettingsHash is a digest of EmbeddedBotSettings used.
	EmbeddedBotSettingsHash string `gae:",noindex"`

	// Outputs.

	// Digest is the bot archive SHA256 digest aka "bot archive version".
	Digest string `gae:",noindex"`

	// Chunks is the list of BotArchiveChunk entities (entries separated by '\n').
	//
	// Need to store them as a single string due to limits on support of repeated
	// fields in datastore entities: botArchive is already stored in a repeated
	// field and can't have repeated fields of its own.
	Chunks string `gae:",noindex"`

	// Metadata.

	// BotConfigRev is the revision of bot_config.py script used when building.
	//
	// This is the first observed revision for the given BotConfigHash.
	BotConfigRev string `gae:",noindex"`

	// LastTouched is when this archive was touched by ensureBotArchiveBuilt.
	//
	// Used to decide when it is OK to delete unused entries. Note that
	// ensureBotArchiveBuilt is called periodically by a cron job, ensuring that
	// all active entries are periodically "touched".
	LastTouched time.Time `gae:",noindex"`
}

// sameInputs is true if the input fields of a and b are the same.
func (a *botArchive) sameInputs(b *botArchive) bool {
	return a.PackageInstanceID == b.PackageInstanceID &&
		a.BotConfigHash == b.BotConfigHash &&
		a.EmbeddedBotSettingsHash == b.EmbeddedBotSettingsHash
}

// botArchiveChunk is a chunk of an actual zip archive.
//
// We split the archive in chunks since there's a 1 MB limit on a datastore
// entity size. The bot archive size (in 2024) is ~1.5 MB, so we'll have 2-3
// chunks overall.
//
// Different bot archives do not share chunks, i.e. it is always safe to delete
// all bot archive's chunks when expiring this bot archive.
type botArchiveChunk struct {
	// Key is botArchiveChunkKey(...).
	Key *datastore.Key `gae:"$key"`
	// Data is the actual chunk data.
	Data []byte `gae:",noindex"`
}

// botArchiveChunkKey is a key of a concrete chunk of a bot archive.
func botArchiveChunkKey(ctx context.Context, id string) *datastore.Key {
	return datastore.NewKey(ctx, "BotArchiveChunk", id, 0, botArchiverStateKey(ctx))
}

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

// ensureBotArchiveBuilt does everything to build a bot archive and store it
// in the datastore if it wasn't built before.
func ensureBotArchiveBuilt(
	ctx context.Context,
	cipd CIPD,
	desc *configpb.BotDeployment_BotPackage,
	botConfigPy []byte,
	botConfigPyRev string,
	ebs *EmbeddedBotSettings,
	chunkSize int,
) (BotArchiveInfo, error) {
	// Resolve all inputs and prepare the botArchive we are trying to build.
	var building botArchive
	var err error
	building.PackageInstanceID, err = cipd.ResolveVersion(ctx, desc.Server, desc.Pkg, desc.Version)
	if err != nil {
		return BotArchiveInfo{}, errors.Fmt("resolving %s to instance ID: %w", desc.Version, err)
	}
	if botConfigPy != nil {
		h := sha256.New()
		_, _ = h.Write(botConfigPy)
		building.BotConfigHash = base64.RawStdEncoding.EncodeToString(h.Sum(nil))
	}
	building.EmbeddedBotSettingsHash = ebs.digest()

	// Check if we already built an archive from these inputs.
	state, err := fetchBotArchiverState(ctx)
	if err != nil {
		return BotArchiveInfo{}, err
	}
	if existing := state.findByInputs(&building); existing != nil {
		// We need to occasionally bump LastTouched to indicate this entry is
		// still used (this also opportunistically deletes old unused archives).
		// There's no need to do it on every request though. If during the bump we
		// discover the entry is gone already (a very rare race condition), go
		// through the complete flow of building a new entry. Otherwise we are done.
		if clock.Since(ctx, existing.LastTouched) > botArchiveTouchPeriod {
			switch found, err := bumpLastTouchedTime(ctx, existing); {
			case err != nil:
				return BotArchiveInfo{}, err
			case !found:
				existing = nil
			}
		}
		if existing != nil {
			return botArchiveInfo(existing, desc), nil
		}
	}

	// Need to actually build a new bot archive.
	logging.Infof(ctx, "Fetching %s/p/%s/+/%s", desc.Server, desc.Pkg, building.PackageInstanceID)
	pkg, err := cipd.FetchInstance(ctx, desc.Server, desc.Pkg, building.PackageInstanceID)
	if err != nil {
		return BotArchiveInfo{}, errors.Fmt("fetching CIPD package: %w", err)
	}
	logging.Infof(ctx, "Building the new bot archive")
	blob, digest, err := buildBotArchive(pkg, botConfigPy, botArchiveConfig{
		Server:            ebs.ServerURL,
		BotCIPDInstanceID: building.PackageInstanceID,
	})
	_ = pkg.Close(ctx, false)
	if err != nil {
		return BotArchiveInfo{}, errors.Fmt("building bot archive: %w", err)
	}

	// Add the new archive to the state if it is still not there.
	logging.Infof(ctx, "Storing the new bot archive (%d bytes): %s", len(blob), digest)
	var built botArchive
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		state, err := fetchBotArchiverState(ctx)
		if err != nil {
			return err
		}

		// It is unlikely but possible a concurrent process added the archive
		// already. We are done in that case. Just mark the archive as used.
		if existing := state.findByInputs(&building); existing != nil {
			existing.LastTouched = clock.Now(ctx).UTC()
			built = *existing
			return datastore.Put(ctx, state)
		}

		// We are adding a new archive. Cleanup old unused ones while at it.
		if err := cleanupOldArchives(ctx, state); err != nil {
			return err
		}

		// Split the new archive into chunks to be stored below.
		var chunks []botArchiveChunk
		var chunkIDs []string
		for i := 0; i < len(blob); i += chunkSize {
			end := min(i+chunkSize, len(blob))
			chunkID := fmt.Sprintf("%s:%d:%d", digest, i, end)
			chunkIDs = append(chunkIDs, chunkID)
			chunks = append(chunks, botArchiveChunk{
				Key:  botArchiveChunkKey(ctx, chunkID),
				Data: blob[i:end],
			})
		}

		// Actually add the new archive to the state.
		built = building
		built.Digest = digest
		built.Chunks = strings.Join(chunkIDs, "\n")
		built.BotConfigRev = botConfigPyRev
		built.LastTouched = clock.Now(ctx).UTC()
		state.Archives = append(state.Archives, built)
		return datastore.Put(ctx, state, chunks)
	}, nil)
	if err != nil {
		return BotArchiveInfo{}, errors.Fmt("updating BotArchiverState: %w", err)
	}
	return botArchiveInfo(&built, desc), nil
}

// fetchBotArchiverState fetches the existing state or returns a new one.
func fetchBotArchiverState(ctx context.Context) (*botArchiverState, error) {
	state := &botArchiverState{Key: botArchiverStateKey(ctx)}
	if err := datastore.Get(ctx, state); err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
		return nil, errors.Fmt("fetching BotArchiverState: %w", err)
	}
	return state, nil
}

// bumpLastTouchedTime bumps LastTouched time in a matching botArchive.
//
// Also deletes old unused archives.
func bumpLastTouchedTime(ctx context.Context, b *botArchive) (found bool, err error) {
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		state, err := fetchBotArchiverState(ctx)
		if err != nil {
			return err
		}
		if existing := state.findByInputs(b); existing != nil {
			found = true
			existing.LastTouched = clock.Now(ctx).UTC()
		} else {
			found = false
		}
		if err := cleanupOldArchives(ctx, state); err != nil {
			return err
		}
		return datastore.Put(ctx, state)
	}, nil)
	if err != nil {
		err = errors.Fmt("in bumpLastTouchedTime: %w", err)
	}
	return
}

// cleanupOldArchives deletes expired botArchive and their chunks.
//
// Must be called within a botArchiverState transaction.
func cleanupOldArchives(ctx context.Context, state *botArchiverState) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("expecting a transaction")
	}
	var oldChunks []*datastore.Key
	var survivors []botArchive
	for _, archive := range state.Archives {
		if age := clock.Since(ctx, archive.LastTouched); age > botArchiveExpiry {
			logging.Infof(ctx, "Dropping bot archive %s:", archive.Digest)
			logging.Infof(ctx, "  Last touched: %s ago", age)
			logging.Infof(ctx, "  CIPD iid: %s", archive.PackageInstanceID)
			logging.Infof(ctx, "  Bot config rev: %s", archive.BotConfigRev)
			for _, chunkID := range strings.Split(archive.Chunks, "\n") {
				oldChunks = append(oldChunks, botArchiveChunkKey(ctx, chunkID))
			}
		} else {
			survivors = append(survivors, archive)
		}
	}
	state.Archives = survivors
	if len(oldChunks) != 0 {
		if err := datastore.Delete(ctx, oldChunks); err != nil {
			return errors.Fmt("deleting unused bot archive chunks: %w", err)
		}
	}
	return nil
}

// botArchiveInfo populates BotArchiveInfo from the config and the built
// archive.
func botArchiveInfo(a *botArchive, desc *configpb.BotDeployment_BotPackage) BotArchiveInfo {
	return BotArchiveInfo{
		Digest:            a.Digest,
		Chunks:            strings.Split(a.Chunks, "\n"),
		BotConfigHash:     a.BotConfigHash,
		BotConfigRev:      a.BotConfigRev,
		PackageInstanceID: a.PackageInstanceID,
		PackageServer:     desc.Server,
		PackageName:       desc.Pkg,
		PackageVersion:    desc.Version,
	}
}

// fetchBotArchive fetches the bot archive from datastore given its chunks.
func fetchBotArchive(ctx context.Context, chunks []string) ([]byte, error) {
	ents := make([]botArchiveChunk, len(chunks))
	for i, chunk := range chunks {
		ents[i] = botArchiveChunk{
			Key: botArchiveChunkKey(ctx, chunk),
		}
	}
	if err := datastore.Get(ctx, ents); err != nil {
		return nil, err
	}
	total := 0
	for _, chunk := range ents {
		total += len(chunk.Data)
	}
	out := make([]byte, 0, total)
	for _, chunk := range ents {
		out = append(out, chunk.Data...)
	}
	return out, nil
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
		return nil, "", errors.Fmt("failed to marshal config.json: %w", err)
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
			return nil, "", errors.Fmt("failed to open %q for reading: %w", name, err)
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
			return nil, "", errors.Fmt("writing zip entry header for %q: %w", name, err)
		}

		// This should write exactly `expectedSize` bytes into both the hasher and
		// the destination zip file. We'll double check.
		wrote, err := io.Copy(io.MultiWriter(hasher, dst), src)
		_ = src.Close()
		if err != nil {
			return nil, "", errors.Fmt("failed to write %q: %w", name, err)
		}
		if uint64(wrote) != expectedSize {
			return nil, "", errors.Fmt("when writing %q wrote %d bytes, while should have %d", name, wrote, expectedSize)
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, "", errors.Fmt("finalizing the zip archive: %w", err)
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
