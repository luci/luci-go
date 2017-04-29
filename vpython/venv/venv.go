// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/danjacques/gofslock/fslock"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/python"
	"github.com/luci/luci-go/vpython/wheel"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/system/filesystem"
)

// ErrNotComplete is a sentinel error returned by AssertCompleteAndLoad to
// indicate that the Environment is missing its completion flag.
var ErrNotComplete = errors.New("environment is not complete")

const (
	lockHeldDelay            = 10 * time.Millisecond
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

// EnvRootFromStampPath calculates the environment root from an exported
// environment specification file path.
//
// The specification path is: <EnvRoot>/<SpecHash>/EnvironmentStampPath, so our
// EnvRoot is two directories up.
//
// We export EnvSpecPath as an asbolute path. However, since someone else
// could have overridden it or exported their own, let's make sure.
func EnvRootFromStampPath(path string) (string, error) {
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
	// Start with an empty VirtualEnv. We will use this to probe the local
	// system.
	//
	// If our configured VirtualEnv is, itself, an empty then we can
	// skip this.
	var e *vpython.Environment
	if cfg.HasWheels() {
		// Use an empty VirtualEnv to probe the runtime environment.
		var err error
		if e, err = getRuntimeEnvironment(c, blocking, cfg.WithoutWheels()); err != nil {
			return errors.Annotate(err).Reason("failed to get runtime environment").Err()
		}
	}

	// Run the real config, now with runtime data.
	env, err := cfg.makeEnv(c, e)
	if err != nil {
		return err
	}
	return env.withImpl(c, blocking, fn)
}

func getRuntimeEnvironment(c context.Context, blocking bool, cfg *Config) (*vpython.Environment, error) {
	e, err := cfg.makeEnv(c, nil)
	if err != nil {
		return nil, err
	}

	// If we already have the environment specification for this environment, then
	// load it.
	switch err := e.AssertCompleteAndLoad(); {
	case err != nil:
		// Not a big deal, but
		logging.Fields{
			logging.ErrorKey: err,
			"path":           e.EnvironmentStampPath,
		}.Debugf(c, "Failed to load stamp file; performing full initialization.")

	default:
		logging.Debugf(c, "Loaded environment stamp from empty VirtualEnv: %s", e.Environment)

		// Try and touch the complete flag. It doesn't matter if we fail, since all
		// it will do is refrain from acknowledging the utility of the empty
		// environment.
		if err := e.touchCompleteFlagNonBlocking(); err != nil {
			logging.Debugf(c, "Failed to update empty environment flag timestamp.")
		}
		return e.Environment, nil
	}

	// Fully initialize the Env, which will popluate its Environment.
	if err := e.setupImpl(c, blocking); err != nil {
		return nil, err
	}
	return e.Environment, nil
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

	// Name is the hash of the specification file for this Env.
	Name string

	// Python is the path to the Env Python interpreter.
	Python string

	// Environment is the resolved Python environment that this VirtualEnv is
	// configured with. It is resolved during environment setup and saved into the
	// environment's EnvironmentStampPath.
	Environment *vpython.Environment

	// BinDir is the VirtualEnv "bin" directory, containing Python and installed
	// wheel binaries.
	BinDir string
	// EnvironmentStampPath is the path to the vpython.Environment stamp file that
	// details a constructed environment. It will be in text protobuf format, and,
	// therefore, suitable for input to other "vpython" invocations.
	EnvironmentStampPath string

	// lockPath is the path to this Env-specific lock file. It will be at:
	// "<baseDir>/.<name>.lock".
	lockPath string
	// completeFlagPath is the path to this Env's complete flag.
	// It will be at "<Root>/complete.flag".
	completeFlagPath string
	// interpreter is the VirtualEnv Python interpreter.
	//
	// It is configured to use the Python member as its base executable, and is
	// initialized on first call to Interpreter.
	interpreter *python.Interpreter
}

func (e *Env) setupImpl(c context.Context, blocking bool) error {
	// Repeatedly try and create our Env. We do this so that if we
	// encounter a lock, we will let the other process finish and try and leverage
	// its success.
	for {
		// We will be creating the Env. We will lock around a file for this Env hash
		// so that any other processes that may be trying to simultaneously
		// manipulate Env will be forced to wait.
		err := e.withLockNonBlocking(func() error {
			// Check for completion flag.
			if err := e.AssertCompleteAndLoad(); err != nil {
				logging.WithError(err).Debugf(c, "VirtualEnv is not complete.")

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
				// environment is setup and complete. No locking or additional work
				// necessary.
				//
				// This will generally happen if another process created the environment
				// in between when we checked for the completion stamp initially and
				// when we actually obtained the lock.
				logging.Debugf(c, "Completion flag found! Environment is set-up: %s", e.completeFlagPath)

				// Mark that we care about this environment. This is non-fatal if it
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
			// Try again to load the environment stamp, asserting the existence of the
			// completion flag in the process. If another process has created the
			// environment, we may be able to use it without ever having to obtain
			// its lock!
			if err := e.AssertCompleteAndLoad(); err == nil {
				logging.Infof(c, "Completion stamp was created while waiting for lock: %s", e.EnvironmentStampPath)
				return nil
			}

			logging.Fields{
				logging.ErrorKey: err,
				"path":           e.EnvironmentStampPath,
			}.Debugf(c, "Lock is held, and environment is not complete.")

			if !blocking {
				return errors.Annotate(err).Reason("VirtualEnv lock is currently held (non-blocking)").Err()
			}

			// Some other process holds the lock. Sleep a little and retry.
			logging.Infof(c, "VirtualEnv lock is currently held. Retrying after delay (%s)...", lockHeldDelay)
			if tr := clock.Sleep(c, lockHeldDelay); tr.Incomplete() {
				return tr.Err
			}
			continue

		default:
			return errors.Annotate(err).Reason("failed to create VirtualEnv").Err()
		}
	}
}

func (e *Env) withImpl(c context.Context, blocking bool, fn func(context.Context, *Env) error) error {
	// Setup the VirtualEnv environment.
	if err := e.setupImpl(c, blocking); err != nil {
		return err
	}

	// Perform a pruning round. Failure is non-fatal.
	if perr := prune(c, e.Config, e.Name); perr != nil {
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

// Interpreter returns the VirtualEnv's isolated Python Interpreter instance.
func (e *Env) Interpreter() *python.Interpreter {
	if e.interpreter == nil {
		e.interpreter = &python.Interpreter{
			Python: e.Python,
		}
	}
	return e.interpreter
}

func (e *Env) withLockNonBlocking(fn func() error) error {
	return fslock.With(e.lockPath, fn)
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

// WriteEnvironmentStamp writes a text protobuf form of spec to path.
func (e *Env) WriteEnvironmentStamp() error {
	environment := e.Environment
	if environment == nil {
		environment = &vpython.Environment{}
	}
	return writeTextProto(e.EnvironmentStampPath, environment)
}

// AssertCompleteAndLoad asserts that the VirtualEnv's completion
// flag exists. If it does, the environment's stamp is loaded into e.Environment
// and nil is returned.
//
// An error is returned if the completion flag does not exist, or if the
// VirtualEnv environment stamp could not be loaded.
func (e *Env) AssertCompleteAndLoad() error {
	// Ensure that the environment has its completion flag.
	switch _, err := os.Stat(e.completeFlagPath); {
	case filesystem.IsNotExist(err):
		return ErrNotComplete
	case err != nil:
		return errors.Annotate(err).Reason("failed to check for completion flag").Err()
	}

	content, err := ioutil.ReadFile(e.EnvironmentStampPath)
	if err != nil {
		return errors.Annotate(err).Reason("failed to load file from: %(path)s").
			D("path", e.EnvironmentStampPath).
			Err()
	}

	var environment vpython.Environment
	if err := proto.UnmarshalText(string(content), &environment); err != nil {
		return errors.Annotate(err).Reason("failed to unmarshal vpython.Env stamp from: %(path)s").
			D("path", e.EnvironmentStampPath).
			Err()
	}
	e.Environment = &environment
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
	packages := make([]*vpython.Spec_Package, 1, 1+len(e.Environment.Spec.Wheel))
	packages[0] = e.Environment.Spec.Virtualenv
	packages = append(packages, e.Environment.Spec.Wheel...)

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

		// Load PEP425 tags, if we don't already have them.
		if e.Environment.Pep425Tag == nil {
			pep425Tags, err := e.getPEP425Tags(c)
			if err != nil {
				return errors.Annotate(err).Reason("failed to get PEP425 tags").Err()
			}
			e.Environment.Pep425Tag = pep425Tags
		}

		// Install our wheel files.
		if len(e.Environment.Spec.Wheel) > 0 {
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
	if err := e.WriteEnvironmentStamp(); err != nil {
		return errors.Annotate(err).Reason("failed to write environment stamp file to: %(path)s").
			D("path", e.EnvironmentStampPath).
			Err()
	}
	logging.Debugf(c, "Wrote environment stamp file to: %s", e.EnvironmentStampPath)

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
	cmd := e.Config.systemInterpreter().IsolatedCommand(c,
		"virtualenv.py",
		"--no-download",
		e.Root)
	cmd.Dir = matches[0]
	attachOutputForLogging(c, logging.Debug, cmd)
	if err := cmd.Run(); err != nil {
		return errors.Annotate(err).Reason("failed to create VirtualEnv").Err()
	}

	return nil
}

// getPEP425Tags calls Python's pip.pep425tags package to retrieve the tags.
//
// This must be run while "pip" is installed in the VirtualEnv.
func (e *Env) getPEP425Tags(c context.Context) ([]*vpython.Pep425Tag, error) {
	// This script will return a list of 3-entry lists:
	// [0]: version (e.g., "cp27")
	// [1]: abi (e.g., "cp27mu", "none")
	// [2]: arch (e.g., "x86_64", "armv7l", "any")
	const script = `import json;` +
		`import pip.pep425tags;` +
		`import sys;` +
		`sys.stdout.write(json.dumps(pip.pep425tags.get_supported()))`
	type pep425TagEntry []string

	cmd := e.Interpreter().IsolatedCommand(c, "-c", script)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	attachOutputForLogging(c, logging.Debug, cmd)
	if err := cmd.Run(); err != nil {
		return nil, errors.Annotate(err).Reason("failed to get PEP425 tags").Err()
	}

	var tagEntries []pep425TagEntry
	if err := json.Unmarshal(stdout.Bytes(), &tagEntries); err != nil {
		return nil, errors.Annotate(err).Reason("failed to unmarshal PEP425 tag output: %(output)s").
			D("output", stdout.String()).
			Err()
	}

	tags := make([]*vpython.Pep425Tag, len(tagEntries))
	for i, te := range tagEntries {
		if len(te) != 3 {
			return nil, errors.Reason("invalid PEP425 tag entry: %(entry)v").
				D("entry", te).
				D("index", i).
				Err()
		}

		tags[i] = &vpython.Pep425Tag{
			Version: te[0],
			Abi:     te[1],
			Arch:    te[2],
		}
	}

	// If we're Debug-logging, calculate and display the tags that were probed.
	if logging.IsLogging(c, logging.Debug) {
		tagStr := make([]string, len(tags))
		for i, t := range tags {
			tagStr[i] = t.TagString()
		}
		logging.Debugf(c, "Loaded PEP425 tags: [%s]", strings.Join(tagStr, ", "))
	}

	return tags, nil
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

	cmd := e.Interpreter().IsolatedCommand(c,
		"-m", "pip",
		"install",
		"--use-wheel",
		"--compile",
		"--no-index",
		"--find-links", pkgDir,
		"--requirement", reqPath)
	attachOutputForLogging(c, logging.Debug, cmd)
	if err := cmd.Run(); err != nil {
		return errors.Annotate(err).Reason("failed to install wheels").Err()
	}
	return nil
}

func (e *Env) finalize(c context.Context) error {
	// Uninstall "pip" and "wheel", preventing (easy) augmentation of the
	// environment.
	if !e.Config.testPreserveInstallationCapability {
		cmd := e.Interpreter().IsolatedCommand(c,
			"-m", "pip",
			"uninstall",
			"--quiet",
			"--yes",
			"pip", "wheel")
		attachOutputForLogging(c, logging.Debug, cmd)
		if err := cmd.Run(); err != nil {
			return errors.Annotate(err).Reason("failed to install wheels").Err()
		}
	}

	// Change all files to read-only, except:
	// - Our root directory, which must be writable in order to update our
	//   completion flag.
	// - Our environment stamp, which must be trivially re-writable.
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

// touchCompleteFlagNonBlocking touches the complete flag, creating it and/or
// updating its timestamp.
//
// If the lock for this VirtualEnv is already held, we will fail with
// fslock.ErrLockHeld.
func (e *Env) touchCompleteFlagNonBlocking() error {
	err := e.withLockNonBlocking(e.touchCompleteFlagLocked)
	if err != nil {
		return errors.Annotate(err).Reason("failed to touch complete flag").Err()
	}
	return err
}

// touchCompleteFlagBlocking touches the complete flag, creating it and/or
// updating its timestamp.
//
// If the lock for this VirtualEnv is already held, we will block until it is
// not and then update the flag.
func (e *Env) touchCompleteFlagBlocking(c context.Context) error {
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
		if err := e.touchCompleteFlagBlocking(c); err != nil {
			errors.Log(c, errors.Annotate(err).Reason("failed to refresh timestamp").Err())
			cancelFunc()
			return
		}
	}
}

func attachOutputForLogging(c context.Context, l logging.Level, cmd *exec.Cmd) {
	if logging.IsLogging(c, logging.Info) {
		logging.Infof(c, "Running Python command (cwd=%s): %s",
			cmd.Dir, strings.Join(cmd.Args, " "))
	}

	if logging.IsLogging(c, l) {
		if cmd.Stdout == nil {
			cmd.Stdout = os.Stdout
		}
		if cmd.Stderr == nil {
			cmd.Stderr = os.Stderr
		}
	}
}
