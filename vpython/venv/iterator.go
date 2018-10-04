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

package venv

import (
	"context"
	"io"
	"os"
	"strings"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// forEachReadDirSize is the number of entries to read in a directory at a time
// when iterating over VirtualEnv.
const forEachReadDirSize = 128

// Iterator iterates over the contents of a "vpython" configuration directory,
// returning all associated VirtualEnv instances.
type Iterator struct {
	// Only return VirtualEnv entries with completion flags.
	OnlyComplete bool

	// Shuffle VirtualEnv results before returning them.
	Shuffle bool
}

// ForEach iterates over all VirtualEnv installations for the supplied "cfg".
//
// "cb" will be invoked for each VirtualEnv, regardless of its completion
// status. The callback may perform additional operations on the VirtualEnv to
// determine its actual status. If the callback returns an error, iteration will
// stop and the error will be forwarded.
//
// If the supplied Context is cancelled, iteration will stop prematurely and
// return the Context's error.
func (it *Iterator) ForEach(c context.Context, cfg *Config, cb func(context.Context, *Env) error) error {
	return iterDir(c, cfg.BaseDir, func(fileInfos []os.FileInfo) error {
		if it.Shuffle {
			for i := range fileInfos {
				j := mathrand.Intn(c, i+1)
				fileInfos[i], fileInfos[j] = fileInfos[j], fileInfos[i]
			}
		}

		for _, fi := range fileInfos {
			// Ignore hidden files.
			if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
				continue
			}

			e := cfg.envForName(fi.Name(), nil)
			if it.OnlyComplete {
				if err := e.AssertCompleteAndLoad(); err != nil {
					logging.WithError(err).Debugf(c, "Skipping VirtualEnv %s; not complete.", fi.Name())
					continue
				}
			}

			if err := cb(c, e); err != nil {
				return err
			}
		}

		return nil
	})
}

func iterDir(c context.Context, dirPath string, cb func([]os.FileInfo) error) error {
	// Get a listing of all VirtualEnv within the base directory.
	dir, err := os.Open(dirPath)
	if err != nil {
		return errors.Annotate(err, "failed to open base directory: %s", dirPath).Err()
	}
	defer dir.Close()

	for done := false; !done; {
		// Check if we've been cancelled.
		select {
		case <-c.Done():
			return c.Err()
		default:
		}

		// Read the next batch of directories.
		fileInfos, err := dir.Readdir(forEachReadDirSize)
		switch err {
		case nil:

		case io.EOF:
			done = true

		default:
			return errors.Annotate(err, "could not read directory contents: %s", dirPath).Err()
		}

		if err := cb(fileInfos); err != nil {
			return err
		}
	}

	return nil
}
