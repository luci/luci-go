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

	"go.chromium.org/luci/vpython"
	vpythonAPI "go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/spec"
	"go.chromium.org/luci/vpython/venv"

	cipdVersion "go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/system/prober"
)

const (
	// VirtualEnvRootENV is an environment variable that, if set, will be used
	// as the default VirtualEnv root.
	//
	// This value overrides the default (~/.vpython-root), but can be overridden
	// by the "-vpython-root" flag.
	//
	// Like "-vpython-root", if this value is present but empty, a tempdir will be
	// used for the VirtualEnv root.
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

	// SpecLoader is a spec.Loader to use when loading "vpython" specifications.
	//
	// The zero value is a valid default loader. Users may specialize it by
	// populating some or all of its fields.
	SpecLoader spec.Loader

	// VENVPackage is the VirtualEnv package to use for bootstrap generation.
	VENVPackage vpythonAPI.Spec_Package

	// BaseWheels is the set of wheels to include in the spec. These will always
	// be merged into the runtime spec and normalized, such that any duplicate
	// wheels will be deduplicated.
	BaseWheels []*vpythonAPI.Spec_Package

	// RelativePathOverride is a series of forward-slash-delimited paths to
	// directories relative to the "vpython" executable that will be checked
	// for Python targets prior to checking PATH. This allows bundles (e.g., CIPD)
	// that include both the wrapper and a real implementation, to force the
	// wrapper to use the bundled implementation if present.
	//
	// See "go.chromium.org/luci/common/wrapper/prober.Probe"'s
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

	// Bypass, if true, instructs vpython to completely bypass VirtualEnv
	// bootstrapping and execute with the local system interpreter.
	Bypass bool

	// MaxScriptPathLen, if > 0, is the maximum generated script path lengt. If
	// a generated script is expected to exist longer than this, we will error.
	//
	// See venv.Config's MaxScriptPathLen.
	MaxScriptPathLen int

	// DefaultVerificationTags is the default set of PEP425 tags to verify a
	// specification against.
	//
	// An individual specification can override this by specifying its own
	// verification tag set.
	DefaultVerificationTags []*vpythonAPI.PEP425Tag
}

type application struct {
	*Config

	// opts is the set of configured options.
	opts vpython.Options

	help      bool
	toolMode  bool
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

	fs.BoolVar(&a.toolMode, "vpython-tool", a.toolMode,
		"Enter tooling subcommand mode (use 'help' subcommand for details).")
	fs.StringVar(&a.opts.EnvConfig.Python, "vpython-interpreter", a.opts.EnvConfig.Python,
		"Path to system Python interpreter to use. Default is found on PATH.")
	fs.StringVar(&a.opts.EnvConfig.BaseDir, "vpython-root", a.opts.EnvConfig.BaseDir,
		"Path to virtual environment root directory. Default is the working directory. "+
			"If explicitly set to empty string, a temporary directory will be used and cleaned up "+
			"on completion.")
	fs.StringVar(&a.specPath, "vpython-spec", a.specPath,
		"Path to environment specification file to load. Default probes for one.")

	// TODO: These flags are deprecated, and are left in for transition. They will
	// go away in the future.
	fs.StringVar(&a.specPath, "spec", a.specPath,
		"Path to environment specification file to load. Default probes for one. (DEPRECATED, use '-vpython-spec').")
	// XXX: END deprecated flags.

	a.logConfig.AddFlags(fs)
}

func (a *application) mainImpl(c context.Context, argv0 string, args []string) error {
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

	// Determine our VirtualEnv base directory.
	if v, ok := a.opts.Environ.Get(VirtualEnvRootENV); ok {
		a.opts.EnvConfig.BaseDir = v
	} else {
		hdir, err := homedir.Dir()
		if err != nil {
			return errors.Annotate(err, "failed to get user home directory").Err()
		}
		a.opts.EnvConfig.BaseDir = filepath.Join(hdir, ".vpython-root")
	}

	// Extract "vpython" arguments and parse them.
	fs := flag.NewFlagSet("", flag.ExitOnError)
	fs.SetOutput(os.Stdout) // Python uses STDOUT for help and flag information.

	a.addToFlagSet(fs)
	selfArgs, args := extractFlagsForSet(args, fs)
	if err := fs.Parse(selfArgs); err != nil && err != flag.ErrHelp {
		return errors.Annotate(err, "failed to parse flags").Err()
	}

	if a.help {
		return a.showPythonHelp(c, fs, &lp)
	}

	c = a.logConfig.Set(c)

	// If a spec path was manually specified, load and use it.
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

	// If we're bypassing "vpython", run Python directly.
	if a.Bypass {
		return a.runDirect(c, args, &lp)
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
	if a.toolMode {
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
		return errors.Annotate(err, "could not find Python interpreter").Err()
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

func (a *application) runDirect(c context.Context, args []string, lp *lookPath) error {
	var version python.Version
	if s := a.opts.EnvConfig.Spec; s != nil {
		var err error
		if version, err = python.ParseVersion(s.PythonVersion); err != nil {
			return errors.Annotate(err, "could not parse Python version from: %q", s.PythonVersion).Err()
		}
	}
	i, err := python.Find(c, version, lp.look)
	if err != nil {
		return errors.Annotate(err, "could not find Python interpreter").Err()
	}

	logging.Infof(c, "Directly executing Python command with %v: %v", i.Python, args)
	return vpython.Exec(c, i, args, a.opts.Environ, "", nil)
}

// Main is the main application entry point.
func (cfg *Config) Main(c context.Context, argv []string, env environ.Env) int {
	if len(argv) == 0 {
		panic("zero-length argument slice")
	}

	// Implementation of "checkWrapper": if CheckWrapperENV is set, we immediately
	// exit with a non-zero value.
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
			BaseWheels: cfg.BaseWheels,
			WaitForEnv: true,
			SpecLoader: cfg.SpecLoader,
			Environ:    env,
		},
		logConfig: logging.Config{
			Level: logging.Error,
		},
	}

	return run(c, func(c context.Context) error {
		return a.mainImpl(c, argv[0], argv[1:])
	})
}
