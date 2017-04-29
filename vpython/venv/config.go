// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/python"
	"github.com/luci/luci-go/vpython/spec"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/system/filesystem"

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

	// Package is the VirtualEnv package to install. It must be non-nil and
	// valid. It will be used if the environment specification doesn't supply an
	// overriding one.
	Package vpython.Spec_Package

	// Python is the Python interpreter to use. If empty, one will be resolved
	// based on the Spec and the current PATH.
	Python string

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
		return nil, errors.Annotate(err).Reason("failed to resolve absolute path of base directory").Err()
	}

	// Enforce maximum path length.
	if cfg.MaxScriptPathLen > 0 {
		if longestPath := longestGeneratedScriptPath(cfg.BaseDir); longestPath != "" {
			longestPathLen := utf8.RuneCountInString(longestPath)
			if longestPathLen > cfg.MaxScriptPathLen {
				return nil, errors.Reason("expected deepest path length (%(len)d) exceeds threshold (%(threshold)d)").
					D("len", longestPathLen).
					D("threshold", cfg.MaxScriptPathLen).
					D("longestPath", longestPath).
					Err()
			}
		}
	}

	if err := filesystem.MakeDirs(cfg.BaseDir); err != nil {
		return nil, errors.Annotate(err).Reason("could not create environment root: %(root)s").
			D("root", cfg.BaseDir).
			Err()
	}

	// Ensure and normalize our specification file.
	if cfg.Spec == nil {
		cfg.Spec = &vpython.Spec{}
	} else {
		cfg.Spec = cfg.Spec.Clone()
	}
	if err := spec.Normalize(cfg.Spec, &cfg.Package); err != nil {
		return nil, errors.Annotate(err).Reason("invalid specification").Err()
	}

	// Choose our VirtualEnv package.
	if cfg.Spec.Virtualenv == nil {
		cfg.Spec.Virtualenv = &cfg.Package
	}

	// Construct a new, independent Enviornment for this Env.
	e = e.Clone()
	e.Spec = cfg.Spec.Clone()

	if err := cfg.Loader.Resolve(c, e); err != nil {
		return nil, errors.Annotate(err).Reason("failed to resolve packages").Err()
	}

	if err := cfg.resolvePythonInterpreter(c, e.Spec); err != nil {
		return nil, errors.Annotate(err).Reason("failed to resolve system Python interpreter").Err()
	}

	// Generate our environment name based on the deterministic hash of its
	// fully-resolved specification.
	return cfg.envForName(cfg.envNameForSpec(e.Spec), e), nil
}

// EnvName returns the VirtualEnv environment name for the environment that cfg
// describes.
func (cfg *Config) envNameForSpec(s *vpython.Spec) string {
	name := spec.Hash(s)
	if cfg.MaxHashLen > 0 && len(name) > cfg.MaxHashLen {
		name = name[:cfg.MaxHashLen]
	}
	return name
}

// Prune performs a pruning round on the environment set described by this
// Config.
func (cfg *Config) Prune(c context.Context) error {
	if err := prune(c, cfg, ""); err != nil {
		return errors.Annotate(err).Err()
	}
	return nil
}

// envForName creates an Env for a named directory.
func (cfg *Config) envForName(name string, e *vpython.Environment) *Env {
	// Env-specific root directory: <BaseDir>/<name>
	venvRoot := filepath.Join(cfg.BaseDir, name)
	binDir := venvBinDir(venvRoot)
	return &Env{
		Config:               cfg,
		Root:                 venvRoot,
		Python:               filepath.Join(binDir, "python"),
		Environment:          e,
		BinDir:               binDir,
		EnvironmentStampPath: filepath.Join(venvRoot, fmt.Sprintf("environment.%s.pb.txt", vpython.Version)),

		name:             name,
		lockPath:         filepath.Join(cfg.BaseDir, fmt.Sprintf(".%s.lock", name)),
		completeFlagPath: filepath.Join(venvRoot, "complete.flag"),
	}
}

func (cfg *Config) resolvePythonInterpreter(c context.Context, s *vpython.Spec) error {
	specVers, err := python.ParseVersion(s.PythonVersion)
	if err != nil {
		return errors.Annotate(err).Reason("failed to parse Python version from: %(value)q").
			D("value", s.PythonVersion).
			Err()
	}

	if cfg.Python == "" {
		// No explicitly-specified Python path. Determine one based on the
		// specification.
		if cfg.si, err = python.Find(c, specVers); err != nil {
			return errors.Annotate(err).Reason("could not find Python for: %(vers)s").
				D("vers", specVers).
				Err()
		}
		cfg.Python = cfg.si.Python
	} else {
		cfg.si = &python.Interpreter{
			Python: cfg.Python,
		}
	}

	// Confirm that the version of the interpreter matches that which is
	// expected.
	interpreterVers, err := cfg.si.GetVersion(c)
	if err != nil {
		return errors.Annotate(err).Reason("failed to determine Python version for: %(python)s").
			D("python", cfg.Python).
			Err()
	}
	if !specVers.IsSatisfiedBy(interpreterVers) {
		return errors.Reason("supplied Python version (%(supplied)s) doesn't match specification (%(spec)s)").
			D("supplied", interpreterVers).
			D("spec", specVers).
			Err()
	}
	s.PythonVersion = interpreterVers.String()

	// Resolve to absolute path.
	if err := filesystem.AbsPath(&cfg.Python); err != nil {
		return errors.Annotate(err).Reason("could not get absolute path for: %(python)s").
			D("python", cfg.Python).
			Err()
	}
	logging.Debugf(c, "Resolved system Python interpreter (%s): %s", s.PythonVersion, cfg.Python)
	return nil
}

func (cfg *Config) systemInterpreter() *python.Interpreter { return cfg.si }
