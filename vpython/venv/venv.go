// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/python"
	"github.com/luci/luci-go/vpython/spec"
	"github.com/luci/luci-go/vpython/wheel"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/system/filesystem"

	"github.com/danjacques/gofslock/fslock"
	"golang.org/x/net/context"
)

const (
	lockHeldDelay            = 5 * time.Second
	defaultKeepAliveInterval = 1 * time.Hour
)

// blocker is an fslock.Blocker implementation that sleeps lockHeldDelay in
// between attempts.
func blocker(c context.Context) fslock.Blocker {
	return func() error {
		logging.Debugf(c, "Lock is currently held. Sleeping %v and retrying...", lockHeldDelay)
		clock.Sleep(c, lockHeldDelay)
		return nil
	}
}

func withTempDir(l logging.Logger, prefix string, fn func(string) error) error {
	tdir, err := ioutil.TempDir("", prefix)
	if err != nil {
		return errors.Annotate(err).Reason("failed to create temporary directory").Err()
	}
	defer func() {
		if err := filesystem.RemoveAll(tdir); err != nil {
			l.Warningf("Failed to remove temporary directory: %s", err)
		}
	}()

	return fn(tdir)
}

// EnvRootFromSpecPath calculates the environment root from an exported
// environment specification file path.
//
// The specification path is: <EnvRoot>/<SpecHash>/<spec>, so our EnvRoot
// is two directories up.
//
// We export EnvSpecPath as an asbolute path. However, since someone else
// could have overridden it or exported their own, let's make sure.
func EnvRootFromSpecPath(path string) (string, error) {
	if err := filesystem.AbsPath(&path); err != nil {
		return "", errors.Annotate(err).
			Reason("failed to get absolute path for specification file path: %(path)s").
			Err()
	}
	return filepath.Dir(filepath.Dir(path)), nil
}

// With creates a new Env and executes "fn" with assumed ownership of that Env.
//
// The Context passed to "fn" will be cancelled if we lose perceived ownership
// of the configured environment. This is not an expected scenario, and should
// be considered an error condition. The Env passed to "fn" is valid only for
// the duration of the callback.
//
// It will lock around the VirtualEnv to ensure that multiple processes do not
// conflict with each other. If a VirtualEnv for this specification already
// exists, it will be used directly without any additional setup.
//
// If another process holds the lock, With will return an error (if blocking is
// false) or try again until it obtains the lock (if blocking is true).
func With(c context.Context, cfg Config, blocking bool, fn func(context.Context, *Env) error) error {
	e, err := cfg.setupEnv(c)
	if err != nil {
		return errors.Annotate(err).Reason("failed to resolve VirtualEnv").Err()
	}

	// Setup the environment.
	if err := e.setupImpl(c, blocking); err != nil {
		return errors.Annotate(err).Err()
	}

	// Perform a pruning round. Failure is non-fatal.
	if perr := prune(c, e.Config, e.name); perr != nil {
		logging.WithError(perr).Warningf(c, "Failed to perform pruning round after initialization.")
	}

	// Set-up our environment Context.
	var monitorWG sync.WaitGroup
	c, cancelFunc := context.WithCancel(c)
	defer func() {
		// Cancel our Context and reap our monitor goroutine(s).
		cancelFunc()
		monitorWG.Wait()
	}()

	// If we have a prune threshold, touch the "complete flag" periodically.
	//
	// Our refresh interval must be at least 1/4 the prune threshold, but ideally
	// would be higher.
	if interval := e.Config.PruneThreshold / 4; interval > 0 {
		if interval > defaultKeepAliveInterval {
			interval = defaultKeepAliveInterval
		}

		monitorWG.Add(1)
		go func() {
			defer monitorWG.Done()
			e.keepAliveMonitor(c, interval, cancelFunc)
		}()
	}

	return fn(c, e)
}

// Env is a fully set-up Python virtual environment. It is configured
// based on the contents of an vpython.Spec file by Setup.
//
// Env should not be instantiated directly; it must be created by calling
// Config.Env.
//
// All paths in Env are absolute.
type Env struct {
	// Config is this Env's Config, fully-resolved.
	Config *Config

	// Root is the Env container's root directory path.
	Root string

	// Python is the path to the Env Python interpreter.
	Python string

	// SepcPath is the path to the specification file that was used to construct
	// this environment. It will be in text protobuf format, and, therefore,
	// suitable for input to other "vpython" invocations.
	SpecPath string

	// name is the hash of the specification file for this Env.
	name string
	// lockPath is the path to this Env-specific lock file. It will be at:
	// "<baseDir>/.<name>.lock".
	lockPath string
	// completeFlagPath is the path to this Env's complete flag.
	// It will be at "<Root>/complete.flag".
	completeFlagPath string
}

// InterpreterCommand returns a Python interpreter Command pointing to the
// VirtualEnv's Python installation.
func (e *Env) InterpreterCommand() *python.Command {
	i := python.Interpreter{
		Python: e.Python,
	}
	cmd := i.Command()
	cmd.Isolated = true
	return cmd
}

func (e *Env) setupImpl(c context.Context, blocking bool) error {
	// Repeatedly try and create our Env. We do this so that if we
	// encounter a lock, we will let the other process finish and try and leverage
	// its success.
	for {
		// We will be creating the Env. We will lock around a file for this Env hash
		// so that any other processes that may be trying to simultaneously
		// manipulate Env will be forced to wait.
		err := fslock.With(e.lockPath, func() error {
			// Check for completion flag.
			if _, err := os.Stat(e.completeFlagPath); err != nil {
				// No complete flag. Create a new VirtualEnv here.
				if err := e.createLocked(c); err != nil {
					return errors.Annotate(err).Reason("failed to create new VirtualEnv").Err()
				}

				// Successfully created the environment! Mark this with a completion
				// flag.
				if err := e.touchCompleteFlagLocked(); err != nil {
					return errors.Annotate(err).Reason("failed to create complete flag").Err()
				}
			} else {
				// Fast path: if our complete flag is present, assume that the
				// environment is setup and complete. No locking or additional work necessary.
				logging.Debugf(c, "Completion flag found! Environment is set-up: %s", e.completeFlagPath)

				// Mark that we care about this enviornment. This is non-fatal if it
				// fails.
				if err := e.touchCompleteFlagLocked(); err != nil {
					logging.WithError(err).Warningf(c, "Failed to update existing complete flag.")
				}
			}

			return nil
		})
		switch err {
		case nil:
			return nil

		case fslock.ErrLockHeld:
			if !blocking {
				return errors.Annotate(err).Reason("VirtualEnv lock is currently held (non-blocking)").Err()
			}

			// Some other process holds the lock. Sleep a little and retry.
			logging.Warningf(c, "VirtualEnv lock is currently held. Retrying after delay (%s)...", lockHeldDelay)
			if tr := clock.Sleep(c, lockHeldDelay); tr.Incomplete() {
				return tr.Err
			}
			continue

		default:
			return errors.Annotate(err).Reason("failed to create VirtualEnv").Err()
		}
	}
}

func (e *Env) withLockBlocking(c context.Context, fn func() error) error {
	return fslock.WithBlocking(e.lockPath, blocker(c), fn)
}

// Delete deletes this environment, if it exists.
func (e *Env) Delete(c context.Context) error {
	err := e.withLockBlocking(c, func() error {
		if err := e.deleteLocked(c); err != nil {
			return errors.Annotate(err).Err()
		}
		return nil
	})
	if err != nil {
		errors.Annotate(err).Reason("failed to delete environment").Err()
	}
	return nil
}

func (e *Env) createLocked(c context.Context) error {
	// If our root directory already exists, delete it.
	if _, err := os.Stat(e.Root); err == nil {
		logging.Warningf(c, "Deleting existing VirtualEnv: %s", e.Root)
		if err := filesystem.RemoveAll(e.Root); err != nil {
			return errors.Reason("failed to remove existing root").Err()
		}
	}

	// Make sure our environment's base directory exists.
	if err := filesystem.MakeDirs(e.Root); err != nil {
		return errors.Annotate(err).Reason("failed to create environment root").Err()
	}
	logging.Infof(c, "Using virtual environment root: %s", e.Root)

	// Build our package list. Always install our base VirtualEnv package.
	packages := make([]*vpython.Spec_Package, 1, 1+len(e.Config.Spec.Wheel))
	packages[0] = e.Config.Spec.Virtualenv
	packages = append(packages, e.Config.Spec.Wheel...)

	// Create a directory to bootstrap VirtualEnv from.
	//
	// This directory will be a very short-named temporary directory. This is
	// because it really quickly runs up into traditional Windows path limitations
	// when ZIP-importing sub-sub-sub-sub-packages (e.g., pip, requests, etc.).
	//
	// We will clean this directory up on termination.
	err := withTempDir(logging.Get(c), "vpython_bootstrap", func(bootstrapDir string) error {
		pkgDir := filepath.Join(bootstrapDir, "packages")
		if err := filesystem.MakeDirs(pkgDir); err != nil {
			return errors.Annotate(err).Reason("could not create bootstrap packages directory").Err()
		}

		if err := e.downloadPackages(c, pkgDir, packages); err != nil {
			return errors.Annotate(err).Reason("failed to download packages").Err()
		}

		// Installing base VirtualEnv.
		if err := e.installVirtualEnv(c, pkgDir); err != nil {
			return errors.Annotate(err).Reason("failed to install VirtualEnv").Err()
		}

		// Install our wheel files.
		if len(e.Config.Spec.Wheel) > 0 {
			// Install wheels into our VirtualEnv.
			if err := e.installWheels(c, bootstrapDir, pkgDir); err != nil {
				return errors.Annotate(err).Reason("failed to install wheels").Err()
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Write our specification file.
	if err := spec.Write(e.Config.Spec, e.SpecPath); err != nil {
		return errors.Annotate(err).Reason("failed to write spec file to: %(path)s").
			D("path", e.SpecPath).
			Err()
	}
	logging.Debugf(c, "Wrote specification file to: %s", e.SpecPath)

	// Finalize our VirtualEnv for bootstrap execution.
	if err := e.finalize(c); err != nil {
		return errors.Annotate(err).Reason("failed to prepare VirtualEnv").Err()
	}

	return nil
}

func (e *Env) downloadPackages(c context.Context, dst string, packages []*vpython.Spec_Package) error {
	// Create a wheel sub-directory underneath of root.
	logging.Debugf(c, "Loading %d package(s) into: %s", len(packages), dst)
	if err := e.Config.Loader.Ensure(c, dst, packages); err != nil {
		return errors.Annotate(err).Reason("failed to download packages").Err()
	}
	return nil
}

func (e *Env) installVirtualEnv(c context.Context, pkgDir string) error {
	// Create our VirtualEnv package staging sub-directory underneath of root.
	bsDir := filepath.Join(e.Root, ".virtualenv")
	if err := filesystem.MakeDirs(bsDir); err != nil {
		return errors.Annotate(err).Reason("failed to create VirtualEnv bootstrap directory").
			D("path", bsDir).
			Err()
	}

	// Identify the virtualenv directory: will have "virtualenv-" prefix.
	matches, err := filepath.Glob(filepath.Join(pkgDir, "virtualenv-*"))
	if err != nil {
		return errors.Annotate(err).Reason("failed to glob for 'virtualenv-' directory").Err()
	}
	if len(matches) == 0 {
		return errors.Reason("no 'virtualenv-' directory provided by package").Err()
	}

	logging.Debugf(c, "Creating VirtualEnv at: %s", e.Root)
	i := e.Config.systemInterpreter()
	i.WorkDir = matches[0]
	err = i.Run(c,
		"virtualenv.py",
		"--no-download",
		e.Root)
	if err != nil {
		return errors.Annotate(err).Reason("failed to create VirtualEnv").Err()
	}

	return nil
}

func (e *Env) installWheels(c context.Context, bootstrapDir, pkgDir string) error {
	// Identify all downloaded wheels and parse them.
	wheels, err := wheel.ScanDir(pkgDir)
	if err != nil {
		return errors.Annotate(err).Reason("failed to load wheels").Err()
	}

	// Build a "wheel" requirements file.
	reqPath := filepath.Join(bootstrapDir, "requirements.txt")
	logging.Debugf(c, "Rendering requirements file to: %s", reqPath)
	if err := wheel.WriteRequirementsFile(reqPath, wheels); err != nil {
		return errors.Annotate(err).Reason("failed to render requirements file").Err()
	}

	cmd := e.venvInterpreter()
	err = cmd.Run(c,
		"-m", "pip",
		"install",
		"--use-wheel",
		"--compile",
		"--no-index",
		"--find-links", pkgDir,
		"--requirement", reqPath)
	if err != nil {
		return errors.Annotate(err).Reason("failed to install wheels").Err()
	}
	return nil
}

func (e *Env) finalize(c context.Context) error {
	// Uninstall "pip" and "wheel", preventing (easy) augmentation of the
	// environment.
	if !e.Config.testPreserveInstallationCapability {
		cmd := e.venvInterpreter()
		err := cmd.Run(c,
			"-m", "pip",
			"uninstall",
			"--quiet",
			"--yes",
			"pip", "wheel")
		if err != nil {
			return errors.Annotate(err).Reason("failed to install wheels").Err()
		}
	}

	// Change all files to read-only, except:
	// - Our root directory, which must be writable in order to update our
	//   completion flag.
	// - Our completion flag, which must be trivially re-writable.
	if !e.Config.testLeaveReadWrite {
		err := filesystem.MakeReadOnly(e.Root, func(path string) bool {
			switch path {
			case e.Root, e.completeFlagPath:
				return false
			default:
				return true
			}
		})
		if err != nil {
			return errors.Annotate(err).Reason("failed to mark environment read-only").Err()
		}
	}
	return nil
}

func (e *Env) venvInterpreter() *python.Command {
	cmd := e.InterpreterCommand()
	cmd.WorkDir = e.Root
	return cmd
}

// touchCompleteFlag touches the complete flag, creating it and/or
// updating its timestamp.
func (e *Env) touchCompleteFlag(c context.Context) error {
	err := e.withLockBlocking(c, e.touchCompleteFlagLocked)
	if err != nil {
		return errors.Annotate(err).Reason("failed to touch complete flag").Err()
	}
	return err
}

func (e *Env) touchCompleteFlagLocked() error {
	if err := filesystem.Touch(e.completeFlagPath, time.Time{}, 0644); err != nil {
		return errors.Annotate(err).Err()
	}
	return nil
}

func (e *Env) deleteLocked(c context.Context) error {
	// Delete our environment directory.
	if err := filesystem.RemoveAll(e.Root); err != nil {
		return errors.Annotate(err).Reason("failed to delete environment root").Err()
	}

	// Delete our lock path.
	if err := os.Remove(e.lockPath); err != nil {
		return errors.Annotate(err).Reason("failed to delete lock").Err()
	}
	return nil
}

// keepAliveMonitor periodically refreshes the environment's "completed" flag
// timestamp.
//
// It runs in its own goroutine.
func (e *Env) keepAliveMonitor(c context.Context, interval time.Duration, cancelFunc context.CancelFunc) {
	timer := clock.NewTimer(c)
	defer timer.Stop()

	for {
		logging.Debugf(c, "Keep-alive: Sleeping %s...", interval)
		timer.Reset(interval)
		select {
		case <-c.Done():
			logging.WithError(c.Err()).Debugf(c, "Keep-alive: monitor's Context was cancelled.")
			return

		case tr := <-timer.GetC():
			if tr.Err != nil {
				logging.WithError(tr.Err).Debugf(c, "Keep-alive: monitor's timer finished.")
				return
			}
		}

		logging.Debugf(c, "Keep-alive: Updating completed flag timestamp.")
		if err := e.touchCompleteFlag(c); err != nil {
			errors.Log(c, errors.Annotate(err).Reason("failed to refresh timestamp").Err())
			cancelFunc()
			return
		}
	}
}
