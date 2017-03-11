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

	"github.com/maruel/subcommands"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/net/context"
)

// ReturnCodeError is an error wrapping a return code value.
type ReturnCodeError int

func (err ReturnCodeError) Error() string {
	return fmt.Sprintf("python interpreter returned non-zero error: %d", err)
}

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

	// Opts is the set of configured options.
	Opts vpython.Options
}

func (cfg *Config) mainDev(c context.Context) error {
	app := cli.Application{
		Name:  "vpython",
		Title: "VirtualEnv Python Bootstrap (Development Mode)",
		Context: func(context.Context) context.Context {
			// Discard the entry Context and use the one passed to us.
			c := c

			// Install our Config instance into the Context.
			c = withConfig(c, cfg)

			// Drop down to Info level debugging.
			if logging.GetLevel(c) > logging.Info {
				c = logging.SetLevel(c, logging.Info)
			}
			return c
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			subcommandInstall,
		},
	}

	return ReturnCodeError(subcommands.Run(&app, cfg.Opts.Args))
}

func (cfg *Config) mainImpl(c context.Context, args []string) error {
	logConfig := logging.Config{
		Level: logging.Warning,
	}

	hdir, err := homedir.Dir()
	if err != nil {
		return errors.Annotate(err).Reason("failed to get user home directory").Err()
	}

	cfg.Opts = vpython.Options{
		EnvConfig: venv.Config{
			BaseDir:           filepath.Join(hdir, ".vpython"),
			MaxHashLen:        6,
			Package:           cfg.VENVPackage,
			PruneThreshold:    cfg.PruneThreshold,
			MaxPrunesPerSweep: cfg.MaxPrunesPerSweep,
			MaxScriptPathLen:  cfg.MaxScriptPathLen,
			Loader:            cfg.PackageLoader,
		},
		WaitForEnv: true,
		Environ:    environ.System(),
	}
	var specPath string
	var devMode bool

	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.BoolVar(&devMode, "dev", devMode,
		"Enter development / subcommand mode (use 'help' for more options).")
	fs.StringVar(&cfg.Opts.EnvConfig.Python, "python", cfg.Opts.EnvConfig.Python,
		"Path to system Python interpreter to use. Default is found on PATH.")
	fs.StringVar(&cfg.Opts.WorkDir, "workdir", cfg.Opts.WorkDir,
		"Working directory to run the Python interpreter in. Default is current working directory.")
	fs.StringVar(&cfg.Opts.EnvConfig.BaseDir, "root", cfg.Opts.EnvConfig.BaseDir,
		"Path to virtual enviornment root directory. Default is the working directory. "+
			"If explicitly set to empty string, a temporary directory will be used and cleaned up "+
			"on completion.")
	fs.StringVar(&specPath, "spec", specPath,
		"Path to enviornment specification file to load. Default probes for one.")
	logConfig.AddFlags(fs)

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return errors.Annotate(err).Reason("failed to parse flags").Err()
	}
	cfg.Opts.Args = fs.Args()

	c = logConfig.Set(c)

	// If an spec path was manually specified, load and use it.
	if specPath != "" {
		var err error
		if cfg.Opts.EnvConfig.Spec, err = spec.Load(specPath); err != nil {
			return errors.Annotate(err).Reason("failed to load specification file (-spec) from: %(path)s").
				D("path", specPath).
				Err()
		}
	}

	// If an empty BaseDir was specified, use a temporary directory and clean it
	// up on completion.
	if cfg.Opts.EnvConfig.BaseDir == "" {
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
		cfg.Opts.EnvConfig.BaseDir = tdir
	}

	// Development mode (subcommands).
	if devMode {
		return cfg.mainDev(c)
	}

	if err := vpython.Run(c, cfg.Opts); err != nil {
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
	c = logging.SetLevel(c, logging.Warning)

	return run(c, func(c context.Context) error {
		return cfg.mainImpl(c, os.Args[1:])
	})
}
