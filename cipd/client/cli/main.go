// Copyright 2014 The LUCI Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maruel/subcommands"
	"golang.org/x/term"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/signals"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/deployer"
	"go.chromium.org/luci/cipd/client/cipd/digests"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/client/cipd/ui"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// TODO(vadimsh): Add some tests.

// This is a killswitch that disables the fancy terminal progress bar UI in case
// it has some fatal bugs or a user has aversion towards it.
//
// Note that cipd.Client doesn't know anything about the UI implementation and
// thus this env var is defined here rather than in cipd/client.go like other
// env vars.
const envSimpleTerminalUI = "CIPD_SIMPLE_TERMINAL_UI"

func expandTemplate(tmpl string) (pkg string, err error) {
	pkg, err = template.DefaultExpander().Expand(tmpl)
	if err != nil {
		err = cliErrorTag.Apply(err)
	}
	return
}

// Parameters carry default configuration values for a CIPD CLI client.
type Parameters struct {
	// DefaultAuthOptions provide default values for authentication related
	// options (most notably SecretsDir: a directory with token cache).
	DefaultAuthOptions auth.Options

	// ServiceURL is a backend URL to use by default.
	ServiceURL string
}

////////////////////////////////////////////////////////////////////////////////
// Common subcommand functions.

// pinInfo contains information about single package pin inside some site root,
// or an error related to it. It is passed through channels when running batch
// operations and dumped to JSON results file in doneWithPins.
type pinInfo struct {
	// Pkg is package name. Always set.
	Pkg string `json:"package"`
	// Pin is not nil if pin related operation succeeded. It contains instanceID.
	Pin *common.Pin `json:"pin,omitempty"`
	// Platform is set by 'ensure-file-verify' to a platform for this pin.
	Platform string `json:"platform,omitempty"`
	// Tracking is what ref is being tracked by that package in the site root.
	Tracking string `json:"tracking,omitempty"`
	// Error is not empty if pin related operation failed. Pin is nil in that case.
	Error string `json:"error,omitempty"`
	// ErrorCode is an enumeration with possible error conditions.
	ErrorCode cipderr.Code `json:"error_code,omitempty"`
	// ErrorDetails are structured error details.
	ErrorDetails *cipderr.Details `json:"error_details,omitempty"`

	// The original annotated error.
	err error `json:"-"`
}

type instanceInfoWithRefs struct {
	cipd.InstanceInfo
	Refs []string `json:"refs,omitempty"`
}

// instancesOutput defines JSON format of 'cipd instances' output.
type instancesOutput struct {
	Instances []instanceInfoWithRefs `json:"instances"`
}

// cipdSubcommand is a base of all CIPD subcommands. It defines some common
// flags, such as logging and JSON output parameters.
type cipdSubcommand struct {
	subcommands.CommandRunBase

	jsonOutput string
	logConfig  logging.Config

	// TODO(dnj): Remove "verbose" flag once all current invocations of it are
	// cleaned up and rolled out, as it is now deprecated in favor of "logConfig".
	verbose bool
}

// ModifyContext implements cli.ContextModificator.
func (c *cipdSubcommand) ModifyContext(ctx context.Context) context.Context {
	if c.verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	} else {
		ctx = c.logConfig.Set(ctx)
	}

	// Give a lever to turn off the fancy UI if necessary.
	useSimpleUI := environ.FromCtx(ctx).Get(envSimpleTerminalUI) == "1"

	// If writing to a real terminal (rather than redirecting to a file) and not
	// running at a non-default logging level, use a fancy UI with progress bars.
	// It is more human readable, but doesn't preserve details of all operations
	// in the terminal output.
	if !useSimpleUI && logging.GetLevel(ctx) == logging.Info && term.IsTerminal(int(os.Stderr.Fd())) {
		ctx = ui.SetImplementation(ctx, &ui.FancyImplementation{Out: os.Stderr})
	}

	return ctx
}

// registerBaseFlags registers common flags used by all subcommands.
func (c *cipdSubcommand) registerBaseFlags() {
	// Minimum default logging level is Info. This accommodates subcommands that
	// don't explicitly set the log level, resulting in the zero value (Debug).
	if c.logConfig.Level < logging.Info {
		c.logConfig.Level = logging.Info
	}

	c.Flags.StringVar(&c.jsonOutput, "json-output", "", "A `path` to write operation results to.")
	c.Flags.BoolVar(&c.verbose, "verbose", false, "Enable more logging (deprecated, use -log-level=debug).")
	c.logConfig.AddFlags(&c.Flags)
}

// checkArgs checks command line args.
//
// It ensures all required positional and flag-like parameters are set.
// Returns true if they are, or false (and prints to stderr) if not.
func (c *cipdSubcommand) checkArgs(args []string, minPosCount, maxPosCount int) bool {
	// Check number of expected positional arguments.
	if maxPosCount == 0 && len(args) != 0 {
		c.printError(makeCLIError("unexpected arguments %v", args))
		return false
	}
	if len(args) < minPosCount || (maxPosCount >= 0 && len(args) > maxPosCount) {
		var err error
		if minPosCount == maxPosCount {
			err = makeCLIError("expecting %d positional argument, got %d instead", minPosCount, len(args))
		} else {
			if maxPosCount >= 0 {
				err = makeCLIError(
					"expecting from %d to %d positional arguments, got %d instead",
					minPosCount, maxPosCount, len(args))
			} else {
				err = makeCLIError(
					"expecting at least %d positional arguments, got %d instead",
					minPosCount, len(args))
			}
		}
		c.printError(err)
		return false
	}

	// Check required unset flags.
	unset := []*flag.Flag{}
	c.Flags.VisitAll(func(f *flag.Flag) {
		if strings.HasPrefix(f.DefValue, "<") && f.Value.String() == f.DefValue {
			unset = append(unset, f)
		}
	})
	if len(unset) != 0 {
		missing := make([]string, len(unset))
		for i, f := range unset {
			missing[i] = f.Name
		}
		c.printError(makeCLIError("missing required flags: %v", missing))
		return false
	}

	return true
}

// printError prints error to stderr (recognizing cliErrorTag).
func (c *cipdSubcommand) printError(err error) {
	if cliErrorTag.In(err) {
		fmt.Fprintf(os.Stderr, "Bad command line: %s.\n\n", err)
		c.Flags.Usage()
		return
	}

	if merr, _ := err.(errors.MultiError); len(merr) != 0 {
		fmt.Fprintln(os.Stderr, "Errors:")
		for _, err := range merr {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "Error: %s.\n", err)
}

// writeJSONOutput writes result to JSON output file. It returns original error
// if it is non-nil.
func (c *cipdSubcommand) writeJSONOutput(result any, err error) error {
	// -json-output flag wasn't specified.
	if c.jsonOutput == "" {
		return err
	}

	// Prepare the body of the output file.
	var body struct {
		Error        string           `json:"error,omitempty"`         // human-readable message
		ErrorCode    cipderr.Code     `json:"error_code,omitempty"`    // error code enum, omitted on success
		ErrorDetails *cipderr.Details `json:"error_details,omitempty"` // structured error details
		Result       any              `json:"result,omitempty"`
	}
	if err != nil {
		body.Error = err.Error()
		body.ErrorCode = cipderr.ToCode(err)
		body.ErrorDetails = cipderr.ToDetails(err)
	}
	body.Result = result
	out, e := json.MarshalIndent(&body, "", "  ")
	if e != nil {
		if err == nil {
			err = e
		} else {
			fmt.Fprintf(os.Stderr, "Failed to serialize JSON output: %s\n", e)
		}
		return err
	}

	e = os.WriteFile(c.jsonOutput, out, 0666)
	if e != nil {
		if err == nil {
			err = e
		} else {
			fmt.Fprintf(os.Stderr, "Failed write JSON output to %s: %s\n", c.jsonOutput, e)
		}
		return err
	}

	return err
}

// done is called as a last step of processing a subcommand. It dumps command
// result (or error) to JSON output file, prints error message and generates
// process exit code.
func (c *cipdSubcommand) done(result any, err error) int {
	err = c.writeJSONOutput(result, err)
	if err != nil {
		c.printError(err)
		return 1
	}
	return 0
}

// doneWithPins is a handy shortcut that prints a pinInfo slice and
// deduces process exit code based on presence of errors there.
//
// This just calls through to doneWithPinMap.
func (c *cipdSubcommand) doneWithPins(pins []pinInfo, err error) int {
	return c.doneWithPinMap(map[string][]pinInfo{"": pins}, err)
}

// doneWithPinMap is a handy shortcut that prints the subdir->pinInfo map and
// deduces process exit code based on presence of errors there.
func (c *cipdSubcommand) doneWithPinMap(pins map[string][]pinInfo, err error) int {
	// If have an overall (not pin-specific error), print it before pin errors.
	if err != nil {
		c.printError(err)
	}
	printPinsAndError(pins)

	// If have no overall error, hoist all pin errors up top, so we have a final
	// overall error for writeJSONOutput, otherwise it may look as if the call
	// succeeded.
	if err == nil {
		var merr errors.MultiError
		for _, pinSlice := range pins {
			for _, pin := range pinSlice {
				if pin.err != nil {
					merr = append(merr, pin.err)
				}
			}
		}
		if len(merr) != 0 {
			err = merr
		}
	}

	// Dump all this to JSON. Note we don't use done(...) to avoid printing the
	// error again.
	if c.writeJSONOutput(pins, err) != nil {
		return 1
	}
	return 0
}

// cliErrorTag is used to tag errors related to CLI.
var cliErrorTag = errtag.Make("CIPD CLI error", true)

// makeCLIError returns a new error tagged with cliErrorTag and BadArgument.
func makeCLIError(msg string, args ...any) error {
	return cliErrorTag.Apply(cipderr.BadArgument.Apply(errors.Fmt(msg, args...)))
}

////////////////////////////////////////////////////////////////////////////////
// maxThreadsOption mixin.

type maxThreadsOption struct {
	maxThreads int
}

func (opts *maxThreadsOption) registerFlags(f *flag.FlagSet) {
	f.IntVar(&opts.maxThreads, "max-threads", 0,
		"Number of worker threads for extracting packages. If 0 or negative, uses CPU count.")
}

// loadMaxThreads should only be used by subcommands that do not instantiate
// the full CIPD client.
func (opts *maxThreadsOption) loadMaxThreads(ctx context.Context) (int, error) {
	clientOpts := cipd.ClientOptions{MaxThreads: opts.maxThreads}
	if err := clientOpts.LoadFromEnv(ctx); err != nil {
		return 0, err
	}
	return clientOpts.MaxThreads, nil
}

////////////////////////////////////////////////////////////////////////////////
// clientOptions mixin.

type rootDirFlag bool

const (
	withRootDir    rootDirFlag = true
	withoutRootDir rootDirFlag = false
)

type maxThreadsFlag bool

const (
	withMaxThreads    maxThreadsFlag = true
	withoutMaxThreads maxThreadsFlag = false
)

// clientOptions defines command line arguments related to CIPD client creation.
// Subcommands that need a CIPD client embed it.
type clientOptions struct {
	hardcoded Parameters // whatever was passed to registerFlags(...)

	serviceURL string // also mutated by loadEnsureFile
	cacheDir   string
	maxThreads maxThreadsOption
	rootDir    string              // used only if registerFlags got withRootDir arg
	versions   ensure.VersionsFile // mutated by loadEnsureFile

	authFlags authcli.Flags
}

func (opts *clientOptions) resolvedServiceURL(ctx context.Context) string {
	if opts.serviceURL != "" {
		return opts.serviceURL
	}
	if v := environ.FromCtx(ctx).Get(cipd.EnvCIPDServiceURL); v != "" {
		return v
	}
	return opts.hardcoded.ServiceURL
}

func (opts *clientOptions) registerFlags(f *flag.FlagSet, params Parameters, rootDir rootDirFlag, maxThreads maxThreadsFlag) {
	opts.hardcoded = params

	f.StringVar(&opts.serviceURL, "service-url", "",
		fmt.Sprintf(`Backend URL. If provided via an "ensure" file, the URL in the file takes precedence. `+
			`(default %s)`, params.ServiceURL))
	f.StringVar(&opts.cacheDir, "cache-dir", "",
		fmt.Sprintf("Directory for the shared cache (can also be set by %s env var).", cipd.EnvCacheDir))

	if rootDir {
		f.StringVar(&opts.rootDir, "root", "<path>", "Path to an installation site root directory.")
	}
	if maxThreads {
		opts.maxThreads.registerFlags(f)
	}

	opts.authFlags.Register(f, params.DefaultAuthOptions)
	opts.authFlags.RegisterCredentialHelperFlags(f)
	opts.authFlags.RegisterADCFlags(f)
}

func (opts *clientOptions) toCIPDClientOpts(ctx context.Context) (cipd.ClientOptions, error) {
	authOpts, err := opts.authFlags.Options()
	if err != nil {
		return cipd.ClientOptions{}, cipderr.BadArgument.Apply(errors.Fmt("bad auth options: %w", err))
	}

	realOpts := cipd.ClientOptions{
		Root:              opts.rootDir,
		CacheDir:          opts.cacheDir,
		Versions:          opts.versions,
		MaxThreads:        opts.maxThreads.maxThreads,
		AnonymousClient:   http.DefaultClient,
		PluginsContext:    ctx,
		LoginInstructions: "run `cipd auth-login` to login or relogin",
	}
	if err := realOpts.LoadFromEnv(ctx); err != nil {
		return cipd.ClientOptions{}, err
	}
	realOpts.ServiceURL = opts.resolvedServiceURL(ctx)

	// When using a proxy, the proxy does authenticated requests to the backend
	// and the client just hits it anonymously (see ClientOptions.ProxyURL). Skip
	// loading credentials.
	if realOpts.ProxyURL != "" {
		logging.Debugf(ctx, "Using %s=%s", cipd.EnvCIPDProxyURL, realOpts.ProxyURL)
	} else {
		realOpts.AuthenticatedClient, err = auth.NewAuthenticator(ctx, auth.OptionalLogin, authOpts).Client()
		if err != nil {
			return cipd.ClientOptions{}, cipderr.Auth.Apply(errors.Fmt("initializing auth client: %w", err))
		}
	}

	return realOpts, nil
}

func (opts *clientOptions) makeCIPDClient(ctx context.Context) (cipd.Client, error) {
	cipdOpts, err := opts.toCIPDClientOpts(ctx)
	if err != nil {
		return nil, err
	}
	return cipd.NewClient(cipdOpts)
}

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

////////////////////////////////////////////////////////////////////////////////
// refsOptions mixin.

// refList holds an array of '-ref' command line options.
type refList []string

func (refs *refList) String() string {
	return "ref"
}

// Set is called by 'flag' package when parsing command line options.
func (refs *refList) Set(value string) error {
	err := common.ValidatePackageRef(value)
	if err != nil {
		return cliErrorTag.Apply(err)
	}
	*refs = append(*refs, value)
	return nil
}

// refsOptions defines command line arguments for commands that accept a set
// of refs.
type refsOptions struct {
	refs refList
}

func (opts *refsOptions) registerFlags(f *flag.FlagSet) {
	f.Var(&opts.refs, "ref", "A `ref` to point to the package instance (can be used multiple times).")
}

////////////////////////////////////////////////////////////////////////////////
// tagsOptions mixin.

// tagList holds an array of '-tag' command line options.
type tagList []string

func (tags *tagList) String() string {
	return "key:value"
}

// Set is called by 'flag' package when parsing command line options.
func (tags *tagList) Set(value string) error {
	err := common.ValidateInstanceTag(value)
	if err != nil {
		return cliErrorTag.Apply(err)
	}
	*tags = append(*tags, value)
	return nil
}

// tagsOptions defines command line arguments for commands that accept a set
// of tags.
type tagsOptions struct {
	tags tagList
}

func (opts *tagsOptions) registerFlags(f *flag.FlagSet) {
	f.Var(&opts.tags, "tag", "A `key:value` tag to attach to the package instance (can be used multiple times).")
}

////////////////////////////////////////////////////////////////////////////////
// metadataOptions mixin.

type metadataFlagValue struct {
	key         string
	value       string // either a literal value or a path to read it from
	contentType string
}

type metadataList struct {
	entries   []metadataFlagValue
	valueKind string // "value" or "path"
}

func (md *metadataList) String() string {
	return "key:" + md.valueKind
}

// Set is called by 'flag' package when parsing command line options.
func (md *metadataList) Set(value string) error {
	// Should have form key_with_possible_content_type:value.
	chunks := strings.SplitN(value, ":", 2)
	if len(chunks) != 2 {
		return md.badFormatError()
	}

	// Extract content-type from within trailing '(...)', if present.
	key, contentType, value := chunks[0], "", chunks[1]
	switch l, r := strings.Index(key, "("), strings.LastIndex(key, ")"); {
	case l == -1 && r == -1:
		// no content type, this is fine
	case l != -1 && r != -1 && l < r:
		// The closing ')' should be the last character.
		if !strings.HasSuffix(key, ")") {
			return md.badFormatError()
		}
		key, contentType = key[:l], key[l+1:r]
	default:
		return md.badFormatError()
	}

	// Validate everything we can.
	if err := common.ValidateInstanceMetadataKey(key); err != nil {
		return cliErrorTag.Apply(err)
	}
	if err := common.ValidateContentType(contentType); err != nil {
		return cliErrorTag.Apply(err)
	}
	if md.valueKind == "value" {
		if err := common.ValidateInstanceMetadataLen(len(value)); err != nil {
			return cliErrorTag.Apply(err)
		}
	}

	md.entries = append(md.entries, metadataFlagValue{
		key:         key,
		value:       value,
		contentType: contentType,
	})
	return nil
}

func (md *metadataList) badFormatError() error {
	return makeCLIError("should have form key:%s or key(content-type):%s", md.valueKind, md.valueKind)
}

// metadataOptions defines command line arguments for commands that accept a set
// of metadata entries.
type metadataOptions struct {
	metadata         metadataList
	metadataFromFile metadataList
}

func (opts *metadataOptions) registerFlags(f *flag.FlagSet) {
	opts.metadata.valueKind = "value"
	f.Var(&opts.metadata, "metadata",
		"A metadata entry (`key:value` or key(content-type):value) to attach to the package instance (can be used multiple times).")

	opts.metadataFromFile.valueKind = "path"
	f.Var(&opts.metadataFromFile, "metadata-from-file",
		"A metadata entry (`key:path` or key(content-type):path) to attach to the package instance (can be used multiple times). The path can be \"-\" to read from stdin.")
}

func (opts *metadataOptions) load(ctx context.Context) ([]cipd.Metadata, error) {
	out := make([]cipd.Metadata, 0, len(opts.metadata.entries)+len(opts.metadataFromFile.entries))

	// Convert -metadata to cipd.Metadata entries.
	for _, md := range opts.metadata.entries {
		entry := cipd.Metadata{
			Key:         md.key,
			Value:       []byte(md.value),
			ContentType: md.contentType,
		}
		// The default content type for -metadata is text/plain (since values are
		// supplied directly via the command line).
		if entry.ContentType == "" {
			entry.ContentType = "text/plain"
		}
		out = append(out, entry)
	}

	// Load -metadata-from-file entries. At most one `-metadata-from-file key:-`
	// is allowed, we have only one stdin.
	keyWithStdin := false
	for _, md := range opts.metadataFromFile.entries {
		if md.value == "-" {
			if keyWithStdin {
				return nil, makeCLIError("at most one -metadata-from-file can use \"-\" as a value")
			}
			keyWithStdin = true
		}
		entry := cipd.Metadata{
			Key:         md.key,
			ContentType: md.contentType,
		}
		var err error
		if entry.Value, err = loadMetadataFromFile(ctx, md.key, md.value); err != nil {
			return nil, err
		}
		// Guess the content type from the file extension and its body.
		if entry.ContentType == "" {
			entry.ContentType = guessMetadataContentType(md.value, entry.Value)
		}
		out = append(out, entry)
	}

	return out, nil
}

func loadMetadataFromFile(ctx context.Context, key, path string) ([]byte, error) {
	var file *os.File
	if path == "-" {
		logging.Infof(ctx, "Reading metadata %q from the stdin...", key)
		file = os.Stdin
	} else {
		logging.Infof(ctx, "Reading metadata %q from %q...", key, path)
		var err error
		file, err = os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, cipderr.BadArgument.Apply(errors.Fmt("missing metadata file: %w", err))
			}
			return nil, cipderr.IO.Apply(errors.Fmt("reading metadata file: %w", err))
		}
		defer file.Close()
	}
	// Read at most MetadataMaxLen plus one more byte to detect true EOF.
	buf := bytes.Buffer{}
	switch _, err := io.CopyN(&buf, file, common.MetadataMaxLen+1); {
	case err == nil:
		// Successfully read more than needed => the file size is too large.
		return nil, cipderr.BadArgument.Apply(errors.Fmt("the metadata value in %q is too long, should be <=%d bytes", path, common.MetadataMaxLen))
	case err != io.EOF:
		// Failed with some unexpected read error.
		return nil, cipderr.IO.Apply(errors.Fmt("error reading metadata from %q: %w", path, err))
	default:
		return buf.Bytes(), nil
	}
}

func guessMetadataContentType(path string, val []byte) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "application/json"
	case ".jwt":
		return "application/jwt"
	default:
		return http.DetectContentType(val)
	}
}

////////////////////////////////////////////////////////////////////////////////
// uploadOptions mixin.

// uploadOptions defines command line options for commands that upload packages.
type uploadOptions struct {
	verificationTimeout time.Duration
}

func (opts *uploadOptions) registerFlags(f *flag.FlagSet) {
	f.DurationVar(
		&opts.verificationTimeout, "verification-timeout",
		cipd.CASFinalizationTimeout, "Maximum time to wait for backend-side package hash verification.")
}

////////////////////////////////////////////////////////////////////////////////
// hashOptions mixin.

// allAlgos is used in the flag help text, it is "sha256, sha1, ...".
var allAlgos string

func init() {
	algos := make([]string, 0, len(api.HashAlgo_name)-1)
	for i := len(api.HashAlgo_name) - 1; i > 0; i-- {
		algos = append(algos, strings.ToLower(api.HashAlgo_name[int32(i)]))
	}
	allAlgos = strings.Join(algos, ", ")
}

// hashAlgoFlag adapts api.HashAlgo to flag.Value interface.
type hashAlgoFlag api.HashAlgo

// String is called by 'flag' package when displaying default value of a flag.
func (ha *hashAlgoFlag) String() string {
	return strings.ToLower(api.HashAlgo(*ha).String())
}

// Set is called by 'flag' package when parsing command line options.
func (ha *hashAlgoFlag) Set(value string) error {
	val := api.HashAlgo_value[strings.ToUpper(value)]
	if val == 0 {
		return makeCLIError("unknown hash algo %q, should be one of: %s", value, allAlgos)
	}
	*ha = hashAlgoFlag(val)
	return nil
}

// hashOptions defines -hash-algo flag that specifies hash algo to use for
// constructing instance IDs.
//
// Default value is given by common.DefaultHashAlgo.
//
// Not all algos may be accepted by the server.
type hashOptions struct {
	algo hashAlgoFlag
}

func (opts *hashOptions) registerFlags(f *flag.FlagSet) {
	opts.algo = hashAlgoFlag(common.DefaultHashAlgo)
	f.Var(&opts.algo, "hash-algo", fmt.Sprintf("Algorithm to use for deriving package instance ID, one of: %s", allAlgos))
}

func (opts *hashOptions) hashAlgo() api.HashAlgo {
	return api.HashAlgo(opts.algo)
}

////////////////////////////////////////////////////////////////////////////////
// ensureFileOptions mixin.

type legacyListFlag bool

const (
	withLegacyListFlag    legacyListFlag = true
	withoutLegacyListFlag legacyListFlag = false
)

type ensureOutFlag bool

const (
	withEnsureOutFlag    ensureOutFlag = true
	withoutEnsureOutFlag ensureOutFlag = false
)

type verifyingEnsureFile bool

const (
	requireVerifyPlatforms verifyingEnsureFile = true
	ignoreVerifyPlatforms  verifyingEnsureFile = false
)

type versionFileOpt bool

const (
	parseVersionsFile  versionFileOpt = true
	ignoreVersionsFile versionFileOpt = false
)

// ensureFileOptions defines -ensure-file flag that specifies a location of the
// "ensure file", which is a manifest that describes what should be installed
// into a site root.
type ensureFileOptions struct {
	ensureFile    string
	ensureFileOut string // used only if registerFlags got withEnsureOutFlag arg
}

func (opts *ensureFileOptions) registerFlags(f *flag.FlagSet, out ensureOutFlag, list legacyListFlag) {
	f.StringVar(&opts.ensureFile, "ensure-file", "<path>",
		`An "ensure" file. See syntax described here: `+
			`https://godoc.org/go.chromium.org/luci/cipd/client/cipd/ensure.`+
			` Providing '-' will read from stdin.`)
	if out {
		f.StringVar(&opts.ensureFileOut, "ensure-file-output", "",
			`A path to write an "ensure" file which is the fully-resolved version `+
				`of the input ensure file for the current platform. This output will `+
				`not contain any ${params} or $Settings other than $ServiceURL.`)
	}
	if list {
		f.StringVar(&opts.ensureFile, "list", "<path>", "(DEPRECATED) A synonym for -ensure-file.")
	}
}

// loadEnsureFile parses the ensure file and mutates clientOpts to point to a
// service URL specified in the ensure file.
func (opts *ensureFileOptions) loadEnsureFile(ctx context.Context, clientOpts *clientOptions, verifying verifyingEnsureFile, parseVers versionFileOpt) (*ensure.File, error) {
	parsedFile, err := ensure.LoadEnsureFile(opts.ensureFile)
	if err != nil {
		return nil, err
	}

	// Prefer the ServiceURL from the file (if set), and log a warning if the user
	// provided one on the command line that doesn't match the one in the file.
	if parsedFile.ServiceURL != "" {
		if clientOpts.serviceURL != "" && clientOpts.serviceURL != parsedFile.ServiceURL {
			logging.Warningf(ctx, "serviceURL in ensure file != serviceURL on CLI (%q v %q). Using %q from file.",
				parsedFile.ServiceURL, clientOpts.serviceURL, parsedFile.ServiceURL)
		}
		clientOpts.serviceURL = parsedFile.ServiceURL
	}

	if verifying && len(parsedFile.VerifyPlatforms) == 0 {
		defaultVerifiedPlatform := template.DefaultTemplate()
		parsedFile.VerifyPlatforms = append(parsedFile.VerifyPlatforms, defaultVerifiedPlatform)

		logging.Infof(ctx, "$VerifiedPlatform directive required but not included in"+
			" ensure file, using '$VerifiedPlatform %s' as default.", defaultVerifiedPlatform)
	}

	if parseVers && parsedFile.ResolvedVersions != "" {
		clientOpts.versions, err = loadVersionsFile(parsedFile.ResolvedVersions, opts.ensureFile)
		if err != nil {
			return nil, err
		}
		logging.Debugf(ctx, "Using the resolved version file %q", filepath.Base(parsedFile.ResolvedVersions))
	}

	return parsedFile, nil
}

////////////////////////////////////////////////////////////////////////////////
// Support for running operations concurrently.

// batchOperation defines what to do with a packages matching a prefix.
type batchOperation struct {
	client        cipd.Client
	packagePrefix string   // a package name or a prefix
	packages      []string // packages to operate on, overrides packagePrefix
	callback      func(pkg string) (common.Pin, error)
}

// expandPkgDir takes a package name or '<prefix>/' and returns a list
// of matching packages (asking backend if necessary). Doesn't recurse, returns
// only direct children.
func expandPkgDir(ctx context.Context, c cipd.Client, packagePrefix string) ([]string, error) {
	if !strings.HasSuffix(packagePrefix, "/") {
		return []string{packagePrefix}, nil
	}
	pkgs, err := c.ListPackages(ctx, packagePrefix, false, false)
	if err != nil {
		return nil, err
	}
	// Skip directories.
	var out []string
	for _, p := range pkgs {
		if !strings.HasSuffix(p, "/") {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil,
			cipderr.RPC.WithDetails(cipderr.Details{
				Package: packagePrefix,
			}).Apply(errors.Fmt("no packages under %s", packagePrefix))

	}
	return out, nil
}

// performBatchOperation expands a package prefix into a list of packages and
// calls callback for each of them (concurrently) gathering the results.
//
// Returns an error only if the prefix expansion fails. Errors from individual
// operations are returned through []pinInfo, use hasErrors to check them.
func performBatchOperation(ctx context.Context, op batchOperation) ([]pinInfo, error) {
	op.client.BeginBatch(ctx)
	defer op.client.EndBatch(ctx)

	pkgs := op.packages
	if len(pkgs) == 0 {
		var err error
		pkgs, err = expandPkgDir(ctx, op.client, op.packagePrefix)
		if err != nil {
			return nil, err
		}
	}
	return callConcurrently(pkgs, func(pkg string) pinInfo {
		pin, err := op.callback(pkg)
		if err != nil {
			return pinInfo{
				Pkg:          pkg,
				Error:        err.Error(),
				ErrorCode:    cipderr.ToCode(err),
				ErrorDetails: cipderr.ToDetails(err),
				err:          err,
			}
		}
		return pinInfo{Pkg: pkg, Pin: &pin}
	}), nil
}

func callConcurrently(pkgs []string, callback func(pkg string) pinInfo) []pinInfo {
	// Push index through channel to make results ordered as 'pkgs'.
	ch := make(chan struct {
		int
		pinInfo
	})
	for idx, pkg := range pkgs {
		go func(idx int, pkg string) {
			ch <- struct {
				int
				pinInfo
			}{idx, callback(pkg)}
		}(idx, pkg)
	}
	pins := make([]pinInfo, len(pkgs))
	for range pkgs {
		res := <-ch
		pins[res.int] = res.pinInfo
	}
	return pins
}

func printPinsAndError(pinMap map[string][]pinInfo) {
	for subdir, pins := range pinMap {
		hasPins := false
		hasErrors := false
		for _, p := range pins {
			if p.Error != "" {
				hasErrors = true
			} else if p.Pin != nil {
				hasPins = true
			}
		}
		subdirString := ""
		if (hasPins || hasErrors) && (len(pinMap) > 1 || subdir != "") {
			// only print this if it's not the root subdir, or there's more than one
			// subdir in pinMap.
			subdirString = fmt.Sprintf(" (subdir %q)", subdir)
		}
		if hasPins {
			fmt.Printf("Packages%s:\n", subdirString)
			for _, p := range pins {
				if p.Error != "" || p.Pin == nil {
					continue
				}
				plat := ""
				if p.Platform != "" {
					plat = fmt.Sprintf(" (for %s)", p.Platform)
				}
				tracking := ""
				if p.Tracking != "" {
					tracking = fmt.Sprintf(" (tracking %q)", p.Tracking)
				}
				fmt.Printf("  %s%s%s\n", p.Pin, plat, tracking)
			}
		}
		if hasErrors {
			fmt.Fprintf(os.Stderr, "Errors%s:\n", subdirString)
			for _, p := range pins {
				if p.Error != "" {
					fmt.Fprintf(os.Stderr, "  %s: %s.\n", p.Pkg, p.Error)
				}
			}
		}
	}
}

func hasErrors(pins []pinInfo) bool {
	for _, p := range pins {
		if p.Error != "" {
			return true
		}
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////
// Ensure-file related helpers.

func resolveEnsureFile(ctx context.Context, f *ensure.File, clientOpts clientOptions) (map[string][]pinInfo, ensure.VersionsFile, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer client.Close(ctx)

	out := ensure.VersionsFile{}
	mu := sync.Mutex{}

	resolver := cipd.Resolver{
		Client:         client,
		VerifyPresence: true,
		Visitor: func(pkg, ver, iid string) {
			mu.Lock()
			out.AddVersion(pkg, ver, iid)
			mu.Unlock()
		},
	}
	results, err := resolver.ResolveAllPlatforms(ctx, f)
	if err != nil {
		return nil, nil, err
	}
	return resolvedFilesToPinMap(results), out, nil
}

func resolvedFilesToPinMap(res map[template.Platform]*ensure.ResolvedFile) map[string][]pinInfo {
	pinMap := map[string][]pinInfo{}
	for plat, resolved := range res {
		for subdir, resolvedPins := range resolved.PackagesBySubdir {
			pins := pinMap[subdir]
			for _, pin := range resolvedPins {
				// Put a copy into 'pins', otherwise they all end up pointing to the
				// same variable living in the outer scope.
				pins = append(pins, pinInfo{
					Pkg:      pin.PackageName,
					Pin:      &pin,
					Platform: plat.String(),
				})
			}
			pinMap[subdir] = pins
		}
	}

	// Sort pins by (package name, platform) for deterministic output.
	for _, v := range pinMap {
		sort.Slice(v, func(i, j int) bool {
			if v[i].Pkg == v[j].Pkg {
				return v[i].Platform < v[j].Platform
			}
			return v[i].Pkg < v[j].Pkg
		})
	}
	return pinMap
}

func loadVersionsFile(path, ensureFile string) (ensure.VersionsFile, error) {
	switch f, err := os.Open(path); {
	case os.IsNotExist(err):
		return nil,
			cipderr.BadArgument.Apply(errors.Fmt("the resolved versions file doesn't exist, "+
				"use 'cipd ensure-file-resolve -ensure-file %q' to generate it", ensureFile))
	case err != nil:
		return nil, cipderr.IO.Apply(errors.Fmt("reading resolved versions file: %w", err))
	default:
		defer f.Close()
		return ensure.ParseVersionsFile(f)
	}
}

func saveVersionsFile(path string, v ensure.VersionsFile) error {
	buf := bytes.Buffer{}
	if err := v.Serialize(&buf); err != nil {
		return err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0666); err != nil {
		return cipderr.IO.Apply(errors.Fmt("writing versions file: %w", err))
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// 'create' subcommand.

func cmdCreate(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "create [options]",
		ShortDesc: "builds and uploads a package instance file",
		LongDesc:  "Builds and uploads a package instance file.",
		CommandRun: func() subcommands.CommandRun {
			c := &createRun{}
			c.registerBaseFlags()
			c.Opts.inputOptions.registerFlags(&c.Flags)
			c.Opts.refsOptions.registerFlags(&c.Flags)
			c.Opts.tagsOptions.registerFlags(&c.Flags)
			c.Opts.metadataOptions.registerFlags(&c.Flags)
			c.Opts.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Opts.uploadOptions.registerFlags(&c.Flags)
			c.Opts.hashOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type createOpts struct {
	inputOptions
	refsOptions
	tagsOptions
	metadataOptions
	clientOptions
	uploadOptions
	hashOptions
}

type createRun struct {
	cipdSubcommand

	Opts createOpts
}

func (c *createRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(buildAndUploadInstance(ctx, &c.Opts))
}

func buildAndUploadInstance(ctx context.Context, opts *createOpts) (common.Pin, error) {
	f, err := os.CreateTemp("", "cipd_pkg")
	if err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("creating temp instance file: %w", err))
	}
	defer func() {
		// Note: we don't care about errors here since this file is used only as
		// a temporary buffer between buildInstanceFile and registerInstanceFile.
		// These functions check that everything was uploaded correctly using
		// hashes.
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()
	pin, err := buildInstanceFile(ctx, f.Name(), opts.inputOptions, opts.hashAlgo())
	if err != nil {
		return common.Pin{}, err
	}
	return registerInstanceFile(ctx, f.Name(), &pin, &registerOpts{
		refsOptions:     opts.refsOptions,
		tagsOptions:     opts.tagsOptions,
		metadataOptions: opts.metadataOptions,
		clientOptions:   opts.clientOptions,
		uploadOptions:   opts.uploadOptions,
		hashOptions:     opts.hashOptions,
	})
}

////////////////////////////////////////////////////////////////////////////////
// 'attach' subcommand.

func cmdAttach(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "attach <package or package prefix> -metadata key:value -metadata-from-file key:path -tag key:value -ref name [options]",
		ShortDesc: "attaches tags, metadata and points refs to an instance",
		LongDesc: `Attaches tags, metadata and points refs to an instance.

Note that this operation is not atomic. It attaches metadata first, then tags,
then moves refs one by one. Reattaching already attached data is not an error
though, so a failed operation can be safely retried.
`,
		CommandRun: func() subcommands.CommandRun {
			c := &attachRun{}
			c.registerBaseFlags()
			c.refsOptions.registerFlags(&c.Flags)
			c.tagsOptions.registerFlags(&c.Flags)
			c.metadataOptions.registerFlags(&c.Flags)
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>",
				"Package version to resolve. Could also be a tag or a ref.")
			return c
		},
	}
}

type attachRun struct {
	cipdSubcommand
	refsOptions
	tagsOptions
	metadataOptions
	clientOptions

	version string
}

func (c *attachRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}

	ctx := cli.GetContext(a, c, env)

	md, err := c.metadataOptions.load(ctx)
	if err != nil {
		return c.done(nil, err)
	}
	if len(c.refs) == 0 && len(c.tags) == 0 && len(md) == 0 {
		return c.done(nil, makeCLIError("no -tags, -refs or -metadata is provided"))
	}

	pkgPrefix, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	return c.doneWithPins(visitPins(ctx, &visitPinsArgs{
		clientOptions: c.clientOptions,
		packagePrefix: pkgPrefix,
		version:       c.version,
		updatePin: func(client cipd.Client, pin common.Pin) error {
			return attachAndMove(ctx, client, pin, md, c.tags, c.refs)
		},
	}))
}

////////////////////////////////////////////////////////////////////////////////
// 'ensure' subcommand.

func cmdEnsure(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ensure [options]",
		ShortDesc: "installs, removes and updates packages in one go",
		LongDesc: `Installs, removes and updates packages in one go.

Prepare an 'ensure file' by listing packages and their versions, each on their
own line, e.g.:

    some/package/name/${platform}  version:1.2.3
    other/package                  some_ref

Then use the ensure command to read this ensure file and 'ensure' that a given
folder has the packages at the versions specified:

    cipd ensure -root a/directory -ensure-file ensure_file

For the full syntax of the ensure file, see:

   https://go.chromium.org/luci/cipd/client/cipd/ensure
`,
		CommandRun: func() subcommands.CommandRun {
			c := &ensureRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withEnsureOutFlag, withLegacyListFlag)
			return c
		},
	}
}

type ensureRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *ensureRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	ef, err := c.loadEnsureFile(ctx, &c.clientOptions, ignoreVerifyPlatforms, parseVersionsFile)
	if err != nil {
		return c.done(nil, err)
	}

	pins, _, err := ensurePackages(ctx, ef, c.ensureFileOut, false, c.clientOptions)
	return c.done(pins, err)
}

func ensurePackages(ctx context.Context, ef *ensure.File, ensureFileOut string, dryRun bool, clientOpts clientOptions) (common.PinSliceBySubdir, cipd.ActionMap, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer client.Close(ctx)

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	resolver := cipd.Resolver{Client: client}
	resolved, err := resolver.Resolve(ctx, ef, template.DefaultExpander())
	if err != nil {
		return nil, nil, err
	}

	actions, err := client.EnsurePackages(ctx, resolved.PackagesBySubdir, &cipd.EnsureOptions{
		Paranoia:            resolved.ParanoidMode,
		DryRun:              dryRun,
		OverrideInstallMode: resolved.OverrideInstallMode,
	})
	if err != nil {
		return nil, actions, err
	}

	if ensureFileOut != "" {
		buf := bytes.Buffer{}
		resolved.ServiceURL = clientOpts.resolvedServiceURL(ctx)
		resolved.ParanoidMode = ""
		if err = resolved.Serialize(&buf); err == nil {
			err = os.WriteFile(ensureFileOut, buf.Bytes(), 0666)
			if err != nil {
				err = cipderr.IO.Apply(errors.Fmt("writing resolved ensure file: %w", err))
			}
		}

	}

	return resolved.PackagesBySubdir, actions, err
}

////////////////////////////////////////////////////////////////////////////////
// 'ensure-file-verify' subcommand.

func cmdEnsureFileVerify(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ensure-file-verify [options]",
		ShortDesc: "verifies packages in a manifest exist for all platforms",
		LongDesc: "Verifies that the packages in the \"ensure\" file exist for all platforms.\n\n" +
			"Additionally if the ensure file uses $ResolvedVersions directive, checks that " +
			"all versions there are up-to-date. Returns non-zero if some version can't be " +
			"resolved or $ResolvedVersions file is outdated.",
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			c := &ensureFileVerifyRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withoutEnsureOutFlag, withoutLegacyListFlag)
			return c
		},
	}
}

type ensureFileVerifyRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *ensureFileVerifyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	ef, err := c.loadEnsureFile(ctx, &c.clientOptions, requireVerifyPlatforms, ignoreVersionsFile)
	if err != nil {
		return c.done(nil, err)
	}

	// Resolving all versions in the ensure file also naturally verifies all
	// versions exist.
	pinMap, versions, err := resolveEnsureFile(ctx, ef, c.clientOptions)
	if err != nil || ef.ResolvedVersions == "" {
		return c.doneWithPinMap(pinMap, err)
	}

	// Verify $ResolvedVersions file is up-to-date too.
	switch existing, err := loadVersionsFile(ef.ResolvedVersions, c.ensureFile); {
	case err != nil:
		return c.done(nil, err)
	case !existing.Equal(versions):
		return c.done(nil,

			cipderr.Stale.Apply(errors.Fmt("the resolved versions file %s is stale, "+
				"use 'cipd ensure-file-resolve -ensure-file %q' to update it",
				filepath.Base(ef.ResolvedVersions), c.ensureFile)))
	default:
		return c.doneWithPinMap(pinMap, err)
	}
}

////////////////////////////////////////////////////////////////////////////////
// 'ensure-file-resolve' subcommand.

func cmdEnsureFileResolve(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ensure-file-resolve [options]",
		ShortDesc: "resolves versions of all packages and writes them into $ResolvedVersions file",
		LongDesc: "Resolves versions of all packages for all verified platforms in the \"ensure\" file.\n\n" +
			`Writes them to a file specified by $ResolvedVersions directive in the ensure file, ` +
			`to be used for version resolution during "cipd ensure ..." instead of calling the backend.`,
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			c := &ensureFileResolveRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withoutEnsureOutFlag, withoutLegacyListFlag)
			return c
		},
	}
}

type ensureFileResolveRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *ensureFileResolveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	ef, err := c.loadEnsureFile(ctx, &c.clientOptions, requireVerifyPlatforms, ignoreVersionsFile)
	switch {
	case err != nil:
		return c.done(nil, err)
	case ef.ResolvedVersions == "":
		logging.Errorf(ctx,
			"The ensure file doesn't have $ResolvedVersion directive that specifies "+
				"where to put the resolved package versions, so it can't be resolved.")
		return c.done(nil, cipderr.BadArgument.Apply(errors.New("no resolved versions file configured")))
	}

	pinMap, versions, err := resolveEnsureFile(ctx, ef, c.clientOptions)
	if err != nil {
		return c.doneWithPinMap(pinMap, err)
	}

	if err := saveVersionsFile(ef.ResolvedVersions, versions); err != nil {
		return c.done(nil, err)
	}

	fmt.Printf("The resolved versions have been written to %s.\n\n", filepath.Base(ef.ResolvedVersions))
	return c.doneWithPinMap(pinMap, nil)
}

////////////////////////////////////////////////////////////////////////////////
// 'puppet-check-updates' subcommand.

func cmdPuppetCheckUpdates(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "puppet-check-updates [options]",
		ShortDesc: "returns 0 exit code iff 'ensure' will do some actions",
		LongDesc: "Returns 0 exit code iff 'ensure' will do some actions.\n\n" +
			"Exists to be used from Puppet's Exec 'onlyif' option to trigger " +
			"'ensure' only if something is out of date. If puppet-check-updates " +
			"fails with a transient error, it returns non-zero exit code (as usual), " +
			"so that Puppet doesn't trigger notification chain (that can result in " +
			"service restarts). On fatal errors it returns 0 to let Puppet run " +
			"'ensure' for real and catch an error.",
		CommandRun: func() subcommands.CommandRun {
			c := &checkUpdatesRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withoutEnsureOutFlag, withLegacyListFlag)
			return c
		},
	}
}

type checkUpdatesRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *checkUpdatesRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	ef, err := c.loadEnsureFile(ctx, &c.clientOptions, ignoreVerifyPlatforms, parseVersionsFile)
	if err != nil {
		return 0 // on fatal errors ask puppet to run 'ensure' for real
	}

	_, actions, err := ensurePackages(ctx, ef, "", true, c.clientOptions)
	if err != nil {
		ret := c.done(actions, err)
		if transient.Tag.In(err) {
			return ret // fail as usual
		}
		return 0 // on fatal errors ask puppet to run 'ensure' for real
	}
	c.done(actions, nil)
	if len(actions) == 0 {
		return 5 // some arbitrary non-zero number, unlikely to show up on errors
	}
	return 0
}

////////////////////////////////////////////////////////////////////////////////
// 'resolve' subcommand.

func cmdResolve(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "resolve <package or package prefix> [options]",
		ShortDesc: "returns concrete package instance ID given a version",
		LongDesc:  "Returns concrete package instance ID given a version.",
		CommandRun: func() subcommands.CommandRun {
			c := &resolveRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>", "Package version to resolve.")
			return c
		},
	}
}

type resolveRun struct {
	cipdSubcommand
	clientOptions

	version string
}

func (c *resolveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.doneWithPins(resolveVersion(ctx, args[0], c.version, c.clientOptions))
}

func resolveVersion(ctx context.Context, packagePrefix, version string, clientOpts clientOptions) ([]pinInfo, error) {
	packagePrefix, err := expandTemplate(packagePrefix)
	if err != nil {
		return nil, err
	}

	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	return performBatchOperation(ctx, batchOperation{
		client:        client,
		packagePrefix: packagePrefix,
		callback: func(pkg string) (common.Pin, error) {
			return client.ResolveVersion(ctx, pkg, version)
		},
	})
}

////////////////////////////////////////////////////////////////////////////////
// 'export' subcommand.

func cmdExport(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "export [options]",
		ShortDesc: "writes packages to disk as a 'one-off'",
		LongDesc: `Writes packages to disk as a 'one-off'.

This writes packages to disk and discards any CIPD-related tracking data.  The
result is an installation of the specified packages on disk in a form you could
use to re-package them elsewhere (e.g. to create a tarball, zip, etc.), or for
some 'one time' use (where you don't need any of the guarantees provided by
"ensure").

In particular:
  * Export will blindly overwrite files in the target directory.
  * If a package removes a file(s) in some newer version, using "export"
    to write over an old version with the new version will not clean
    up the file(s).

Prepare an 'ensure file' as you would pass to the "ensure" subcommand.
This command implies "$OverrideInstallMode copy" in the ensure file.

    cipd export -root a/directory -ensure-file ensure_file

For the full syntax of the ensure file, see:

   https://go.chromium.org/luci/cipd/client/cipd/ensure
`,
		CommandRun: func() subcommands.CommandRun {
			c := &exportRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withEnsureOutFlag, withLegacyListFlag)

			return c
		},
	}
}

type exportRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *exportRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	ef, err := c.loadEnsureFile(ctx, &c.clientOptions, ignoreVerifyPlatforms, parseVersionsFile)
	if err != nil {
		return c.done(nil, err)
	}
	ef.OverrideInstallMode = pkg.InstallModeCopy

	pins, _, err := ensurePackages(ctx, ef, c.ensureFileOut, false, c.clientOptions)
	if err != nil {
		return c.done(pins, err)
	}

	logging.Infof(ctx, "Removing cipd metadata")
	fsystem := fs.NewFileSystem(c.rootDir, "")
	ssd, err := fsystem.RootRelToAbs(fs.SiteServiceDir)
	if err != nil {
		err = cipderr.IO.Apply(errors.Fmt("unable to resolve service dir: %w", err))
	} else if err = fsystem.EnsureDirectoryGone(ctx, ssd); err != nil {
		err = cipderr.IO.Apply(errors.Fmt("unable to purge service dir: %w", err))
	}
	return c.done(pins, err)
}

////////////////////////////////////////////////////////////////////////////////
// 'describe' subcommand.

func cmdDescribe(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "describe <package> [options]",
		ShortDesc: "returns information about a package instance given its version",
		LongDesc: "Returns information about a package instance given its version: " +
			"who uploaded the instance and when and a list of attached tags.",
		CommandRun: func() subcommands.CommandRun {
			c := &describeRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>", "Package version to describe.")
			return c
		},
	}
}

type describeRun struct {
	cipdSubcommand
	clientOptions

	version string
}

func isPrintableContentType(contentType string) bool {
	if strings.HasPrefix(contentType, "text/") ||
		slices.Contains([]string{"application/json", "application/jwt"}, contentType) {
		return true
	}
	return false
}

func (c *describeRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(describeInstance(ctx, args[0], c.version, c.clientOptions))
}

func describeInstance(ctx context.Context, pkg, version string, clientOpts clientOptions) (*cipd.InstanceDescription, error) {
	pkg, err := expandTemplate(pkg)
	if err != nil {
		return nil, err
	}

	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	pin, err := client.ResolveVersion(ctx, pkg, version)
	if err != nil {
		return nil, err
	}

	desc, err := client.DescribeInstance(ctx, pin, &cipd.DescribeInstanceOpts{
		DescribeRefs:     true,
		DescribeTags:     true,
		DescribeMetadata: true,
	})
	if err != nil {
		return nil, err
	}

	fmt.Printf("Package:       %s\n", desc.Pin.PackageName)
	fmt.Printf("Instance ID:   %s\n", desc.Pin.InstanceID)
	fmt.Printf("Registered by: %s\n", desc.RegisteredBy)
	fmt.Printf("Registered at: %s\n", time.Time(desc.RegisteredTs).Local())
	if len(desc.Refs) != 0 {
		fmt.Printf("Refs:\n")
		for _, t := range desc.Refs {
			fmt.Printf("  %s\n", t.Ref)
		}
	} else {
		fmt.Printf("Refs:          none\n")
	}
	if len(desc.Tags) != 0 {
		fmt.Printf("Tags:\n")
		for _, t := range desc.Tags {
			fmt.Printf("  %s\n", t.Tag)
		}
	} else {
		fmt.Printf("Tags:          none\n")
	}
	if len(desc.Metadata) != 0 {
		fmt.Printf("Metadata:\n")
		for _, md := range desc.Metadata {
			printValue := string(md.Value)
			if !isPrintableContentType(md.ContentType) {
				// Content type is highly unlikely to be meaningfully printable.
				// Output something reasonable to the CLI (JSON will still have
				// the full payload).
				printValue = fmt.Sprintf("<%s binary, %d bytes>", md.ContentType, len(md.Value))
			}
			// Add indentation if the value contains newlines.
			if strings.Contains(printValue, "\n") {
				printValue = "\n" + printValue
				printValue = strings.ReplaceAll(printValue, "\n", "\n    ")
			}
			fmt.Printf("  %s:%s\n", md.Key, printValue)
		}
	} else {
		fmt.Printf("Metadata:      none\n")
	}

	return desc, nil
}

////////////////////////////////////////////////////////////////////////////////
// 'instances' subcommand.

func cmdInstances(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "instances <package> [-limit ...]",
		ShortDesc: "lists instances of a package",
		LongDesc:  "Lists instances of a package, most recently uploaded first.",
		CommandRun: func() subcommands.CommandRun {
			c := &instancesRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.IntVar(&c.limit, "limit", 20, "How many instances to return or 0 for all.")
			return c
		},
	}
}

type instancesRun struct {
	cipdSubcommand
	clientOptions

	limit int
}

func (c *instancesRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(listInstances(ctx, args[0], c.limit, c.clientOptions))
}

func listInstances(ctx context.Context, pkg string, limit int, clientOpts clientOptions) (*instancesOutput, error) {
	pkg, err := expandTemplate(pkg)
	if err != nil {
		return nil, err
	}

	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	// TODO(vadimsh): The backend currently doesn't support retrieving
	// per-instance refs when listing instances. Instead we fetch ALL refs in
	// parallel and then merge this information with the instance listing. This
	// works fine for packages with few refs (up to 50 maybe), but horribly
	// inefficient if the cardinality of the set of all refs is larger than a
	// typical size of instance listing (we spend time fetching data we don't
	// need). To support this case better, the backend should learn to maintain
	// {instance ID => ref} mapping (in addition to {ref => instance ID} mapping
	// it already has). This would require back filling all existing entities.

	// Fetch the refs in parallel with the first page of results. We merge them
	// with the list of instances during the display.
	type refsMap map[string][]string // instance ID => list of refs
	type refsOrErr struct {
		refs refsMap
		err  error
	}
	refsChan := make(chan refsOrErr, 1)
	go func() {
		defer close(refsChan)
		asMap := refsMap{}
		refs, err := client.FetchPackageRefs(ctx, pkg)
		for _, info := range refs {
			asMap[info.InstanceID] = append(asMap[info.InstanceID], info.Ref)
		}
		refsChan <- refsOrErr{asMap, err}
	}()

	enum, err := client.ListInstances(ctx, pkg)
	if err != nil {
		return nil, err
	}

	formatRow := func(instanceID, when, who, refs string) string {
		if len(who) > 25 {
			who = who[:22] + "..."
		}
		return fmt.Sprintf("%-44s │ %-21s │ %-25s │ %-12s", instanceID, when, who, refs)
	}

	var refs refsMap // populated on after fetching first page

	out := []instanceInfoWithRefs{}
	for {
		pageSize := 200
		if limit != 0 && limit-len(out) < pageSize {
			pageSize = limit - len(out)
			if pageSize == 0 {
				// Fetched everything we wanted. There's likely more instances available
				// (unless '-limit' happens to exactly match number of instances on the
				// backend, which is not very probable). Hint this by printing '...'.
				fmt.Println(formatRow("...", "...", "...", "..."))
				break
			}
		}
		page, err := enum.Next(ctx, pageSize)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			if len(out) == 0 {
				fmt.Println("No instances found")
			}
			break // no more results to fetch
		}

		if len(out) == 0 {
			// Need to wait for refs to be fetched, they are required to display
			// "Refs" column.
			refsOrErr := <-refsChan
			if refsOrErr.err != nil {
				return nil, refsOrErr.err
			}
			refs = refsOrErr.refs

			// Draw the header now that we have some real results (i.e no errors).
			hdr := formatRow("Instance ID", "Timestamp", "Uploader", "Refs")
			fmt.Println(hdr)
			fmt.Println(strings.Repeat("─", len(hdr)))
		}

		for _, info := range page {
			instanceRefs := refs[info.Pin.InstanceID]
			out = append(out, instanceInfoWithRefs{
				InstanceInfo: info,
				Refs:         instanceRefs,
			})
			fmt.Println(formatRow(
				info.Pin.InstanceID,
				time.Time(info.RegisteredTs).Local().Format("Jan 02 15:04 MST 2006"),
				strings.TrimPrefix(info.RegisteredBy, "user:"),
				strings.Join(instanceRefs, " ")))
		}
	}

	return &instancesOutput{out}, nil
}

////////////////////////////////////////////////////////////////////////////////
// 'set-ref' subcommand.

func cmdSetRef(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "set-ref <package or package prefix> [options]",
		ShortDesc: "moves a ref to point to a given version",
		LongDesc:  "Moves a ref to point to a given version.",
		CommandRun: func() subcommands.CommandRun {
			c := &setRefRun{}
			c.registerBaseFlags()
			c.refsOptions.registerFlags(&c.Flags)
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>", "Package version to point the ref to.")
			return c
		},
	}
}

type setRefRun struct {
	cipdSubcommand
	refsOptions
	clientOptions

	version string
}

func (c *setRefRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	if len(c.refs) == 0 {
		return c.done(nil, makeCLIError("at least one -ref must be provided"))
	}
	pkgPrefix, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.doneWithPins(visitPins(ctx, &visitPinsArgs{
		clientOptions: c.clientOptions,
		packagePrefix: pkgPrefix,
		version:       c.version,
		updatePin: func(client cipd.Client, pin common.Pin) error {
			for _, ref := range c.refs {
				if err := client.SetRefWhenReady(ctx, ref, pin); err != nil {
					return err
				}
			}
			return nil
		},
	}))
}

type visitPinsArgs struct {
	clientOptions

	packagePrefix string
	version       string

	updatePin func(client cipd.Client, pin common.Pin) error
}

func visitPins(ctx context.Context, args *visitPinsArgs) ([]pinInfo, error) {
	client, err := args.clientOptions.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	// Do not touch anything if some packages do not have requested version. So
	// resolve versions first and only then move refs.
	pins, err := performBatchOperation(ctx, batchOperation{
		client:        client,
		packagePrefix: args.packagePrefix,
		callback: func(pkg string) (common.Pin, error) {
			return client.ResolveVersion(ctx, pkg, args.version)
		},
	})
	if err != nil {
		return nil, err
	}
	if hasErrors(pins) {
		printPinsAndError(map[string][]pinInfo{"": pins})
		return nil,
			cipderr.InvalidVersion.WithDetails(cipderr.Details{
				Version: args.version,
			}).Apply(errors.
				Fmt("can't find %q version in all packages, aborting", args.version))

	}

	// Prepare for the next batch call.
	packages := make([]string, len(pins))
	pinsToUse := make(map[string]common.Pin, len(pins))
	for i, p := range pins {
		packages[i] = p.Pkg
		pinsToUse[p.Pkg] = *p.Pin
	}

	// Update all refs or tags.
	return performBatchOperation(ctx, batchOperation{
		client:   client,
		packages: packages,
		callback: func(pkg string) (common.Pin, error) {
			pin := pinsToUse[pkg]
			if err := args.updatePin(client, pin); err != nil {
				return common.Pin{}, err
			}
			return pin, nil
		},
	})
}

////////////////////////////////////////////////////////////////////////////////
// 'set-tag' subcommand.

func cmdSetTag(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "set-tag <package or package prefix> -tag key:value [options]",
		ShortDesc: "tags package of a specific version",
		LongDesc:  "Tags package of a specific version",
		CommandRun: func() subcommands.CommandRun {
			c := &setTagRun{}
			c.registerBaseFlags()
			c.tagsOptions.registerFlags(&c.Flags)
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>",
				"Package version to resolve. Could also be a tag or a ref.")
			return c
		},
	}
}

type setTagRun struct {
	cipdSubcommand
	tagsOptions
	clientOptions

	version string
}

func (c *setTagRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	if len(c.tags) == 0 {
		return c.done(nil, makeCLIError("at least one -tag must be provided"))
	}
	pkgPrefix, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(visitPins(ctx, &visitPinsArgs{
		clientOptions: c.clientOptions,
		packagePrefix: pkgPrefix,
		version:       c.version,
		updatePin: func(client cipd.Client, pin common.Pin) error {
			return client.AttachTagsWhenReady(ctx, pin, c.tags)
		},
	}))
}

////////////////////////////////////////////////////////////////////////////////
// 'set-metadata' subcommand.

func cmdSetMetadata(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "set-metadata <package or package prefix> -metadata key:value -metadata-from-file key:path [options]",
		ShortDesc: "attaches metadata to an instance",
		LongDesc:  "Attaches metadata to an instance",
		CommandRun: func() subcommands.CommandRun {
			c := &setMetadataRun{}
			c.registerBaseFlags()
			c.metadataOptions.registerFlags(&c.Flags)
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>",
				"Package version to resolve. Could also be a tag or a ref.")
			return c
		},
	}
}

type setMetadataRun struct {
	cipdSubcommand
	metadataOptions
	clientOptions

	version string
}

func (c *setMetadataRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}

	ctx := cli.GetContext(a, c, env)

	md, err := c.metadataOptions.load(ctx)
	if err == nil && len(md) == 0 {
		err = makeCLIError("at least one -metadata or -metadata-from-file must be provided")
	}
	if err != nil {
		return c.done(nil, err)
	}

	pkgPrefix, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	return c.doneWithPins(visitPins(ctx, &visitPinsArgs{
		clientOptions: c.clientOptions,
		packagePrefix: pkgPrefix,
		version:       c.version,
		updatePin: func(client cipd.Client, pin common.Pin) error {
			return client.AttachMetadataWhenReady(ctx, pin, md)
		},
	}))
}

////////////////////////////////////////////////////////////////////////////////
// 'expand-package-name' subcommand.

func cmdExpandPackageName(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "expand-package-name",
		ShortDesc: "replaces any placeholder variables in the given package name",
		LongDesc: "Replaces any placeholder variables in the given package " +
			"name.\n If supplying a name using the feature ${var=possible,values} " +
			"an empty string will be returned if the expansion does not match the " +
			"current variable state.",
		Advanced: true,
		CommandRun: func() subcommands.CommandRun {
			c := &expandPackageNameRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			return c
		},
	}
}

type expandPackageNameRun struct {
	cipdSubcommand
	clientOptions
}

func (c *expandPackageNameRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 1) {
		return 1
	}

	if len(args) == 1 {
		path, err := template.DefaultExpander().Expand(args[0])
		if err != nil {
			if !errors.Is(err, template.ErrSkipTemplate) {
				return c.done(nil, err)
			}
			// return an empty string if the variable expansion does not
			// apply to current system.
			path = ""
		}

		fmt.Println(path)

		return c.done(path, nil)
	}

	return c.done("", makeCLIError("one package name must be supplied: %v", args))
}

////////////////////////////////////////////////////////////////////////////////
// 'ls' subcommand.

func cmdListPackages(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ls [-r] [<prefix string>]",
		ShortDesc: "lists matching packages on the server",
		LongDesc: "Queries the backend for a list of packages in the given path to " +
			"which the user has access, optionally recursively.",
		CommandRun: func() subcommands.CommandRun {
			c := &listPackagesRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.BoolVar(&c.recursive, "r", false, "Whether to list packages in subdirectories.")
			c.Flags.BoolVar(&c.showHidden, "h", false, "Whether also to list hidden packages.")
			return c
		},
	}
}

type listPackagesRun struct {
	cipdSubcommand
	clientOptions

	recursive  bool
	showHidden bool
}

func (c *listPackagesRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 1) {
		return 1
	}
	path, err := "", error(nil)
	if len(args) == 1 {
		path, err = expandTemplate(args[0])
		if err != nil {
			return c.done(nil, err)
		}
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(listPackages(ctx, path, c.recursive, c.showHidden, c.clientOptions))
}

func listPackages(ctx context.Context, path string, recursive, showHidden bool, clientOpts clientOptions) ([]string, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	packages, err := client.ListPackages(ctx, path, recursive, showHidden)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		fmt.Println("No matching packages.")
	} else {
		for _, p := range packages {
			fmt.Println(p)
		}
	}
	return packages, nil
}

////////////////////////////////////////////////////////////////////////////////
// 'search' subcommand.

func cmdSearch(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "search <package> -tag key:value [options]",
		ShortDesc: "searches for package instances by tag",
		LongDesc:  "Searches for instances of some package with all given tags.",
		CommandRun: func() subcommands.CommandRun {
			c := &searchRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.tagsOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type searchRun struct {
	cipdSubcommand
	clientOptions
	tagsOptions
}

func (c *searchRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	if len(c.tags) == 0 {
		return c.done(nil, makeCLIError("at least one -tag must be provided"))
	}
	packageName, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(searchInstances(ctx, packageName, c.tags, c.clientOptions))
}

func searchInstances(ctx context.Context, packageName string, tags []string, clientOpts clientOptions) ([]common.Pin, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	pins, err := client.SearchInstances(ctx, packageName, tags)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		fmt.Println("No matching instances.")
	} else {
		fmt.Println("Instances:")
		for _, pin := range pins {
			fmt.Printf("  %s\n", pin)
		}
	}
	return pins, nil
}

////////////////////////////////////////////////////////////////////////////////
// 'acl-list' subcommand.

func cmdListACL(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "acl-list <package subpath>",
		ShortDesc: "lists package path Access Control List",
		LongDesc:  "Lists package path Access Control List.",
		CommandRun: func() subcommands.CommandRun {
			c := &listACLRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			return c
		},
	}
}

type listACLRun struct {
	cipdSubcommand
	clientOptions
}

func (c *listACLRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	pkg, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(listACL(ctx, pkg, c.clientOptions))
}

func listACL(ctx context.Context, packagePath string, clientOpts clientOptions) (map[string][]cipd.PackageACL, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	acls, err := client.FetchACL(ctx, packagePath)
	if err != nil {
		return nil, err
	}

	// Split by role, drop empty ACLs.
	byRole := map[string][]cipd.PackageACL{}
	for _, a := range acls {
		if len(a.Principals) != 0 {
			byRole[a.Role] = append(byRole[a.Role], a)
		}
	}

	listRoleACL := func(title string, acls []cipd.PackageACL) {
		fmt.Printf("%s:\n", title)
		if len(acls) == 0 {
			fmt.Printf("  none\n")
			return
		}
		for _, a := range acls {
			fmt.Printf("  via %q:\n", a.PackagePath)
			for _, u := range a.Principals {
				fmt.Printf("    %s\n", u)
			}
		}
	}

	listRoleACL("Owners", byRole["OWNER"])
	listRoleACL("Writers", byRole["WRITER"])
	listRoleACL("Readers", byRole["READER"])

	return byRole, nil
}

////////////////////////////////////////////////////////////////////////////////
// 'acl-edit' subcommand.

// principalsList is used as custom flag value. It implements flag.Value.
type principalsList []string

func (l *principalsList) String() string {
	return fmt.Sprintf("%v", *l)
}

func (l *principalsList) Set(value string) error {
	// Ensure <type>:<id> syntax is used. Let the backend to validate the rest.
	chunks := strings.Split(value, ":")
	if len(chunks) != 2 {
		return makeCLIError("%q doesn't look like principal id (<type>:<id>)", value)
	}
	*l = append(*l, value)
	return nil
}

func cmdEditACL(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "acl-edit <package subpath> [options]",
		ShortDesc: "modifies package path Access Control List",
		LongDesc:  "Modifies package path Access Control List.",
		CommandRun: func() subcommands.CommandRun {
			c := &editACLRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.Var(&c.owner, "owner", "Users (user:email) or groups (`group:name`) to grant OWNER role.")
			c.Flags.Var(&c.writer, "writer", "Users (user:email) or groups (`group:name`) to grant WRITER role.")
			c.Flags.Var(&c.reader, "reader", "Users (user:email) or groups (`group:name`) to grant READER role.")
			c.Flags.Var(&c.revoke, "revoke", "Users (user:email) or groups (`group:name`) to remove from all roles.")
			return c
		},
	}
}

type editACLRun struct {
	cipdSubcommand
	clientOptions

	owner  principalsList
	writer principalsList
	reader principalsList
	revoke principalsList
}

func (c *editACLRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	pkg, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(nil, editACL(ctx, pkg, c.owner, c.writer, c.reader, c.revoke, c.clientOptions))
}

func editACL(ctx context.Context, packagePath string, owners, writers, readers, revoke principalsList, clientOpts clientOptions) error {
	changes := []cipd.PackageACLChange{}

	makeChanges := func(action cipd.PackageACLChangeAction, role string, list principalsList) {
		for _, p := range list {
			changes = append(changes, cipd.PackageACLChange{
				Action:    action,
				Role:      role,
				Principal: p,
			})
		}
	}

	makeChanges(cipd.GrantRole, "OWNER", owners)
	makeChanges(cipd.GrantRole, "WRITER", writers)
	makeChanges(cipd.GrantRole, "READER", readers)

	makeChanges(cipd.RevokeRole, "OWNER", revoke)
	makeChanges(cipd.RevokeRole, "WRITER", revoke)
	makeChanges(cipd.RevokeRole, "READER", revoke)

	if len(changes) == 0 {
		return nil
	}

	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close(ctx)

	err = client.ModifyACL(ctx, packagePath, changes)
	if err != nil {
		return err
	}
	fmt.Println("ACL changes applied.")
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// 'acl-check' subcommand.

func cmdCheckACL(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "acl-check <package subpath> [options]",
		ShortDesc: "checks whether the caller has given roles in a package",
		LongDesc:  "Checks whether the caller has given roles in a package.",
		CommandRun: func() subcommands.CommandRun {
			c := &checkACLRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.BoolVar(&c.owner, "owner", false, "Check for OWNER role.")
			c.Flags.BoolVar(&c.writer, "writer", false, "Check for WRITER role.")
			c.Flags.BoolVar(&c.reader, "reader", false, "Check for READER role.")
			return c
		},
	}
}

type checkACLRun struct {
	cipdSubcommand
	clientOptions

	owner  bool
	writer bool
	reader bool
}

func (c *checkACLRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}

	var roles []string
	if c.owner {
		roles = append(roles, "OWNER")
	}
	if c.writer {
		roles = append(roles, "WRITER")
	}
	if c.reader {
		roles = append(roles, "READER")
	}

	// By default, check for READER access.
	if len(roles) == 0 {
		roles = append(roles, "READER")
	}

	pkg, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(checkACL(ctx, pkg, roles, c.clientOptions))
}

func checkACL(ctx context.Context, packagePath string, roles []string, clientOpts clientOptions) (bool, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return false, err
	}
	defer client.Close(ctx)

	actualRoles, err := client.FetchRoles(ctx, packagePath)
	if err != nil {
		return false, err
	}
	roleSet := stringset.NewFromSlice(actualRoles...)

	var missing []string
	for _, r := range roles {
		if !roleSet.Has(r) {
			missing = append(missing, r)
		}
	}

	if len(missing) == 0 {
		fmt.Printf("The caller has all requested role(s): %s\n", strings.Join(roles, ", "))
		return true, nil
	}

	fmt.Printf("The caller doesn't have following role(s): %s\n", strings.Join(missing, ", "))
	return false, nil
}

////////////////////////////////////////////////////////////////////////////////
// 'pkg-build' subcommand.

func cmdBuild() *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-build [options]",
		ShortDesc: "builds a package instance file",
		LongDesc:  "Builds a package instance producing *.cipd file.",
		CommandRun: func() subcommands.CommandRun {
			c := &buildRun{}
			c.registerBaseFlags()
			c.inputOptions.registerFlags(&c.Flags)
			c.hashOptions.registerFlags(&c.Flags)
			c.Flags.StringVar(&c.outputFile, "out", "<path>", "Path to a file to write the final package to.")
			return c
		},
	}
}

type buildRun struct {
	cipdSubcommand
	inputOptions
	hashOptions

	outputFile string
}

func (c *buildRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	_, err := buildInstanceFile(ctx, c.outputFile, c.inputOptions, c.hashAlgo())
	if err != nil {
		return c.done(nil, err)
	}
	return c.done(inspectInstanceFile(ctx, c.outputFile, c.hashAlgo(), false))
}

func buildInstanceFile(ctx context.Context, instanceFile string, inputOpts inputOptions, algo api.HashAlgo) (common.Pin, error) {
	// Read the list of files to add to the package.
	buildOpts, err := inputOpts.prepareInput()
	if err != nil {
		return common.Pin{}, err
	}

	// Prepare the destination, update build options with io.Writer to it.
	out, err := os.OpenFile(instanceFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("opening instance file for writing: %w", err))
	}
	buildOpts.Output = out
	buildOpts.HashAlgo = algo

	// Build the package.
	pin, err := builder.BuildInstance(ctx, buildOpts)
	if err != nil {
		out.Close()
		os.Remove(instanceFile)
		return common.Pin{}, err
	}

	// Make sure it is flushed properly by ensuring Close succeeds.
	if err := out.Close(); err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("flushing built instance file: %w", err))
	}

	return pin, nil
}

////////////////////////////////////////////////////////////////////////////////
// 'pkg-deploy' subcommand.

func cmdDeploy() *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-deploy <package instance file> [options]",
		ShortDesc: "deploys a package instance file",
		LongDesc:  "Deploys a *.cipd package instance into a site root.",
		CommandRun: func() subcommands.CommandRun {
			c := &deployRun{}
			c.registerBaseFlags()
			c.hashOptions.registerFlags(&c.Flags)
			c.maxThreadsOption.registerFlags(&c.Flags)
			c.Flags.StringVar(&c.rootDir, "root", "<path>", "Path to an installation site root directory.")
			c.Flags.Var(&c.overrideInstallMode, "install-mode",
				"Deploy using this mode instead, even if the instance manifest specifies otherwise.")
			return c
		},
	}
}

type deployRun struct {
	cipdSubcommand
	hashOptions
	maxThreadsOption

	rootDir             string
	overrideInstallMode pkg.InstallMode
}

func (c *deployRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	maxThreads, err := c.loadMaxThreads(ctx)
	if err != nil {
		return c.done(nil, err)
	}
	return c.done(deployInstanceFile(ctx, c.rootDir, args[0], c.hashAlgo(), maxThreads, c.overrideInstallMode))
}

func deployInstanceFile(ctx context.Context, root, instanceFile string, hashAlgo api.HashAlgo, maxThreads int, overrideInstallMode pkg.InstallMode) (common.Pin, error) {
	inst, err := reader.OpenInstanceFile(ctx, instanceFile, reader.OpenInstanceOpts{
		VerificationMode: reader.CalculateHash,
		HashAlgo:         hashAlgo,
	})
	if err != nil {
		return common.Pin{}, err
	}
	defer inst.Close(ctx, false)

	inspectInstance(ctx, inst, false)

	d := deployer.New(root)
	defer d.FS().CleanupTrash(ctx)

	// TODO(iannucci): add subdir arg to deployRun

	return d.DeployInstance(ctx, "", inst, overrideInstallMode, maxThreads)
}

////////////////////////////////////////////////////////////////////////////////
// 'pkg-fetch' subcommand.

func cmdFetch(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-fetch <package> [options]",
		ShortDesc: "fetches a package instance file from the repository",
		LongDesc:  "Fetches a package instance file from the repository.",
		CommandRun: func() subcommands.CommandRun {
			c := &fetchRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>", "Package version to fetch.")
			c.Flags.StringVar(&c.outputPath, "out", "<path>", "Path to a file to write fetch to.")
			return c
		},
	}
}

type fetchRun struct {
	cipdSubcommand
	clientOptions

	version    string
	outputPath string
}

func (c *fetchRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	pkg, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(fetchInstanceFile(ctx, pkg, c.version, c.outputPath, c.clientOptions))
}

func fetchInstanceFile(ctx context.Context, packageName, version, instanceFile string, clientOpts clientOptions) (pin common.Pin, err error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return common.Pin{}, err
	}
	defer client.Close(ctx)

	defer func() {
		cipderr.AttachDetails(&err, cipderr.Details{
			Package: packageName,
			Version: version,
		})
	}()

	pin, err = client.ResolveVersion(ctx, packageName, version)
	if err != nil {
		return common.Pin{}, err
	}

	out, err := os.OpenFile(instanceFile, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("opening the instance file for writing: %w", err))
	}
	ok := false
	defer func() {
		if !ok {
			out.Close()
			os.Remove(instanceFile)
		}
	}()

	err = client.FetchInstanceTo(ctx, pin, out)
	if err != nil {
		return common.Pin{}, err
	}

	if err := out.Close(); err != nil {
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("flushing fetched instance file: %w", err))
	}
	ok = true

	// Print information about the instance. 'FetchInstanceTo' already verified
	// the hash.
	inst, err := reader.OpenInstanceFile(ctx, instanceFile, reader.OpenInstanceOpts{
		VerificationMode: reader.SkipHashVerification,
		InstanceID:       pin.InstanceID,
	})
	if err != nil {
		os.Remove(instanceFile)
		return common.Pin{}, err
	}
	defer inst.Close(ctx, false)
	inspectInstance(ctx, inst, false)
	return inst.Pin(), nil
}

////////////////////////////////////////////////////////////////////////////////
// 'pkg-inspect' subcommand.

func cmdInspect() *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-inspect <package instance file>",
		ShortDesc: "inspects contents of a package instance file",
		LongDesc:  "Reads contents *.cipd file and prints information about it.",
		CommandRun: func() subcommands.CommandRun {
			c := &inspectRun{}
			c.registerBaseFlags()
			c.hashOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type inspectRun struct {
	cipdSubcommand
	hashOptions
}

func (c *inspectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(inspectInstanceFile(ctx, args[0], c.hashAlgo(), true))
}

func inspectInstanceFile(ctx context.Context, instanceFile string, hashAlgo api.HashAlgo, listFiles bool) (common.Pin, error) {
	inst, err := reader.OpenInstanceFile(ctx, instanceFile, reader.OpenInstanceOpts{
		VerificationMode: reader.CalculateHash,
		HashAlgo:         hashAlgo,
	})
	if err != nil {
		return common.Pin{}, err
	}
	defer inst.Close(ctx, false)
	inspectInstance(ctx, inst, listFiles)
	return inst.Pin(), nil
}

func inspectPin(ctx context.Context, pin common.Pin) {
	fmt.Printf("Instance: %s\n", pin)
}

func inspectInstance(ctx context.Context, inst pkg.Instance, listFiles bool) {
	inspectPin(ctx, inst.Pin())
	if listFiles {
		fmt.Println("Package files:")
		for _, f := range inst.Files() {
			if f.Symlink() {
				target, err := f.SymlinkTarget()
				if err != nil {
					fmt.Printf(" E %s (%s)\n", f.Name(), err)
				} else {
					fmt.Printf(" S %s -> %s\n", f.Name(), target)
				}
			} else {
				flags := make([]string, 0, 3)
				if f.Executable() {
					flags = append(flags, "+x")
				}
				if f.WinAttrs()&fs.WinAttrHidden != 0 {
					flags = append(flags, "+H")
				}
				if f.WinAttrs()&fs.WinAttrSystem != 0 {
					flags = append(flags, "+S")
				}
				flagText := ""
				if len(flags) > 0 {
					flagText = fmt.Sprintf(" (%s)", strings.Join(flags, ""))
				}
				fmt.Printf(" F %s%s\n", f.Name(), flagText)
			}
		}
	}
}

////////////////////////////////////////////////////////////////////////////////
// 'pkg-register' subcommand.

func cmdRegister(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-register <package instance file>",
		ShortDesc: "uploads and registers package instance in the package repository",
		LongDesc:  "Uploads and registers package instance in the package repository.",
		CommandRun: func() subcommands.CommandRun {
			c := &registerRun{}
			c.registerBaseFlags()
			c.Opts.refsOptions.registerFlags(&c.Flags)
			c.Opts.tagsOptions.registerFlags(&c.Flags)
			c.Opts.metadataOptions.registerFlags(&c.Flags)
			c.Opts.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Opts.uploadOptions.registerFlags(&c.Flags)
			c.Opts.hashOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type registerOpts struct {
	refsOptions
	tagsOptions
	metadataOptions
	clientOptions
	uploadOptions
	hashOptions
}

type registerRun struct {
	cipdSubcommand

	Opts registerOpts
}

func (c *registerRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(registerInstanceFile(ctx, args[0], nil, &c.Opts))
}

func registerInstanceFile(ctx context.Context, instanceFile string, knownPin *common.Pin, opts *registerOpts) (common.Pin, error) {
	// Load metadata, in particular process -metadata-from-file, which may fail.
	metadata, err := opts.metadataOptions.load(ctx)
	if err != nil {
		return common.Pin{}, err
	}

	src, err := pkg.NewFileSource(instanceFile)
	if err != nil {
		if os.IsNotExist(err) {
			return common.Pin{}, cipderr.BadArgument.Apply(errors.Fmt("missing input instance file: %w", err))
		}
		return common.Pin{}, cipderr.IO.Apply(errors.Fmt("opening input instance file: %w", err))
	}
	defer src.Close(ctx, false)

	// Calculate the pin if not yet known.
	var pin common.Pin
	if knownPin != nil {
		pin = *knownPin
	} else {
		pin, err = reader.CalculatePin(ctx, src, opts.hashAlgo())
		if err != nil {
			return common.Pin{}, err
		}
	}
	inspectPin(ctx, pin)

	client, err := opts.clientOptions.makeCIPDClient(ctx)
	if err != nil {
		return common.Pin{}, err
	}
	defer client.Close(ctx)

	err = client.RegisterInstance(ctx, pin, src, opts.uploadOptions.verificationTimeout)
	if err != nil {
		return common.Pin{}, err
	}
	err = attachAndMove(ctx, client, pin, metadata, opts.tags, opts.refs)
	if err != nil {
		return common.Pin{}, err
	}
	return pin, nil
}

func attachAndMove(ctx context.Context, client cipd.Client, pin common.Pin, md []cipd.Metadata, tags tagList, refs refList) error {
	if err := client.AttachMetadataWhenReady(ctx, pin, md); err != nil {
		return err
	}
	if err := client.AttachTagsWhenReady(ctx, pin, tags); err != nil {
		return err
	}
	for _, ref := range refs {
		if err := client.SetRefWhenReady(ctx, ref, pin); err != nil {
			return err
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// 'selfupdate' subcommand.

func cmdSelfUpdate(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "selfupdate -version <version> | -version-file <path>",
		ShortDesc: "updates the current CIPD client binary",
		LongDesc: "Does an in-place upgrade to the current CIPD binary.\n\n" +
			"Reads the version either from the command line (when using -version) or " +
			"from a file (when using -version-file). When using -version-file, also " +
			"loads special *.digests file (from <version-file>.digests path) with " +
			"pinned hashes of the client binary for all platforms. When selfupdating, " +
			"the client will verify the new downloaded binary has a hash specified in " +
			"the *.digests file.",
		CommandRun: func() subcommands.CommandRun {
			c := &selfupdateRun{}

			// By default, show a reduced number of logs unless something goes wrong.
			c.logConfig.Level = logging.Warning

			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "", "Version of the client to update to (incompatible with -version-file).")
			c.Flags.StringVar(&c.versionFile, "version-file", "",
				"Indicates the path to read the new version from (<version-file> itself) and "+
					"the path to the file with pinned hashes of the CIPD binary (<version-file>.digests file).")
			return c
		},
	}
}

type selfupdateRun struct {
	cipdSubcommand
	clientOptions

	version     string
	versionFile string
}

func (c *selfupdateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}

	ctx := cli.GetContext(a, c, env)

	switch {
	case c.version != "" && c.versionFile != "":
		return c.done(nil, makeCLIError("-version and -version-file are mutually exclusive, use only one"))
	case c.version == "" && c.versionFile == "":
		return c.done(nil, makeCLIError("either -version or -version-file are required"))
	}

	var version = c.version
	var digests *digests.ClientDigestsFile

	if version == "" { // using -version-file instead? load *.digests
		var err error
		version, err = loadClientVersion(c.versionFile)
		if err != nil {
			return c.done(nil, err)
		}
		digests, err = loadClientDigests(c.versionFile + digestsSfx)
		if err != nil {
			return c.done(nil, err)
		}
	}

	return c.done(func() (common.Pin, error) {
		exePath, err := os.Executable()
		if err != nil {
			return common.Pin{}, err
		}
		opts, err := c.clientOptions.toCIPDClientOpts(ctx)
		if err != nil {
			return common.Pin{}, err
		}
		return cipd.MaybeUpdateClient(ctx, opts, version, exePath, digests)
	}())
}

////////////////////////////////////////////////////////////////////////////////
// 'selfupdate-roll' subcommand.

func cmdSelfUpdateRoll(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "selfupdate-roll -version-file <path> (-version <version> | -check)",
		ShortDesc: "generates or checks the client version and *.digests files",
		LongDesc: "Generates or checks the client version and *.digests files.\n\n" +
			"When -version is specified, takes its value as CIPD client version, " +
			"resolves it into a list of hashes of the client binary at this version " +
			"for all known platforms, and (on success) puts the version into a file " +
			"specified by -version-file (referred to as <version-file> below), and " +
			"all hashes into <version-file>.digests file. They are later used by " +
			"'selfupdate -version-file <version-file>' to verify validity of the " +
			"fetched binary.\n\n" +
			"If -version is not specified, reads it from <version-file> and generates " +
			"<version-file>.digests file based on it.\n\n" +
			"When using -check, just verifies hashes in the <version-file>.digests " +
			"file match the version recorded in the <version-file>.",
		CommandRun: func() subcommands.CommandRun {
			c := &selfupdateRollRun{}

			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "", "Version of the client to roll to.")
			c.Flags.StringVar(&c.versionFile, "version-file", "<version-file>",
				"Indicates the path to a file with the version (<version-file> itself) and "+
					"the path to the file with pinned hashes of the CIPD binary (<version-file>.digests file).")
			c.Flags.BoolVar(&c.check, "check", false, "If set, checks that the file with "+
				"pinned hashes of the CIPD binary (<version-file>.digests file) is up-to-date.")
			return c
		},
	}
}

type selfupdateRollRun struct {
	cipdSubcommand
	clientOptions

	version     string
	versionFile string
	check       bool
}

func (c *selfupdateRollRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}

	ctx := cli.GetContext(a, c, env)
	client, err := c.clientOptions.makeCIPDClient(ctx)
	if err != nil {
		return c.done(nil, err)
	}
	defer client.Close(ctx)

	if c.check {
		if c.version != "" {
			return c.done(nil, makeCLIError("-version should not be used in -check mode"))
		}
		version, err := loadClientVersion(c.versionFile)
		if err != nil {
			return c.done(nil, err)
		}
		return c.doneWithPins(checkClientDigests(ctx, client, c.versionFile+digestsSfx, version))
	}

	// Grab the version from the command line and fallback to the -version-file
	// otherwise. The fallback is useful when we just want to regenerate *.digests
	// without touching the version file itself.
	version := c.version
	if version == "" {
		var err error
		version, err = loadClientVersion(c.versionFile)
		if err != nil {
			return c.done(nil, err)
		}
	}

	// It really makes sense to pin only tags. Warn about that. Still proceed,
	// maybe users are using refs and do not move them by convention.
	switch {
	case common.ValidateInstanceID(version, common.AnyHash) == nil:
		return c.done(nil,
			cipderr.BadArgument.Apply(errors.New("expecting a version identifier that can be "+
				"resolved for all per-platform CIPD client packages, not a concrete instance ID")))
	case common.ValidateInstanceTag(version) != nil:
		fmt.Printf(
			"WARNING! Version %q is not a tag. The hash pinning in *.digests file is "+
				"only useful for unmovable version identifiers. Proceeding, assuming "+
				"the immutability of %q is maintained manually. If it moves, selfupdate "+
				"will break due to *.digests file no longer matching the packages!\n\n",
			version, version)
	}

	pins, err := generateClientDigests(ctx, client, c.versionFile+digestsSfx, version)
	if err != nil {
		return c.doneWithPins(pins, err)
	}

	if c.version != "" {
		if err := os.WriteFile(c.versionFile, []byte(c.version+"\n"), 0666); err != nil {
			return c.done(nil, cipderr.IO.Apply(errors.Fmt("writing the client version file: %w", err)))
		}
	}

	return c.doneWithPins(pins, nil)
}

////////////////////////////////////////////////////////////////////////////////

const digestsSfx = ".digests"

func generateClientDigests(ctx context.Context, client cipd.Client, path, version string) ([]pinInfo, error) {
	digests, pins, err := assembleClientDigests(ctx, client, version)
	if err != nil {
		return pins, err
	}

	buf := bytes.Buffer{}
	versionFileName := strings.TrimSuffix(filepath.Base(path), digestsSfx)
	if err := digests.Serialize(&buf, version, versionFileName); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, buf.Bytes(), 0666); err != nil {
		return nil, cipderr.IO.Apply(errors.Fmt("writing client digests file: %w", err))
	}

	fmt.Printf("The pinned client hashes have been written to %s.\n\n", filepath.Base(path))
	return pins, nil
}

func checkClientDigests(ctx context.Context, client cipd.Client, path, version string) ([]pinInfo, error) {
	existing, err := loadClientDigests(path)
	if err != nil {
		return nil, err
	}
	digests, pins, err := assembleClientDigests(ctx, client, version)
	if err != nil {
		return pins, err
	}
	if !digests.Equal(existing) {
		base := filepath.Base(path)
		return nil,

			cipderr.Stale.Apply(errors.Fmt("the file with pinned client hashes (%s) is stale, "+
				"use 'cipd selfupdate-roll -version-file %s' to update it",
				base, strings.TrimSuffix(base, digestsSfx)))
	}
	fmt.Printf("The file with pinned client hashes (%s) is up-to-date.\n\n", filepath.Base(path))
	return pins, nil
}

// loadClientVersion reads a version string from a file.
func loadClientVersion(path string) (string, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return "", cipderr.IO.Apply(errors.Fmt("reading client version file: %w", err))
	}
	version := strings.TrimSpace(string(blob))
	if err := common.ValidateInstanceVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

// loadClientDigests loads the *.digests file with client binary hashes.
func loadClientDigests(path string) (*digests.ClientDigestsFile, error) {
	switch f, err := os.Open(path); {
	case os.IsNotExist(err):
		base := filepath.Base(path)
		return nil,

			cipderr.Stale.Apply(errors.Fmt("the file with pinned client hashes (%s) doesn't exist, "+
				"use 'cipd selfupdate-roll -version-file %s' to generate it",
				base, strings.TrimSuffix(base, digestsSfx)))
	case err != nil:
		return nil, cipderr.IO.Apply(errors.Fmt("error reading client digests file: %w", err))
	default:
		defer f.Close()
		return digests.ParseClientDigestsFile(f)
	}
}

// assembleClientDigests produces the digests file by making backend RPCs.
func assembleClientDigests(ctx context.Context, c cipd.Client, version string) (*digests.ClientDigestsFile, []pinInfo, error) {
	if !strings.HasSuffix(cipd.ClientPackage, "/${platform}") {
		panic(fmt.Sprintf("client package template (%q) is expected to end with '${platform}'", cipd.ClientPackage))
	}

	out := &digests.ClientDigestsFile{}
	mu := sync.Mutex{}

	// Ask the backend to give us hashes of the client binary for all platforms.
	pins, err := performBatchOperation(ctx, batchOperation{
		client:        c,
		packagePrefix: strings.TrimSuffix(cipd.ClientPackage, "${platform}"),
		callback: func(pkg string) (common.Pin, error) {
			pin, err := c.ResolveVersion(ctx, pkg, version)
			if err != nil {
				return common.Pin{}, err
			}
			desc, err := c.DescribeClient(ctx, pin)
			if err != nil {
				return common.Pin{}, err
			}
			mu.Lock()
			defer mu.Unlock()
			plat := pkg[strings.LastIndex(pkg, "/")+1:]
			if err := out.AddClientRef(plat, desc.Digest); err != nil {
				return common.Pin{}, err
			}
			return pin, nil
		},
	})
	switch {
	case err != nil:
		return nil, pins, err
	case hasErrors(pins):
		return nil, pins, cipderr.RPC.Apply(errors.New("failed to obtain the client binary digest for all platforms"))
	}

	out.Sort()
	return out, pins, nil
}

////////////////////////////////////////////////////////////////////////////////
// 'deployment-check' subcommand.

func cmdCheckDeployment(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "deployment-check [options]",
		ShortDesc: "verifies all files that are supposed to be installed are present",
		LongDesc: "Compares CIPD package manifests stored in .cipd/* with what's on disk.\n\n" +
			"Useful when debugging issues with broken installations.",
		CommandRun: func() subcommands.CommandRun {
			c := &checkDeploymentRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withoutMaxThreads)
			return c
		},
	}
}

type checkDeploymentRun struct {
	cipdSubcommand
	clientOptions
}

func (c *checkDeploymentRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(checkDeployment(ctx, c.clientOptions))
}

func checkDeployment(ctx context.Context, clientOpts clientOptions) (cipd.ActionMap, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	currentDeployment, err := client.FindDeployed(ctx)
	if err != nil {
		return nil, err
	}

	actions, err := client.EnsurePackages(ctx, currentDeployment, &cipd.EnsureOptions{
		Paranoia: cipd.CheckIntegrity,
		DryRun:   true,
		Silent:   true,
	})
	if err != nil {
		return nil, err
	}
	actions.Log(ctx, true)
	if len(actions) != 0 {
		err = cipderr.Stale.Apply(errors.New("the deployment needs a repair"))
	}
	return actions, err
}

////////////////////////////////////////////////////////////////////////////////
// 'deployment-repair' subcommand.

func cmdRepairDeployment(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "deployment-repair [options]",
		ShortDesc: "attempts to repair a deployment if it is broken",
		LongDesc:  "This is equivalent of running 'ensure' in paranoia.",
		CommandRun: func() subcommands.CommandRun {
			c := &repairDeploymentRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withMaxThreads)
			return c
		},
	}
}

type repairDeploymentRun struct {
	cipdSubcommand
	clientOptions
}

func (c *repairDeploymentRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(repairDeployment(ctx, c.clientOptions))
}

func repairDeployment(ctx context.Context, clientOpts clientOptions) (cipd.ActionMap, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	currentDeployment, err := client.FindDeployed(ctx)
	if err != nil {
		return nil, err
	}

	return client.EnsurePackages(ctx, currentDeployment, &cipd.EnsureOptions{
		Paranoia: cipd.CheckIntegrity,
	})
}

////////////////////////////////////////////////////////////////////////////////
// 'proxy' subcommand.

func cmdProxy(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "proxy <flags>",
		ShortDesc: "Runs a local CIPD proxy",
		LongDesc:  "Runs a local CIPD proxy.",
		CommandRun: func() subcommands.CommandRun {
			c := &proxyRun{}
			c.registerBaseFlags()
			c.authFlags.Register(&c.Flags, params.DefaultAuthOptions)
			c.authFlags.RegisterCredentialHelperFlags(&c.Flags)
			c.authFlags.RegisterADCFlags(&c.Flags)
			c.Flags.StringVar(&c.unixSocket, "unix-socket", "", "Unix domain socket path to serve the proxy on (default to a temp file).")
			c.Flags.StringVar(&c.proxyPolicy, "proxy-policy", "-", "Path to a text protobuf file with cipd.proxy.Policy message (or - for reading from stdin).")
			return c
		},
	}
}

type proxyRun struct {
	cipdSubcommand
	authFlags authcli.Flags

	unixSocket  string // -unix-socket flag
	proxyPolicy string // -proxy-policy flag
}

func (c *proxyRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(runProxy(ctx, c.authFlags, c.unixSocket, c.proxyPolicy))
}

func runProxy(ctx context.Context, authFlags authcli.Flags, unixSocket, proxyPolicy string) (*proxyserver.ProxyStats, error) {
	if unixSocket == "" {
		dir, err := os.MkdirTemp("", "cipd")
		if err != nil {
			return nil, cipderr.IO.Apply(errors.Fmt("failed to create a temp dir: %w", err))
		}
		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				logging.Warningf(ctx, "Failed to cleanup CIPD proxy temp dir: %s", err)
			}
		}()
		unixSocket = filepath.Join(dir, "proxy.unix")
	}
	policy, err := readProxyPolicy(ctx, proxyPolicy)
	if err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("bad proxy policy file: %w", err))
	}
	authOpts, err := authFlags.Options()
	if err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("bad auth options: %w", err))
	}
	authClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).Client()
	if err != nil {
		return nil, cipderr.Auth.Apply(errors.Fmt("initializing auth client: %w", err))
	}
	return runProxyImpl(ctx, unixSocket, policy, authClient)
}

func readProxyPolicy(ctx context.Context, path string) (*proxypb.Policy, error) {
	var file *os.File
	if path == "-" {
		logging.Infof(ctx, "Reading CIPD proxy policy from stdin...")
		file = os.Stdin
	} else {
		var err error
		file, err = os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, cipderr.BadArgument.Apply(errors.Fmt("missing proxy policy file: %w", err))
			}
			return nil, cipderr.IO.Apply(errors.Fmt("reading proxy policy file: %w", err))
		}
		defer func() { _ = file.Close() }()
	}

	blob, err := io.ReadAll(file)
	if err != nil {
		return nil, cipderr.IO.Apply(errors.Fmt("reading proxy policy file: %w", err))
	}

	var policy proxypb.Policy
	if err := (prototext.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(blob, &policy); err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("malformed proxy policy file: %w", err))
	}
	return &policy, nil
}

////////////////////////////////////////////////////////////////////////////////
// Main.

// GetApplication returns cli.Application.
//
// It can be used directly by subcommands.Run(...), or nested into another
// application.
func GetApplication(params Parameters) *cli.Application {
	return &cli.Application{
		Name:  "cipd",
		Title: "Chrome Infra Package Deployer (" + cipd.UserAgent + ")",

		Context: func(ctx context.Context) context.Context {
			loggerConfig := gologger.LoggerConfig{
				Format: `[P%{pid} %{time:15:04:05.000} %{shortfile} %{level:.1s}] %{message}`,
				Out:    os.Stderr,
			}
			ctx, cancel := context.WithCancel(loggerConfig.Use(ctx))
			signals.HandleInterrupt(cancel)
			return ctx
		},

		EnvVars: map[string]subcommands.EnvVarDefinition{
			cipd.EnvConfigFile: {
				Advanced: true,
				ShortDesc: fmt.Sprintf(
					"Path to a config file to load instead of the default %q. If `-`, just ignore the default config file.",
					cipd.DefaultConfigFilePath(),
				),
			},
			cipd.EnvHTTPUserAgentPrefix: {
				Advanced:  true,
				ShortDesc: "Optional http User-Agent prefix.",
			},
			cipd.EnvCacheDir: {
				ShortDesc: "Directory with shared instance and tags cache " +
					"(-cache-dir, if given, takes precedence).",
			},
			cipd.EnvMaxThreads: {
				Advanced: true,
				ShortDesc: "Number of worker threads for extracting packages. " +
					"If 0 or negative, uses CPU count. (-max-threads, if given and not 0, takes precedence.)",
			},
			cipd.EnvParallelDownloads: {
				Advanced: true,
				ShortDesc: fmt.Sprintf("How many packages are allowed to be fetched concurrently. "+
					"If <=1, packages will be fetched sequentially. Default is %d.", cipd.DefaultParallelDownloads),
			},
			cipd.EnvAdmissionPlugin: {
				Advanced:  true,
				ShortDesc: "JSON-encoded list with a command line of a deployment admission plugin.",
			},
			cipd.EnvCIPDServiceURL: {
				Advanced:  true,
				ShortDesc: "Override CIPD service URL.",
			},
			cipd.EnvCIPDProxyURL: {
				Advanced:  true,
				ShortDesc: "If set, send requests here instead of the remote backend. Only unix://<path> is supported currently.",
			},
			envSimpleTerminalUI: {
				Advanced:  true,
				ShortDesc: "If set disables the fancy terminal UI with progress bars in favor of a simpler one that just logs to stderr.",
			},
		},

		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			versioncli.CmdVersion(cipd.UserAgent),

			// Authentication related commands.
			{}, // These are spacers so that the commands appear in groups.
			authcli.SubcommandInfoWithParams(authcli.CommandParams{
				Name:                     "auth-info",
				Advanced:                 true,
				AuthOptions:              params.DefaultAuthOptions,
				UseCredentialHelperFlags: true,
			}),
			authcli.SubcommandLogin(params.DefaultAuthOptions, "auth-login", false),
			authcli.SubcommandLogout(params.DefaultAuthOptions, "auth-logout", false),

			// High level read commands.
			{},
			cmdListPackages(params),
			cmdSearch(params),
			cmdResolve(params),
			cmdDescribe(params),
			cmdInstances(params),

			// High level remote write commands.
			{},
			cmdCreate(params),
			cmdAttach(params),
			cmdSetRef(params),
			cmdSetTag(params),
			cmdSetMetadata(params),

			// High level local write commands.
			{},
			cmdEnsure(params),
			cmdExport(params),
			cmdSelfUpdate(params),
			cmdSelfUpdateRoll(params),

			// Advanced ensure file operations.
			{Advanced: true},
			cmdEnsureFileVerify(params),
			cmdEnsureFileResolve(params),

			// User friendly subcommands that operates within a site root. Implemented
			// in friendly.go. These are advanced because they're half-baked.
			{Advanced: true},
			cmdInit(params),
			cmdInstall(params),
			cmdInstalled(params),

			// ACLs.
			{Advanced: true},
			cmdListACL(params),
			cmdEditACL(params),
			cmdCheckACL(params),

			// Low level pkg-* commands.
			{Advanced: true},
			cmdBuild(),
			cmdDeploy(),
			cmdFetch(params),
			cmdInspect(),
			cmdRegister(params),

			// Low level deployment-* commands.
			{Advanced: true},
			cmdCheckDeployment(params),
			cmdRepairDeployment(params),

			// Low level misc commands.
			{Advanced: true},
			cmdExpandPackageName(params),
			cmdPuppetCheckUpdates(params),

			// Utility commands.
			{Advanced: true},
			cmdProxy(params),
		},
	}
}

// Main runs the CIPD CLI.
func Main(params Parameters, args []string) int {
	return subcommands.Run(GetApplication(params), fixflagpos.FixSubcommands(args))
}
