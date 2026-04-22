// Copyright 2026 The LUCI Authors.
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

package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// inputOptions mixin.

// packageVars holds array of '-pkg-var' command line options.
type packageVars map[string]string

func (vars *packageVars) String() string {
	return "key:value"
}

// Set is called by 'flag' package when parsing command line options.
func (vars *packageVars) Set(value string) error {
	// <key>:<value> pair.
	chunks := strings.SplitN(value, ":", 2)
	if len(chunks) != 2 {
		return makeCLIError("expecting <key>:<value> pair, got %q", value)
	}
	(*vars)[chunks[0]] = chunks[1]
	return nil
}

// inputOptions defines command line arguments that specify where to get data
// for a new package and how to build it.
//
// Subcommands that build packages embed it.
type inputOptions struct {
	// Path to *.yaml file with package definition.
	packageDef string
	vars       packageVars

	// Alternative to 'pkg-def'.
	packageName      string
	inputDir         string
	installMode      pkg.InstallMode
	preserveModTime  bool
	preserveWritable bool

	// Deflate compression level (if [1-9]) or 0 to disable compression.
	//
	// Default is 5.
	compressionLevel int
}

func (opts *inputOptions) registerFlags(f *flag.FlagSet) {
	// Set default vars (e.g. ${platform}). They may be overridden through flags.
	defVars := template.DefaultExpander()
	opts.vars = make(packageVars, len(defVars))
	for k, v := range defVars {
		opts.vars[k] = v
	}

	// Interface to accept package definition file.
	f.StringVar(&opts.packageDef, "pkg-def", "", "A *.yaml file `path` that defines what to put into the package.")
	f.Var(&opts.vars, "pkg-var", "A `key:value` with a variable accessible from package definition file (can be used multiple times).")

	// Interface to accept a single directory (alternative to -pkg-def).
	f.StringVar(&opts.packageName, "name", "", "Package `name` (unused with -pkg-def).")
	f.StringVar(&opts.inputDir, "in", "", "A `path` to a directory with files to package (unused with -pkg-def).")
	f.Var(&opts.installMode, "install-mode",
		"How the package should be installed: \"copy\" or \"symlink\" (unused with -pkg-def).")
	f.BoolVar(&opts.preserveModTime, "preserve-mtime", false,
		"Preserve file's modification time (unused with -pkg-def).")
	f.BoolVar(&opts.preserveWritable, "preserve-writable", false,
		"Preserve file's writable permission bit (unused with -pkg-def).")

	// Options for the builder.
	f.IntVar(&opts.compressionLevel, "compression-level", 5,
		"Deflate compression level [0-9]: 0 - disable, 1 - best speed, 9 - best compression.")
}

// prepareInput processes inputOptions by collecting all files to be added to
// a package and populating builder.Options. Caller is still responsible to fill
// out Output field of Options.
func (opts *inputOptions) prepareInput() (builder.Options, error) {
	empty := builder.Options{}

	if opts.compressionLevel < 0 || opts.compressionLevel > 9 {
		return empty, makeCLIError("invalid -compression-level: must be in [0-9] set")
	}

	// Handle -name and -in if defined. Do not allow -pkg-def in that case, since
	// it provides same information as -name and -in. Note that -pkg-var are
	// ignored, even if defined. There's nothing to apply them to.
	if opts.inputDir != "" {
		if opts.packageName == "" {
			return empty, makeCLIError("missing required flag: -name")
		}
		if opts.packageDef != "" {
			return empty, makeCLIError("-pkg-def and -in can not be used together")
		}

		packageName, err := expandTemplate(opts.packageName)
		if err != nil {
			return empty, err
		}

		// Simply enumerate files in the directory.
		files, err := fs.ScanFileSystem(opts.inputDir, opts.inputDir, nil, fs.ScanOptions{
			PreserveModTime:  opts.preserveModTime,
			PreserveWritable: opts.preserveWritable,
		})
		if err != nil {
			return empty, err
		}
		return builder.Options{
			Input:            files,
			PackageName:      packageName,
			InstallMode:      opts.installMode,
			CompressionLevel: opts.compressionLevel,
		}, nil
	}

	// Handle -pkg-def case. -in is "" (already checked), reject -name.
	if opts.packageDef != "" {
		if opts.packageName != "" {
			return empty, makeCLIError("-pkg-def and -name can not be used together")
		}
		if opts.installMode != "" {
			return empty, makeCLIError("-install-mode is ignored if -pkg-def is used")
		}
		if opts.preserveModTime {
			return empty, makeCLIError("-preserve-mtime is ignored if -pkg-def is used")
		}
		if opts.preserveWritable {
			return empty, makeCLIError("-preserve-writable is ignored if -pkg-def is used")
		}

		// Parse the file, perform variable substitution.
		f, err := os.Open(opts.packageDef)
		if err != nil {
			if os.IsNotExist(err) {
				return empty, cipderr.BadArgument.Apply(errors.Fmt("package definition file is missing: %w", err))
			}
			return empty, cipderr.IO.Apply(errors.Fmt("opening package definition file: %w", err))
		}
		defer f.Close()
		pkgDef, err := builder.LoadPackageDef(f, opts.vars)
		if err != nil {
			return empty, err
		}

		// Scan the file system. Package definition may use path relative to the
		// package definition file itself, so pass its location.
		fmt.Println("Enumerating files to zip...")
		files, err := pkgDef.FindFiles(filepath.Dir(opts.packageDef))
		if err != nil {
			return empty, err
		}
		return builder.Options{
			Input:            files,
			PackageName:      pkgDef.Package,
			VersionFile:      pkgDef.VersionFile(),
			InstallMode:      pkgDef.InstallMode,
			CompressionLevel: opts.compressionLevel,
		}, nil
	}

	// All command line options are missing.
	return empty, makeCLIError("-pkg-def or -name/-in are required")
}
