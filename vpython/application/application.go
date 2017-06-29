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
	"github.com/luci/luci-go/vpython/python"
	"github.com/luci/luci-go/vpython/spec"
	"github.com/luci/luci-go/vpython/venv"

	cipdVersion "github.com/luci/luci-go/cipd/version"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/system/environ"
	"github.com/luci/luci-go/common/system/exitcode"
	"github.com/luci/luci-go/common/system/filesystem"
	"github.com/luci/luci-go/common/system/prober"
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

	// DefaultSpecENV is an enviornment variable that, if set, will be used as the
	// default VirtualEnv spec file if none is provided or found through probing.
	DefaultSpecENV = "VPYTHON_DEFAULT_SPEC"
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

	// RelativePathOverride is a series of forward-slash-delimited paths to
	// directories relative to the "vpython" executable that will be checked
	// for Python targets prior to checking PATH. This allows bundles (e.g., CIPD)
	// that include both the wrapper and a real implementation, to force the
	// wrapper to use the bundled implementation if present.
	//
	// See "github.com/luci/luci-go/common/wrapper/prober.Probe"'s
	// RelativePathOverride member for more information.
	RelativePathOverride []string

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

	// WithVerificationConfig, if not nil, runs the supplied callback with
	// a Config instance to use for verification and the set of default
	// verification scenarios.
	//
	// If nil, verification will only include the validation of the spec protobuf.
	WithVerificationConfig func(context.Context, func(Config, []*vpythonAPI.PEP425Tag) error) error
}

type application struct {
	*Config

	// opts is the set of configured options.
	opts vpython.Options

	help      bool
	devMode   bool
	specPath  string
	logConfig logging.Config
}

func (a *application) mainDev(c context.Context, args []string) error {
	app := cli.Application{
		Name:  "vpython",
		Title: "VirtualEnv Python Bootstrap (Development Mode)",
		Context: func(context.Context) context.Context {
			// Discard the entry Context and use the one passed to us.
			c := c

			// Install our Config instance into the Context.
			return withApplication(c, a)
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			subcommandInstall,
			subcommandVerify,
			subcommandDelete,
		},
	}

	return ReturnCodeError(subcommands.Run(&app, args))
}

func (a *application) addToFlagSet(fs *flag.FlagSet) {
	fs.BoolVar(&a.help, "help", a.help,
		"Display help for 'vpython' top-level arguments.")
	fs.BoolVar(&a.help, "h", a.help,
		"Display help for 'vpython' top-level arguments (same as -help).")
	fs.BoolVar(&a.devMode, "dev", a.devMode,
		"Enter development / subcommand mode (use 'help' for more options).")
	fs.StringVar(&a.opts.EnvConfig.Python, "python", a.opts.EnvConfig.Python,
		"Path to system Python interpreter to use. Default is found on PATH.")
	fs.StringVar(&a.opts.WorkDir, "workdir", a.opts.WorkDir,
		"Working directory to run the Python interpreter in. Default is current working directory.")
	fs.StringVar(&a.opts.EnvConfig.BaseDir, "root", a.opts.EnvConfig.BaseDir,
		"Path to virtual environment root directory. Default is the working directory. "+
			"If explicitly set to empty string, a temporary directory will be used and cleaned up "+
			"on completion.")
	fs.StringVar(&a.specPath, "spec", a.specPath,
		"Path to environment specification file to load. Default probes for one.")

	a.logConfig.AddFlags(fs)
}

func (a *application) mainImpl(c context.Context, argv0 string, args []string) error {
	// Determine our VirtualEnv base directory.
	if v, ok := a.opts.Environ.Get(VirtualEnvRootENV); ok {
		a.opts.EnvConfig.BaseDir = v
	} else {
		hdir, err := homedir.Dir()
		if err != nil {
			return errors.Annotate(err, "failed to get user home directory").Err()
		}
		a.opts.EnvConfig.BaseDir = filepath.Join(hdir, ".vpython")
	}

	// Extract "vpython" arguments and parse them.
	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.SetOutput(os.Stdout) // Python uses STDOUT for help and flag information.

	a.addToFlagSet(fs)
	selfArgs, args := extractFlagsForSet(args, fs)
	if err := fs.Parse(selfArgs); err != nil && err != flag.ErrHelp {
		return errors.Annotate(err, "failed to parse flags").Err()
	}

	// Identify the "self" executable. Use this to construct a "lookPath", which
	// will be used to locate the base Python interpreter.
	lp := lookPath{
		probeBase: prober.Probe{
			RelativePathOverride: a.RelativePathOverride,
		},
		env: a.opts.Environ,
	}
	if err := lp.probeBase.ResolveSelf(argv0); err != nil {
		logging.WithError(err).Warningf(c, "Failed to resolve 'self'")
	}
	a.opts.EnvConfig.LookPathFunc = lp.look

	if a.help {
		return a.showPythonHelp(c, fs, &lp)
	}

	c = a.logConfig.Set(c)

	// If an spec path was manually specified, load and use it.
	if a.specPath != "" {
		var sp vpythonAPI.Spec
		if err := spec.Load(a.specPath, &sp); err != nil {
			return err
		}
		a.opts.EnvConfig.Spec = &sp
	} else if specPath := a.opts.Environ.GetEmpty(DefaultSpecENV); specPath != "" {
		if err := spec.Load(specPath, &a.opts.DefaultSpec); err != nil {
			return errors.Annotate(err, "failed to load default specification file (%s) from %s",
				DefaultSpecENV, specPath).Err()
		}
	}

	// If an empty BaseDir was specified, use a temporary directory and clean it
	// up on completion.
	if a.opts.EnvConfig.BaseDir == "" {
		tdir, err := ioutil.TempDir("", "vpython")
		if err != nil {
			return errors.Annotate(err, "failed to create temporary directory").Err()
		}
		defer func() {
			logging.Debugf(c, "Removing temporary directory: %s", tdir)
			if terr := filesystem.RemoveAll(tdir); terr != nil {
				logging.WithError(terr).Warningf(c, "Failed to clean up temporary directory; leaking: %s", tdir)
			}
		}()
		a.opts.EnvConfig.BaseDir = tdir
	}

	// Development mode (subcommands).
	if a.devMode {
		return a.mainDev(c, args)
	}

	a.opts.Args = args
	if err := vpython.Run(c, a.opts); err != nil {
		// If the process failed because of a non-zero return value, return that
		// as our error.
		if rc, has := exitcode.Get(errors.Unwrap(err)); has {
			err = ReturnCodeError(rc)
		}

		return errors.Annotate(err, "").Err()
	}
	return nil
}

func (a *application) showPythonHelp(c context.Context, fs *flag.FlagSet, lp *lookPath) error {
	self, err := os.Executable()
	if err != nil {
		self = "vpython"
	}
	vers, err := cipdVersion.GetStartupVersion()
	if err == nil && vers.PackageName != "" && vers.InstanceID != "" {
		self = fmt.Sprintf("%s (%s@%s)", self, vers.PackageName, vers.InstanceID)
	}

	fmt.Fprintf(os.Stdout, "Usage of %s:\n", self)
	fs.SetOutput(os.Stdout)
	fs.PrintDefaults()

	i, err := python.Find(c, python.Version{}, lp.look)
	if err != nil {
		return errors.Annotate(err, "could not find Python interpreter for help").Err()
	}

	// Redirect all "--help" to Stdout for consistency.
	fmt.Fprintf(os.Stdout, "\nPython help for %s:\n", i.Python)
	cmd := i.IsolatedCommand(c, "--help")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Run(); err != nil {
		return errors.Annotate(err, "failed to dump Python help from: %s", i.Python).Err()
	}
	return nil
}

// Main is the main application entry point.
func (cfg *Config) Main(c context.Context) int {
	// Implementation of "checkWrapper": if CheckWrapperENV is set, we immediately
	// exit with a non-zero value.
	env := environ.System()
	if wrapperCheck(env) {
		return 1
	}

	c = gologger.StdConfig.Use(c)
	c = logging.SetLevel(c, logging.Error)

	a := application{
		Config: cfg,
		opts: vpython.Options{
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
		},
		logConfig: logging.Config{
			Level: logging.Error,
		},
	}

	return run(c, func(c context.Context) error {
		return a.mainImpl(c, os.Args[0], os.Args[1:])
	})
}
