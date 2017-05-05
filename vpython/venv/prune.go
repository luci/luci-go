// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"github.com/danjacques/gofslock/fslock"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
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
	pruneThreshold := cfg.PruneThreshold
	if pruneThreshold <= 0 {
		// Pruning is disabled.
		return nil
	}

	now := clock.Now(c)
	minPruneAge := now.Add(-pruneThreshold)

	// Run a series of independent scan/prune operations.
	logging.Debugf(c, "Pruning entries in [%s] older than %s (%s).", cfg.BaseDir, pruneThreshold, minPruneAge)

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

		if ts, err := e.completionFlagTimestamp(); err == nil && ts.After(minPruneAge) {
			logging.Debugf(c, "Environment [%s] is younger than minimum prune age (%s).", e.Name, ts)
			return nil
		}

		switch err := e.Delete(c); errors.Unwrap(err) {
		case nil:
			totalPruned++
			if cfg.MaxPrunesPerSweep > 0 && totalPruned >= cfg.MaxPrunesPerSweep {
				logging.Debugf(c, "Hit prune limit of %d.", cfg.MaxPrunesPerSweep)
				hitLimitStr = " (limit)"
				cancelFunc()
			}

		case fslock.ErrLockHeld:
			logging.WithError(err).Debugf(c, "Environment [%s] is in use.", e.Name)

		default:
			err = errors.Annotate(err).Reason("failed to prune file: %(name)s").
				D("name", e.Name).
				D("dir", e.Config.BaseDir).
				Err()
			allErrs = append(allErrs, err)
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
