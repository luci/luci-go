// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"github.com/danjacques/gofslock/fslock"
	"golang.org/x/net/context"
)

// pruneReadDirSize is the number of entries to read in a directory at a time
// when pruning.
const pruneReadDirSize = 128

// prune examines environments in cfg's BaseDir. If any are found that are older
// than the prune threshold in "cfg", they will be safely deleted.
//
// If forceKeep is not empty, prune will skip pruning the VirtualEnv named
// "forceKeep", even if it is would otherwise be candidate for pruning.
func prune(c context.Context, cfg *Config, forceKeep string) error {
	if cfg.PruneThreshold <= 0 {
		// Pruning is disabled.
		return nil
	}
	pruneThreshold := clock.Now(c).Add(-cfg.PruneThreshold)

	// Get a listing of all VirtualEnv within the base directory.
	dir, err := os.Open(cfg.BaseDir)
	if err != nil {
		return errors.Annotate(err).Reason("failed to open base directory: %(dir)s").
			D("dir", cfg.BaseDir).
			Err()
	}
	defer dir.Close()

	// Run a series of independent scan/prune operations.
	logging.Debugf(c, "Pruning entries in [%s] older than %s.", cfg.BaseDir, cfg.PruneThreshold)

	var (
		allErrs     errors.MultiError
		totalPruned = 0
		done        = false
		hitLimitStr = ""
	)
	for !done {
		fileInfos, err := dir.Readdir(pruneReadDirSize)
		switch err {
		case nil:

		case io.EOF:
			done = true

		default:
			return errors.Annotate(err).Reason("could not read directory contents: %(dir)s").
				D("dir", err).
				Err()
		}

		// Shuffle the slice randomly. We do this in case others are also processing
		// this directory simultaneously.
		for i := range fileInfos {
			j := mathrand.Intn(c, i+1)
			fileInfos[i], fileInfos[j] = fileInfos[j], fileInfos[i]
		}

		for _, fi := range fileInfos {
			// Ignore hidden files. This includes the package loader root.
			if strings.HasPrefix(fi.Name(), ".") {
				continue
			}

			switch pruned, err := maybePruneFile(c, cfg, fi, pruneThreshold, forceKeep); {
			case err != nil:
				allErrs = append(allErrs, errors.Annotate(err).
					Reason("failed to prune file: %(name)s").
					D("name", fi.Name()).
					D("dir", cfg.BaseDir).
					Err())

			case pruned:
				totalPruned++
				if cfg.MaxPrunesPerSweep > 0 && totalPruned >= cfg.MaxPrunesPerSweep {
					logging.Debugf(c, "Hit prune limit of %d.", cfg.MaxPrunesPerSweep)
					done, hitLimitStr = true, " (limit)"
				}
			}
		}
	}

	logging.Infof(c, "Pruned %d environment(s)%s with %d error(s)", totalPruned, hitLimitStr, len(allErrs))
	if len(allErrs) > 0 {
		return allErrs
	}
	return nil
}

// maybePruneFile examines the specified FileIfo within cfg.BaseDir and
// determines if it should be pruned.
func maybePruneFile(c context.Context, cfg *Config, fi os.FileInfo, pruneThreshold time.Time,
	forceKeep string) (pruned bool, err error) {

	name := fi.Name()
	if !fi.IsDir() || name == forceKeep {
		logging.Debugf(c, "Not pruning currently in-use file: %s", name)
		return
	}

	// Grab the lock file for this directory.
	e := cfg.envForName(name, nil)
	err = fslock.With(e.lockPath, func() error {
		// Read the complete flag file's timestamp.
		switch st, err := os.Stat(e.completeFlagPath); {
		case err == nil:
			if !st.ModTime().Before(pruneThreshold) {
				return nil
			}

			logging.Infof(c, "Env [%s] is older than the prune threshold (%v < %v); pruning...",
				e.name, st.ModTime(), pruneThreshold)

		case os.IsNotExist(err):
			logging.Infof(c, "Env [%s] has no completed flag; pruning...", e.name)

		default:
			return errors.Annotate(err).Reason("failed to stat complete flag: %(path)s").
				D("path", e.completeFlagPath).
				Err()
		}

		// Delete the environment. We currently hold its lock, so use deleteLocked.
		if err := e.deleteLocked(c); err != nil {
			return errors.Annotate(err).Reason("failed to delete Env").Err()
		}
		pruned = true
		return nil
	})
	switch err {
	case nil:
		return

	case fslock.ErrLockHeld:
		// Something else currently holds the lock for this directory, so ignore it.
		logging.Warningf(c, "Lock [%s] is currently held; skipping.", e.lockPath)
		return

	default:
		err = errors.Annotate(err).Err()
		return
	}
}
