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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/spec"
	"go.chromium.org/luci/vpython/venv/assets"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
)

// EnvironmentVersion is an environment version string. It must advance each
// time the layout of a VirtualEnv environment changes.
const EnvironmentVersion = "v4"

const siteCustomizePy = "sitecustomize.py"

type SliceFlag []string

func (i *SliceFlag) String() string {
	return fmt.Sprintf("%v", *i)
}

func (i *SliceFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

// Config is the configuration for a managed VirtualEnv.
//
// A VirtualEnv is specified based on its resolved vpython.Spec.
type Config struct {
	// MaxHashLen is the maximum number of hash characters to use in VirtualEnv
	// directory names.
	MaxHashLen int

	// BaseDir is the parent directory of all VirtualEnv.
	BaseDir string

	// SetupEnv if not nil, is the base environment that will be used during
	// VirtualEnv setup operations.
	SetupEnv environ.Env

	// OverrideName overrides the name of the specified VirtualEnv.
	//
	// Because the name is no longer derived from the specification, this will
	// force revalidation and deletion of any existing content if it is not a
	// fully defined and matching VirtualEnv
	OverrideName string

	// Package is the VirtualEnv package to install. It must be non-nil and
	// valid. It will be used if the environment specification doesn't supply an
	// overriding one.
	Package vpython.Spec_Package

	// Map of python version (e.g. "3.8") to a CIPD package for the interpreter.
	// The interpreter will be installed to the vpython root if needed before
	// executing the virtual environment.
	PythonPackages map[string]vpython.Spec_Package

	// Python is the set of Python interpreters to use. If empty, one will be
	// resolved based on the Spec and the current PATH. If more than one, the
	// first interpreter matching the Spec will be used.
	Python SliceFlag

	// LookPathFunc, if not nil, will be used instead of exec.LookPath to find the
	// underlying Python interpreter.
	LookPathFunc python.LookPathFunc

	// Spec is the specification file to use to construct the VirtualEnv. If
	// nil, or if fields are missing, they will be filled in by probing the system
	// PATH.
	Spec *vpython.Spec

	// PruneThreshold, if >0, is the maximum age of a VirtualEnv before it should
	// be pruned. If <= 0, there is no maximum age, so no pruning will be
	// performed.
	PruneThreshold time.Duration
	// MaxPrunesPerSweep applies a limit to the number of items to prune per
	// execution.
	//
	// If <= 0, no limit will be applied.
	MaxPrunesPerSweep int

	// Loader is the PackageLoader instance to use for package resolution and
	// deployment.
	Loader PackageLoader

	// FailIfLocked, if true, means that if a lock is encountered during
	// VirtualEnv operation, the VirtualEnv will exit immediately with an error.
	// If false, the VirtualEnv will block pending the lock's availability.
	FailIfLocked bool

	// si is the system Python interpreter. It is resolved during
	// "resolvePythonInterpreter".
	si *python.Interpreter

	// testLeaveReadWrite, if true, instructs the VirtualEnv setup to leave the
	// directory read/write. This makes it easier to manage, and is safe since it
	// is not a production directory.
	testLeaveReadWrite bool
}

// WithoutWheels returns a clone of cfg that depends on no additional packages.
//
// If cfg is already an empty it will be returned directly.
func (cfg *Config) WithoutWheels() *Config {
	if !cfg.HasWheels() {
		return cfg
	}

	clone := *cfg
	clone.OverrideName = ""
	clone.Spec = clone.Spec.Clone()
	clone.Spec.Wheel = nil
	return &clone
}

// HasWheels returns true if this environment declares wheel dependencies.
func (cfg *Config) HasWheels() bool {
	return cfg.Spec != nil && len(cfg.Spec.Wheel) > 0
}

// resolveRuntime find the python interpreter in ine vpython environment, and
// update the environment runtime.
func (cfg *Config) resolveRuntime(c context.Context, e *vpython.Environment) error {
	if cfg.si != nil && e.Runtime != nil {
		return nil
	}
	if err := cfg.resolvePythonInterpreter(c, e.Spec); err != nil {
		return errors.Annotate(err, "failed to resolve system Python interpreter").Err()
	}

	r, err := cfg.si.GetRuntime(c)
	if err != nil {
		return errors.Annotate(err, "failed to resolve system Python runtime").Err()
	}
	e.Runtime = r
	logging.Debugf(c, "Resolved system Python runtime: %#s", e.Runtime)
	return nil
}

// makeEnv processes the config, validating and, where appropriate, populating
// any components. Upon success, it returns a configured Env instance.
//
// The supplied vpython.Environment is derived externally, and may be nil if
// this is a bootstrapped Environment.
//
// The returned Env instance may or may not actually exist. Setup must be called
// prior to using it.
func (cfg *Config) makeEnv(c context.Context, e *vpython.Environment) (*Env, error) {
	// We MUST have a package loader.
	if cfg.Loader == nil {
		return nil, errors.New("no package loader provided")
	}

	// Resolve our base directory, if one is not supplied.
	if cfg.BaseDir == "" {
		// Use one in a temporary directory.
		cfg.BaseDir = filepath.Join(os.TempDir(), "vpython")
		logging.Debugf(c, "Using tempdir-relative environment root: %s", cfg.BaseDir)
	}
	if err := filesystem.AbsPath(&cfg.BaseDir); err != nil {
		return nil, errors.Annotate(err, "failed to resolve absolute path of base directory").Err()
	}

	// Construct a new, independent Environment for this Env.
	e = e.Clone()
	if cfg.Spec != nil {
		e.Spec = cfg.Spec.Clone()
	}
	if err := spec.NormalizeEnvironment(e); err != nil {
		return nil, errors.Annotate(err, "invalid environment").Err()
	}

	// If the environment doesn't specify a VirtualEnv package (expected), use
	// our default.
	if e.Spec.Virtualenv == nil {
		e.Spec.Virtualenv = &cfg.Package
	}

	if err := cfg.Loader.Resolve(c, e); err != nil {
		return nil, errors.Annotate(err, "failed to resolve packages").Err()
	}

	if err := cfg.resolveRuntime(c, e); err != nil {
		return nil, err
	}

	// Ensure that our base directory exists.
	if err := filesystem.MakeDirs(cfg.BaseDir); err != nil {
		return nil, errors.Annotate(err, "could not create environment root: %s", cfg.BaseDir).Err()
	}

	// Generate our environment name based on the deterministic hash of its
	// fully-resolved specification.
	envName := cfg.OverrideName
	if envName == "" {
		envName = cfg.envNameForSpec(c, e.Spec, e.Runtime)
	}
	env := cfg.envForName(envName, e)
	return env, nil
}

// EnvName returns the VirtualEnv environment name for the environment that cfg
// describes.
func (cfg *Config) envNameForSpec(c context.Context, s *vpython.Spec, rt *vpython.Runtime) string {
	uid := "<unknown>"
	if user, err := currentUID(); err == nil {
		uid = user
	} else {
		logging.Warningf(c, "Unable to find current user ID: %s", err)
	}

	name := spec.Hash(s, rt, EnvironmentVersion,
		string(assets.GetAssetSHA256(siteCustomizePy)), uid)
	if cfg.MaxHashLen > 0 && len(name) > cfg.MaxHashLen {
		name = name[:cfg.MaxHashLen]
	}
	return name
}

// Prune performs a pruning round on the environment set described by this
// Config.
func (cfg *Config) Prune(c context.Context) error {
	if err := prune(c, cfg, nil); err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

// envForName creates an Env for a named directory.
//
// The Environment, e, can be nil; however, code paths that require it may not
// be called.
func (cfg *Config) envForName(name string, e *vpython.Environment) *Env {
	// Env-specific root directory: <BaseDir>/<name>
	venvRoot := filepath.Join(cfg.BaseDir, name)
	binDir := venvBinDir(venvRoot)
	return &Env{
		Config:               cfg,
		Root:                 venvRoot,
		Name:                 name,
		Python:               filepath.Join(binDir, "python"),
		Environment:          e,
		BinDir:               binDir,
		EnvironmentStampPath: filepath.Join(venvRoot, fmt.Sprintf("environment.%s.pb.txt", vpython.Version)),
		lockPath:             filepath.Join(cfg.BaseDir, fmt.Sprintf(".%s.lock", name)),

		completeFlagPath: filepath.Join(venvRoot, "complete.flag"),
	}
}

func (cfg *Config) resolvePythonInterpreter(c context.Context, s *vpython.Spec) error {
	specVers, err := python.ParseVersion(s.PythonVersion)
	if err != nil {
		return errors.Annotate(err, "failed to parse Python version from: %q", s.PythonVersion).Err()
	}

	if len(cfg.Python) == 0 {
		// No explicitly-specified Python path. Search PATH to find one
		// matching the specification.
		if cfg.si, err = python.Find(c, specVers, cfg.LookPathFunc); err != nil {
			return errors.Annotate(err, "could not find Python for: %s", specVers).Err()
		}
	} else {
		// Select the interpreter out of cfg.Python that matches the
		// specification.
		for _, pythonPath := range cfg.Python {
			i := python.Interpreter{Python: pythonPath}
			if err := i.Normalize(); err != nil {
				continue
			}

			iv, err := i.GetVersion(c)
			if err != nil {
				continue
			}
			if specVers.IsSatisfiedBy(iv) {
				cfg.si = &i
				break
			}
		}

		if cfg.si == nil {
			return errors.Reason("none of %v matched specification %s", cfg.Python, specVers).Err()
		}
	}
	if err := cfg.si.Normalize(); err != nil {
		return err
	}

	interpreterVers, err := cfg.si.GetVersion(c)
	if err != nil {
		return errors.Annotate(err, "failed to determine Python version for: %s", cfg.si.Python).Err()
	}
	s.PythonVersion = interpreterVers.String()
	return nil
}

func (cfg *Config) systemInterpreter() *python.Interpreter { return cfg.si }
