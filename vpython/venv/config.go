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

// PackageLoader loads package information from a specification file's Package
// message onto the local system.
type PackageLoader interface {
	// Resolve processes the supplied packages, updating their fields to their
	// resolved values. Resolved packages must fully specify the package instance
	// that is being deployed.
	//
	// If needed, resolution may use the supplied root path as a persistent
	// working directory. This path may not exist; Resolve is responsible for
	// creating it if needed.
	//
	// root, if used, must be safe for concurrent use.
	Resolve(c context.Context, root string, packages []*vpython.Spec_Package) error

	// Ensure installs the supplied packages into root.
	//
	// The packages will have been previously resolved via Resolve.
	Ensure(c context.Context, root string, packages []*vpython.Spec_Package) error
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

	// LoaderResolveRoot is the common persistent root directory to use for the
	// package loader's package resolution. This must be safe for concurrent
	// use.
	//
	// Each VirtualEnv will have its own loader destination directory (root).
	// However, that root is homed in a directory that is named after the resolved
	// packages. LoaderResolveRoot is used during setup to resolve those package
	// values, and therefore can't re-use the VirtualEnv root.
	LoaderResolveRoot string

	// MaxScriptPathLen, if >0, is the maximum allowed VirutalEnv path length.
	// If set, and if the VirtualEnv is configured to be installed at a path
	// greater than this, Env will fail.
	//
	// This can be used to enforce "shebang" length limits, whereupon generated
	// VirtualEnv scripts may be generated with a "shebang" (#!) line longer than
	// what is allowed by the operating system.
	MaxScriptPathLen int

	// testPreserveInstallationCapability is a testing parameter. If true, the
	// VirtualEnv's ability to install will be preserved after the setup. This is
	// used by the test whell generation bootstrap code.
	testPreserveInstallationCapability bool

	// testLeaveReadWrite, if true, instructs the VirtualEnv setup to leave the
	// directory read/write. This makes it easier to manage, and is safe since it
	// is not a production directory.
	testLeaveReadWrite bool
}

// setupEnv processes the config, validating and, where appropriate, populating
// any components. Upon success, it returns a configured Env instance.
//
// The returned Env instance may or may not actually exist. Setup must be called
// prior to using it.
func (cfg *Config) setupEnv(c context.Context) (*Env, error) {
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

	// Determine our common loader root.
	if cfg.LoaderResolveRoot == "" {
		cfg.LoaderResolveRoot = filepath.Join(cfg.BaseDir, ".package_loader")
	}

	// Ensure and normalize our specification file.
	if cfg.Spec == nil {
		cfg.Spec = &vpython.Spec{}
	}
	if err := spec.Normalize(cfg.Spec); err != nil {
		return nil, errors.Annotate(err).Reason("invalid specification").Err()
	}

	// Choose our VirtualEnv package.
	if cfg.Spec.Virtualenv == nil {
		cfg.Spec.Virtualenv = &cfg.Package
	}

	if err := cfg.resolvePackages(c); err != nil {
		return nil, errors.Annotate(err).Reason("failed to resolve packages").Err()
	}

	if err := cfg.resolvePythonInterpreter(c); err != nil {
		return nil, errors.Annotate(err).Reason("failed to resolve system Python interpreter").Err()
	}

	// Generate our enviornment name based on the deterministic hash of its
	// fully-resolved specification.
	envName := spec.Hash(cfg.Spec)
	if cfg.MaxHashLen > 0 && len(envName) > cfg.MaxHashLen {
		envName = envName[:cfg.MaxHashLen]
	}
	return cfg.envForName(envName), nil
}

// Prune performs a pruning round on the environment set described by this
// Config.
func (cfg *Config) Prune(c context.Context) error {
	if err := prune(c, cfg, ""); err != nil {
		return errors.Annotate(err).Err()
	}
	return nil
}

// envForExisting creates an Env for a named directory.
func (cfg *Config) envForName(name string) *Env {
	// Env-specific root directory: <BaseDir>/<name>
	venvRoot := filepath.Join(cfg.BaseDir, name)
	binDir := venvBinDir(venvRoot)
	return &Env{
		Config:   cfg,
		Root:     venvRoot,
		Python:   filepath.Join(binDir, "python"),
		BinDir:   binDir,
		SpecPath: filepath.Join(venvRoot, "enviornment.pb.txt"),

		name:             name,
		lockPath:         filepath.Join(cfg.BaseDir, fmt.Sprintf(".%s.lock", name)),
		completeFlagPath: filepath.Join(venvRoot, "complete.flag"),
	}
}

func (cfg *Config) resolvePackages(c context.Context) error {
	// Create a single package list. Our VirtualEnv will be index 0 (need
	// this so we can back-port it into its VirtualEnv property).
	packages := make([]*vpython.Spec_Package, 1, 1+len(cfg.Spec.Wheel))
	packages[0] = cfg.Spec.Virtualenv
	packages = append(packages, cfg.Spec.Wheel...)

	// Resolve our packages. Because we're using pointers, the in-place
	// updating will update the actual spec file!
	if err := cfg.Loader.Resolve(c, cfg.LoaderResolveRoot, packages); err != nil {
		return errors.Annotate(err).Reason("failed to resolve packages").Err()
	}
	return nil
}

func (cfg *Config) resolvePythonInterpreter(c context.Context) error {
	specVers, err := python.ParseVersion(cfg.Spec.PythonVersion)
	if err != nil {
		return errors.Annotate(err).Reason("failed to parse Python version from: %(value)q").
			D("value", cfg.Spec.PythonVersion).
			Err()
	}

	var i *python.Interpreter
	if cfg.Python == "" {
		// No explicitly-specified Python path. Determine one based on the
		// specification.
		if i, err = python.Find(c, specVers); err != nil {
			return errors.Annotate(err).Reason("could not find Python for: %(vers)s").
				D("vers", specVers).
				Err()
		}
		cfg.Python = i.Python
	} else {
		i = &python.Interpreter{
			Python: cfg.Python,
		}
	}

	// Confirm that the version of the interpreter matches that which is
	// expected.
	interpreterVers, err := i.GetVersion(c)
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
	cfg.Spec.PythonVersion = interpreterVers.String()

	// Resolve to absolute path.
	if err := filesystem.AbsPath(&cfg.Python); err != nil {
		return errors.Annotate(err).Reason("could not get absolute path for: %(python)s").
			D("python", cfg.Python).
			Err()
	}
	logging.Debugf(c, "Resolved system Python interpreter (%s): %s", cfg.Spec.PythonVersion, cfg.Python)
	return nil
}

func (cfg *Config) systemInterpreter() *python.Command {
	if cfg.Python == "" {
		return nil
	}
	i := python.Interpreter{
		Python: cfg.Python,
	}
	cmd := i.Command()
	cmd.Isolated = true
	return cmd
}
