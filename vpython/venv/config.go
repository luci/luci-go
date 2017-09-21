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
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/spec"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"

	"golang.org/x/net/context"
)

// Config is the configuration for a managed VirtualEnv.
//
// A VirtualEnv is specified based on its resolved vpython.Spec.
type Config struct {
	// MaxHashLen is the maximum number of hash characters to use in VirtualEnv
	// directory names.
	MaxHashLen int

	// BaseDir is the parent directory of all VirtualEnv.
	BaseDir string

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

	// Python is the Python interpreter to use. If empty, one will be resolved
	// based on the Spec and the current PATH.
	Python string

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

	// MaxScriptPathLen, if >0, is the maximum allowed VirutalEnv path length.
	// If set, and if the VirtualEnv is configured to be installed at a path
	// greater than this, Env will fail.
	//
	// This can be used to enforce "shebang" length limits, whereupon generated
	// VirtualEnv scripts may be generated with a "shebang" (#!) line longer than
	// what is allowed by the operating system.
	MaxScriptPathLen int

	// si is the system Python interpreter. It is resolved during
	// "resolvePythonInterpreter".
	si *python.Interpreter

	// rt is the resolved Python runtime.
	rt vpython.Runtime

	// testPreserveInstallationCapability is a testing parameter. If true, the
	// VirtualEnv's ability to install will be preserved after the setup. This is
	// used by the test whell generation bootstrap code.
	testPreserveInstallationCapability bool

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

	// Enforce maximum path length.
	if cfg.MaxScriptPathLen > 0 {
		if longestPath := longestGeneratedScriptPath(cfg.BaseDir); longestPath != "" {
			longestPathLen := utf8.RuneCountInString(longestPath)
			if longestPathLen > cfg.MaxScriptPathLen {
				return nil, errors.Reason(
					"expected deepest path length (%d) exceeds threshold (%d)",
					longestPathLen, cfg.MaxScriptPathLen,
				).InternalReason("longestPath(%q)", longestPath).Err()
			}
		}
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

	if err := cfg.resolvePythonInterpreter(c, e.Spec); err != nil {
		return nil, errors.Annotate(err, "failed to resolve system Python interpreter").Err()
	}
	e.Runtime.Path = cfg.si.Python
	e.Runtime.Version = e.Spec.PythonVersion

	var err error
	if e.Runtime.Hash, err = cfg.si.Hash(); err != nil {
		return nil, err
	}
	logging.Debugf(c, "Resolved system Python runtime (%s @ %s): %s",
		e.Runtime.Version, e.Runtime.Hash, e.Runtime.Path)

	// Ensure that our base directory exists.
	if err := filesystem.MakeDirs(cfg.BaseDir); err != nil {
		return nil, errors.Annotate(err, "could not create environment root: %s", cfg.BaseDir).Err()
	}

	// Generate our environment name based on the deterministic hash of its
	// fully-resolved specification.
	envName := cfg.OverrideName
	if envName == "" {
		envName = cfg.envNameForSpec(e.Spec, e.Runtime)
	}
	env := cfg.envForName(envName, e)
	return env, nil
}

// EnvName returns the VirtualEnv environment name for the environment that cfg
// describes.
func (cfg *Config) envNameForSpec(s *vpython.Spec, rt *vpython.Runtime) string {
	name := spec.Hash(s, rt, EnvironmentVersion)
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

	if cfg.Python == "" {
		// No explicitly-specified Python path. Determine one based on the
		// specification.
		if cfg.si, err = python.Find(c, specVers, cfg.LookPathFunc); err != nil {
			return errors.Annotate(err, "could not find Python for: %s", specVers).Err()
		}
		cfg.Python = cfg.si.Python
	} else {
		cfg.si = &python.Interpreter{
			Python: cfg.Python,
		}
	}
	if err := cfg.si.Normalize(); err != nil {
		return err
	}

	// Confirm that the version of the interpreter matches that which is
	// expected.
	interpreterVers, err := cfg.si.GetVersion(c)
	if err != nil {
		return errors.Annotate(err, "failed to determine Python version for: %s", cfg.Python).Err()
	}
	if !specVers.IsSatisfiedBy(interpreterVers) {
		return errors.Reason("supplied Python version (%s) doesn't match specification (%s)", interpreterVers, specVers).Err()
	}
	s.PythonVersion = interpreterVers.String()

	// Resolve to absolute path.
	if err := filesystem.AbsPath(&cfg.Python); err != nil {
		return errors.Annotate(err, "could not get absolute path for: %s", cfg.Python).Err()
	}
	return nil
}

func (cfg *Config) systemInterpreter() *python.Interpreter { return cfg.si }
