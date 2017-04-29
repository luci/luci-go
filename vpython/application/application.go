// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package application

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/maruel/subcommands"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/vpython"
	vpythonAPI "github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/spec"
	"github.com/luci/luci-go/vpython/venv"

	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/system/environ"
	"github.com/luci/luci-go/common/system/exitcode"
	"github.com/luci/luci-go/common/system/filesystem"
)

const (
	// VirtualEnvRootENV is an environment variable that, if set, will be used
	// as the default VirtualEnv root.
	//
	// This value overrides the default (~/.vpython), but can be overridden by the
	// "-root" flag.
	//
	// Like "-root", if this value is present but empty, a tempdir will be used
	// for the VirtualEnv root.
	VirtualEnvRootENV = "VPYTHON_VIRTUALENV_ROOT"
)

// ReturnCodeError is an error wrapping a return code value.
type ReturnCodeError int

func (err ReturnCodeError) Error() string {
	return fmt.Sprintf("python interpreter returned non-zero error: %d", err)
}

// VerificationFunc is a function used in environment verification.
//
// VerificationFunc will be invoked with a PackageLoader and an Environment to
// use for verification.
type VerificationFunc func(context.Context, string, venv.PackageLoader, *vpythonAPI.Environment)

// Config is an application's default configuration.
type Config struct {
	// PackageLoader is the package loader to use.
	PackageLoader venv.PackageLoader

	// VENVPackage is the VirtualEnv package to use for bootstrap generation.
	VENVPackage vpythonAPI.Spec_Package

	// PruneThreshold, if > 0, is the maximum age of a VirtualEnv before it
	// becomes candidate for pruning. If <= 0, no pruning will be performed.
	//
	// See venv.Config's PruneThreshold.
	PruneThreshold time.Duration
	// MaxPrunesPerSweep, if > 0, is the maximum number of VirtualEnv that should
	// be pruned passively. If <= 0, no limit will be applied.
	//
	// See venv.Config's MaxPrunesPerSweep.
	MaxPrunesPerSweep int

	// MaxScriptPathLen, if > 0, is the maximum generated script path lengt. If
	// a generated script is expected to exist longer than this, we will error.
	//
	// See venv.Config's MaxScriptPathLen.
	MaxScriptPathLen int

	// Verification, if not nil, is the generator function to use for environment
	// verification. It will invoke the supplied VerificationFunc once for each
	// independent environment.
	//
	// Verification may terminate early if the Context is cancelled.
	Verification func(context.Context, VerificationFunc)

	// opts is the set of configured options.
	opts vpython.Options
}

func (cfg *Config) mainDev(c context.Context, args []string) error {
	app := cli.Application{
		Name:  "vpython",
		Title: "VirtualEnv Python Bootstrap (Development Mode)",
		Context: func(context.Context) context.Context {
			// Discard the entry Context and use the one passed to us.
			c := c

			// Install our Config instance into the Context.
			return withConfig(c, cfg)
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			subcommandInstall,
			subcommandVerify,
		},
	}

	return ReturnCodeError(subcommands.Run(&app, args))
}

func (cfg *Config) mainImpl(c context.Context, args []string) error {
	logConfig := logging.Config{
		Level: logging.Error,
	}

	env := environ.System()
	cfg.opts = vpython.Options{
		EnvConfig: venv.Config{
			BaseDir:           "", // (Determined below).
			MaxHashLen:        6,
			Package:           cfg.VENVPackage,
			PruneThreshold:    cfg.PruneThreshold,
			MaxPrunesPerSweep: cfg.MaxPrunesPerSweep,
			MaxScriptPathLen:  cfg.MaxScriptPathLen,
			Loader:            cfg.PackageLoader,
		},
		WaitForEnv: true,
		Environ:    env,
	}

	// Determine our VirtualEnv base directory.
	if v, ok := env.Get(VirtualEnvRootENV); ok {
		cfg.opts.EnvConfig.BaseDir = v
	} else {
		hdir, err := homedir.Dir()
		if err != nil {
			return errors.Annotate(err).Reason("failed to get user home directory").Err()
		}
		cfg.opts.EnvConfig.BaseDir = filepath.Join(hdir, ".vpython")
	}

	var specPath string
	var devMode bool

	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.BoolVar(&devMode, "dev", devMode,
		"Enter development / subcommand mode (use 'help' for more options).")
	fs.StringVar(&cfg.opts.EnvConfig.Python, "python", cfg.opts.EnvConfig.Python,
		"Path to system Python interpreter to use. Default is found on PATH.")
	fs.StringVar(&cfg.opts.WorkDir, "workdir", cfg.opts.WorkDir,
		"Working directory to run the Python interpreter in. Default is current working directory.")
	fs.StringVar(&cfg.opts.EnvConfig.BaseDir, "root", cfg.opts.EnvConfig.BaseDir,
		"Path to virtual environment root directory. Default is the working directory. "+
			"If explicitly set to empty string, a temporary directory will be used and cleaned up "+
			"on completion.")
	fs.StringVar(&specPath, "spec", specPath,
		"Path to environment specification file to load. Default probes for one.")
	logConfig.AddFlags(fs)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return errors.Annotate(err).Reason("failed to parse flags").Err()
	}
	args = fs.Args()

	c = logConfig.Set(c)

	// If an spec path was manually specified, load and use it.
	if specPath != "" {
		var err error
		if cfg.opts.EnvConfig.Spec, err = spec.Load(specPath); err != nil {
			return errors.Annotate(err).Reason("failed to load specification file (-spec) from: %(path)s").
				D("path", specPath).
				Err()
		}
	}

	// If an empty BaseDir was specified, use a temporary directory and clean it
	// up on completion.
	if cfg.opts.EnvConfig.BaseDir == "" {
		tdir, err := ioutil.TempDir("", "vpython")
		if err != nil {
			return errors.Annotate(err).Reason("failed to create temporary directory").Err()
		}
		defer func() {
			logging.Debugf(c, "Removing temporary directory: %s", tdir)
			if terr := filesystem.RemoveAll(tdir); terr != nil {
				logging.WithError(terr).Warningf(c, "Failed to clean up temporary directory; leaking: %s", tdir)
			}
		}()
		cfg.opts.EnvConfig.BaseDir = tdir
	}

	// Development mode (subcommands).
	if devMode {
		return cfg.mainDev(c, args)
	}

	cfg.opts.Args = args
	if err := vpython.Run(c, cfg.opts); err != nil {
		// If the process failed because of a non-zero return value, return that
		// as our error.
		if rc, has := exitcode.Get(errors.Unwrap(err)); has {
			err = ReturnCodeError(rc)
		}

		return errors.Annotate(err).Err()
	}
	return nil
}

// Main is the main application entry point.
func (cfg *Config) Main(c context.Context) int {
	c = gologger.StdConfig.Use(c)
	c = logging.SetLevel(c, logging.Error)

	return run(c, func(c context.Context) error {
		return cfg.mainImpl(c, os.Args[1:])
	})
}
