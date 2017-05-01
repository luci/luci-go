// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"os"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/stringset"
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
// If exempt is not nil, it contains a list of VirtualEnv names that will be
// exempted from pruning. This is used to prevent pruning from modifying
// environments that are known to be recently used, and to completely avoid a
// case where an environment could be pruned while it's in use by this program.
func prune(c context.Context, cfg *Config, exempt stringset.Set) error {
	if cfg.PruneThreshold <= 0 {
		// Pruning is disabled.
		return nil
	}
	pruneThreshold := clock.Now(c).Add(-cfg.PruneThreshold)

	// Run a series of independent scan/prune operations.
	logging.Debugf(c, "Pruning entries in [%s] older than %s.", cfg.BaseDir, cfg.PruneThreshold)

	// Iterate over all of our VirtualEnv candidates.
	//
	// Any pruning errors will be accumulated in "allErrs", so ForEach will only
	// receive nil return values from the callback. This means that any error
	// returned by ForEach was an actual error with the iteration itself.
	var (
		allErrs     errors.MultiError
		totalPruned = 0
		hitLimitStr = ""
	)

	// We need to cancel if we hit our prune limit.
	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	// Iterate over all VirtualEnv directories, regardless of their completion
	// status.
	it := Iterator{
		// Shuffle the slice randomly. We do this in case others are also processing
		// this directory simultaneously.
		Shuffle: true,
	}
	err := it.ForEach(c, cfg, func(c context.Context, e *Env) error {
		if exempt != nil && exempt.Has(e.Name) {
			logging.Debugf(c, "Not pruning currently in-use environment: %s", e.Name)
			return nil
		}

		switch pruned, err := maybePruneFile(c, e, pruneThreshold); {
		case err != nil:
			err = errors.Annotate(err).Reason("failed to prune file: %(name)s").
				D("name", e.Name).
				D("dir", e.Config.BaseDir).
				Err()
			allErrs = append(allErrs, err)

		case pruned:
			totalPruned++
			if cfg.MaxPrunesPerSweep > 0 && totalPruned >= cfg.MaxPrunesPerSweep {
				logging.Debugf(c, "Hit prune limit of %d.", cfg.MaxPrunesPerSweep)
				hitLimitStr = " (limit)"
				cancelFunc()
			}
		}
		return nil
	})
	if err != nil {
		// Error during iteration.
		return err
	}

	logging.Infof(c, "Pruned %d environment(s)%s with %d error(s)", totalPruned, hitLimitStr, len(allErrs))
	if len(allErrs) > 0 {
		return allErrs
	}
	return nil
}

// maybePruneFile examines the specified FileIfo within cfg.BaseDir and
// determines if it should be pruned.
func maybePruneFile(c context.Context, e *Env, pruneThreshold time.Time) (pruned bool, err error) {
	// Grab the lock file for this directory.
	err = e.withLockNonBlocking(func() error {
		// Read the complete flag file's timestamp.
		switch st, err := os.Stat(e.completeFlagPath); {
		case err == nil:
			if !st.ModTime().Before(pruneThreshold) {
				return nil
			}

			logging.Infof(c, "Env [%s] is older than the prune threshold (%v < %v); pruning...",
				e.Name, st.ModTime(), pruneThreshold)

		case os.IsNotExist(err):
			logging.Infof(c, "Env [%s] has no completed flag; pruning...", e.Name)

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
