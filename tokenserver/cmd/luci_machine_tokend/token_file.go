// Copyright 2016 The LUCI Authors.
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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/logging"

	tokenserver "go.chromium.org/luci/tokenserver/api"
)

// stateInToken is stored in the token file in 'tokend_state' field.
//
// The content of this struct is private implementation detail of tokend, so
// we don't bother with proto serialization and use simpler JSON.
type stateInToken struct {
	InputsDigest string // digest of parameters used to generate the token
	Version      string // version of the daemon that produced the token
}

// readTokenFile reads the token file from disk.
//
// In case of problems, logs errors and returns default structs.
func readTokenFile(ctx context.Context, path string) (*tokenserver.TokenFile, *stateInToken) {
	blob, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Warningf(ctx, "Failed to read token file from %q - %s", path, err)
		}
		return &tokenserver.TokenFile{}, &stateInToken{}
	}
	out := &tokenserver.TokenFile{}
	if err = protojson.Unmarshal(blob, out); err != nil {
		logging.Warningf(ctx, "Failed to unmarshal token file %q - %s", path, err)
		return &tokenserver.TokenFile{}, &stateInToken{}
	}
	// Attempt to decode stateInToken. Ignore if doesn't work, no big deal.
	state := &stateInToken{}
	if err = json.Unmarshal(out.TokendState, state); err != nil {
		logging.Warningf(ctx, "Failed to unmarshal tokend_state - %s", err)
		*state = stateInToken{}
	}
	return out, state
}

// writeTokenFiles replaces token files on disk.
//
// It updates tokenFile.TokendState field first, then serializes the token and
// dumps it on disk in multiple copies. Updates the file only if its current
// content is different from the expected one (to avoid disrupting any potential
// concurrent readers of the file).
//
// It replaces the token file atomically, retrying a bunch of times in case of
// concurrent access errors (important on Windows).
//
// Returns true if wrote at least one token (i.e. returns false if all files
// are already up-to-date). Logs some errors inside.
//
// The token file is world-readable (0644 permissions).
func writeTokenFiles(ctx context.Context, tokenFile *tokenserver.TokenFile, state *stateInToken, paths []string) (bool, error) {
	if len(paths) == 0 {
		return false, nil
	}

	stateBlob, err := json.Marshal(state)
	if err != nil {
		logging.Errorf(ctx, "Failed to marshal tokend_state - %s", err)
		return false, err
	}
	tokenFile.TokendState = stateBlob

	opts := protojson.MarshalOptions{Indent: "  "}
	blob, err := opts.Marshal(tokenFile)
	if err != nil {
		logging.Errorf(ctx, "Failed to marshal the token file - %s", err)
		return false, err
	}

	// A token file is never empty, we can use nil slice as an indicator of a
	// missing or unreadable file.
	bestEffortRead := func(path string) []byte {
		blob, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		return blob
	}

	var firstErr error
	var wroteFile bool
	for _, path := range paths {
		if bytes.Equal(blob, bestEffortRead(path)) {
			continue
		}
		if err := AtomicWriteFile(ctx, path, blob, 0644); err != nil {
			logging.Errorf(ctx, "Failed to write token file %s - %s", path, err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			wroteFile = true
		}
	}
	return wroteFile, firstErr
}
