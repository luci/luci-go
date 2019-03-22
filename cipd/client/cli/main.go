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
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/client/versioncli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/fixflagpos"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/auth/client/authcli"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/builder"
	"go.chromium.org/luci/cipd/client/cipd/deployer"
	"go.chromium.org/luci/cipd/client/cipd/digests"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
)

// TODO(vadimsh): Add some tests.

func expandTemplate(tmpl string) (pkg string, err error) {
	pkg, err = template.DefaultExpander().Expand(tmpl)
	if err != nil {
		err = commandLineError{err}
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
	// Err is not empty if pin related operation failed. Pin is nil in that case.
	Err string `json:"error,omitempty"`
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
	return ctx
}

// registerBaseFlags registers common flags used by all subcommands.
func (c *cipdSubcommand) registerBaseFlags() {
	// Minimum default logging level is Info. This accommodates subcommands that
	// don't explicitly set the log level, resulting in the zero value (Debug).
	if c.logConfig.Level < logging.Info {
		c.logConfig.Level = logging.Info
	}

	c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
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

// printError prints error to stderr (recognizing commandLineError).
func (c *cipdSubcommand) printError(err error) {
	if _, ok := err.(commandLineError); ok {
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
func (c *cipdSubcommand) writeJSONOutput(result interface{}, err error) error {
	// -json-output flag wasn't specified.
	if c.jsonOutput == "" {
		return err
	}

	// Prepare the body of the output file.
	var body struct {
		Error  string      `json:"error,omitempty"`
		Result interface{} `json:"result,omitempty"`
	}
	if err != nil {
		body.Error = err.Error()
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

	e = ioutil.WriteFile(c.jsonOutput, out, 0666)
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
func (c *cipdSubcommand) done(result interface{}, err error) int {
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
	if len(pins) == 0 {
		if err == nil { // this error will be printed in c.done(...)
			fmt.Println("No packages.")
		}
	} else {
		printPinsAndError(pins)
	}
	if ret := c.done(pins, err); ret != 0 {
		return ret
	}
	for _, infos := range pins {
		if hasErrors(infos) {
			return 1
		}
	}
	return 0
}

// commandLineError is used to tag errors related to CLI.
type commandLineError struct {
	error
}

// makeCLIError returns new commandLineError.
func makeCLIError(msg string, args ...interface{}) error {
	return commandLineError{fmt.Errorf(msg, args...)}
}

////////////////////////////////////////////////////////////////////////////////
// clientOptions mixin.

type rootDirFlag bool

const (
	withRootDir    rootDirFlag = true
	withoutRootDir rootDirFlag = false
)

// clientOptions defines command line arguments related to CIPD client creation.
// Subcommands that need a CIPD client embed it.
type clientOptions struct {
	hardcoded Parameters // whatever was passed to registerFlags(...)

	serviceURL string // also mutated by loadEnsureFile
	cacheDir   string
	rootDir    string              // used only if registerFlags got withRootDir arg
	versions   ensure.VersionsFile // mutated by loadEnsureFile

	authFlags authcli.Flags
}

func (opts *clientOptions) resolvedServiceURL() string {
	if opts.serviceURL != "" {
		return opts.serviceURL
	}
	return opts.hardcoded.ServiceURL
}

func (opts *clientOptions) registerFlags(f *flag.FlagSet, params Parameters, rootDir rootDirFlag) {
	opts.hardcoded = params

	f.StringVar(&opts.serviceURL, "service-url", "",
		fmt.Sprintf(`Backend URL. If provided via an "ensure" file, the URL in the file takes precedence. `+
			`(default %s)`, params.ServiceURL))
	f.StringVar(&opts.cacheDir, "cache-dir", "",
		fmt.Sprintf("Directory for the shared cache (can also be set by %s env var).", cipd.EnvCacheDir))

	if rootDir {
		f.StringVar(&opts.rootDir, "root", "<path>", "Path to an installation site root directory.")
	}

	opts.authFlags.Register(f, params.DefaultAuthOptions)
}

func (opts *clientOptions) toCIPDClientOpts(ctx context.Context) (cipd.ClientOptions, error) {
	authOpts, err := opts.authFlags.Options()
	if err != nil {
		return cipd.ClientOptions{}, err
	}
	client, err := auth.NewAuthenticator(ctx, auth.OptionalLogin, authOpts).Client()
	if err != nil {
		return cipd.ClientOptions{}, err
	}

	realOpts := cipd.ClientOptions{
		ServiceURL:          opts.resolvedServiceURL(),
		Root:                opts.rootDir,
		CacheDir:            opts.cacheDir,
		Versions:            opts.versions,
		AuthenticatedClient: client,
		AnonymousClient:     http.DefaultClient,
	}
	if err := realOpts.LoadFromEnv(cli.MakeGetEnv(ctx)); err != nil {
		return cipd.ClientOptions{}, err
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
	// String() for empty vars used in -help output.
	if len(*vars) == 0 {
		return "key:value"
	}
	chunks := make([]string, 0, len(*vars))
	for k, v := range *vars {
		chunks = append(chunks, fmt.Sprintf("%s:%s", k, v))
	}
	sort.Strings(chunks)
	return strings.Join(chunks, ", ")
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
	f.StringVar(&opts.packageDef, "pkg-def", "", "*.yaml file that defines what to put into the package.")
	f.Var(&opts.vars, "pkg-var", "Variables accessible from package definition file.")

	// Interface to accept a single directory (alternative to -pkg-def).
	f.StringVar(&opts.packageName, "name", "", "Package name (unused with -pkg-def).")
	f.StringVar(&opts.inputDir, "in", "", "Path to a directory with files to package (unused with -pkg-def).")
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
			return empty, err
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
	// String() for empty vars used in -help output.
	if len(*refs) == 0 {
		return "ref"
	}
	return strings.Join(*refs, " ")
}

// Set is called by 'flag' package when parsing command line options.
func (refs *refList) Set(value string) error {
	err := common.ValidatePackageRef(value)
	if err != nil {
		return commandLineError{err}
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
	f.Var(&opts.refs, "ref", "A ref to point to the package instance (can be used multiple times).")
}

////////////////////////////////////////////////////////////////////////////////
// tagsOptions mixin.

// tagList holds an array of '-tag' command line options.
type tagList []string

func (tags *tagList) String() string {
	// String() for empty vars used in -help output.
	if len(*tags) == 0 {
		return "key:value"
	}
	return strings.Join(*tags, " ")
}

// Set is called by 'flag' package when parsing command line options.
func (tags *tagList) Set(value string) error {
	err := common.ValidateInstanceTag(value)
	if err != nil {
		return commandLineError{err}
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
	f.Var(&opts.tags, "tag", "A tag to attach to the package instance (can be used multiple times).")
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
		return fmt.Errorf("unknown hash algo %q, should be one of: %s", value, allAlgos)
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
		logging.Errorf(ctx,
			"For this feature to work, verification platforms must be specified in "+
				"the ensure file using one or more $VerifiedPlatform directives.")
		return nil, errors.New("no verification platforms configured")
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
		return nil, fmt.Errorf("no packages under %s", packagePrefix)
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
			return pinInfo{Pkg: pkg, Err: err.Error()}
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
	for i := 0; i < len(pkgs); i++ {
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
			if p.Err != "" {
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
				if p.Err != "" || p.Pin == nil {
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
				if p.Err != "" {
					fmt.Fprintf(os.Stderr, "  %s: %s.\n", p.Pkg, p.Err)
				}
			}
		}
	}
}

func hasErrors(pins []pinInfo) bool {
	for _, p := range pins {
		if p.Err != "" {
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
				pin := pin
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
		return nil, fmt.Errorf("the resolved versions file doesn't exist, "+
			"use 'cipd ensure-file-resolve -ensure-file %q' to generate it", ensureFile)
	case err != nil:
		return nil, err
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
	return ioutil.WriteFile(path, buf.Bytes(), 0666)
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
			c.Opts.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
	f, err := ioutil.TempFile("", "cipd_pkg")
	if err != nil {
		return common.Pin{}, err
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()
	pin, err := buildInstanceFile(ctx, f.Name(), opts.inputOptions, opts.hashAlgo())
	if err != nil {
		return common.Pin{}, err
	}
	return registerInstanceFile(ctx, f.Name(), &pin, &registerOpts{
		refsOptions:   opts.refsOptions,
		tagsOptions:   opts.tagsOptions,
		clientOptions: opts.clientOptions,
		uploadOptions: opts.uploadOptions,
		hashOptions:   opts.hashOptions,
	})
}

////////////////////////////////////////////////////////////////////////////////
// 'ensure' subcommand.

func cmdEnsure(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ensure [options]",
		ShortDesc: "installs, removes and updates packages in one go",
		LongDesc: "Installs, removes and updates packages in one go.\n\n" +
			"Supposed to be used from scripts and automation. Alternative to 'init', " +
			"'install' and 'remove'. As such, it doesn't try to discover site root " +
			"directory on its own.",
		CommandRun: func() subcommands.CommandRun {
			c := &ensureRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir)
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

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	resolver := cipd.Resolver{Client: client}
	resolved, err := resolver.Resolve(ctx, ef, template.DefaultExpander())
	if err != nil {
		return nil, nil, err
	}

	actions, err := client.EnsurePackages(ctx, resolved.PackagesBySubdir, resolved.ParanoidMode, dryRun)
	if err != nil {
		return nil, actions, err
	}

	if ensureFileOut != "" {
		buf := bytes.Buffer{}
		resolved.ServiceURL = clientOpts.resolvedServiceURL()
		resolved.ParanoidMode = ""
		if err = resolved.Serialize(&buf); err == nil {
			err = ioutil.WriteFile(ensureFileOut, buf.Bytes(), 0666)
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
		return c.done(nil, fmt.Errorf("the resolved versions file %s is stale, "+
			"use 'cipd ensure-file-resolve -ensure-file %q' to update it",
			filepath.Base(ef.ResolvedVersions), c.ensureFile))
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
		return c.done(nil, errors.New("no resolved versions file configured"))
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
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir)
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
	return performBatchOperation(ctx, batchOperation{
		client:        client,
		packagePrefix: packagePrefix,
		callback: func(pkg string) (common.Pin, error) {
			return client.ResolveVersion(ctx, pkg, version)
		},
	})
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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

	pin, err := client.ResolveVersion(ctx, pkg, version)
	if err != nil {
		return nil, err
	}

	desc, err := client.DescribeInstance(ctx, pin, &cipd.DescribeInstanceOpts{
		DescribeRefs: true,
		DescribeTags: true,
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
	return c.doneWithPins(setRefOrTag(ctx, &setRefOrTagArgs{
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

type setRefOrTagArgs struct {
	clientOptions

	packagePrefix string
	version       string

	updatePin func(client cipd.Client, pin common.Pin) error
}

func setRefOrTag(ctx context.Context, args *setRefOrTagArgs) ([]pinInfo, error) {
	client, err := args.clientOptions.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}

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
		return nil, fmt.Errorf("can't find %q version in all packages, aborting", args.version)
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
		UsageLine: "set-tag <package or package prefix> -tag=key:value [options]",
		ShortDesc: "tags package of a specific version",
		LongDesc:  "Tags package of a specific version",
		CommandRun: func() subcommands.CommandRun {
			c := &setTagRun{}
			c.registerBaseFlags()
			c.tagsOptions.registerFlags(&c.Flags)
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
			c.Flags.StringVar(&c.version, "version", "<version>",
				"Package version to resolve. Could also be itself a tag or ref")
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
	return c.done(setRefOrTag(ctx, &setRefOrTagArgs{
		clientOptions: c.clientOptions,
		packagePrefix: pkgPrefix,
		version:       c.version,
		updatePin: func(client cipd.Client, pin common.Pin) error {
			return client.AttachTagsWhenReady(ctx, pin, c.tags)
		},
	}))
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
	return pins, err
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
			c.Flags.Var(&c.owner, "owner", "Users or groups to grant OWNER role.")
			c.Flags.Var(&c.writer, "writer", "Users or groups to grant WRITER role.")
			c.Flags.Var(&c.reader, "reader", "Users or groups to grant READER role.")
			c.Flags.Var(&c.revoke, "revoke", "Users or groups to remove from all roles.")
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
		return common.Pin{}, err
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
		return common.Pin{}, err
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
			c.Flags.StringVar(&c.rootDir, "root", "<path>", "Path to an installation site root directory.")
			return c
		},
	}
}

type deployRun struct {
	cipdSubcommand
	hashOptions

	rootDir string
}

func (c *deployRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(deployInstanceFile(ctx, c.rootDir, args[0], c.hashAlgo()))
}

func deployInstanceFile(ctx context.Context, root, instanceFile string, hashAlgo api.HashAlgo) (common.Pin, error) {
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
	defer d.CleanupTrash(ctx)

	// TODO(iannucci): add subdir arg to deployRun

	return d.DeployInstance(ctx, "", inst)
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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

func fetchInstanceFile(ctx context.Context, packageName, version, instanceFile string, clientOpts clientOptions) (common.Pin, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return common.Pin{}, err
	}
	pin, err := client.ResolveVersion(ctx, packageName, version)
	if err != nil {
		return common.Pin{}, err
	}

	out, err := os.OpenFile(instanceFile, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return common.Pin{}, err
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

	out.Close()
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
			c.Opts.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
			c.Opts.uploadOptions.registerFlags(&c.Flags)
			c.Opts.hashOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type registerOpts struct {
	refsOptions
	tagsOptions
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
	src, err := os.Open(instanceFile)
	if err != nil {
		return common.Pin{}, err
	}
	defer src.Close()

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

	err = client.RegisterInstance(ctx, pin, src, opts.uploadOptions.verificationTimeout)
	if err != nil {
		return common.Pin{}, err
	}
	err = client.AttachTagsWhenReady(ctx, pin, opts.tagsOptions.tags)
	if err != nil {
		return common.Pin{}, err
	}
	for _, ref := range opts.refsOptions.refs {
		err = client.SetRefWhenReady(ctx, ref, pin)
		if err != nil {
			return common.Pin{}, err
		}
	}

	return pin, nil
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir)
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
		return c.done(nil, fmt.Errorf("expecting a version identifier that can be "+
			"resolved for all per-platform CIPD client packages, not a concrete instance ID"))
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
		if err := ioutil.WriteFile(c.versionFile, []byte(c.version+"\n"), 0666); err != nil {
			return c.done(nil, err)
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
	if err := ioutil.WriteFile(path, buf.Bytes(), 0666); err != nil {
		return nil, err
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
		return nil, fmt.Errorf("the file with pinned client hashes (%s) is stale, "+
			"use 'cipd selfupdate-roll -version-file %s' to update it",
			base, strings.TrimSuffix(base, digestsSfx))
	}
	fmt.Printf("The file with pinned client hashes (%s) is up-to-date.\n\n", filepath.Base(path))
	return pins, nil
}

// loadClientVersion reads a version string from a file.
func loadClientVersion(path string) (string, error) {
	blob, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
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
		return nil, fmt.Errorf("the file with pinned client hashes (%s) doesn't exist, "+
			"use 'cipd selfupdate-roll -version-file %s' to generate it",
			base, strings.TrimSuffix(base, digestsSfx))
	case err != nil:
		return nil, err
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
		return nil, pins, errors.New("failed to obtain the client binary digest for all platforms")
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
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir)
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
	actions, err := client.CheckDeployment(ctx, cipd.CheckIntegrity)
	if err != nil {
		return nil, err
	}
	actions.Log(ctx, true)
	if len(actions) != 0 {
		err = fmt.Errorf("the deployment needs a repair")
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
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir)
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
	return client.RepairDeployment(ctx, cipd.CheckIntegrity)
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
			return loggerConfig.Use(ctx)
		},

		EnvVars: map[string]subcommands.EnvVarDefinition{
			cipd.EnvHTTPUserAgentPrefix: {
				Advanced:  true,
				ShortDesc: "Optional http User-Agent prefix.",
			},
			cipd.EnvCacheDir: {
				ShortDesc: "Directory with shared instance and tags cache " +
					"(-cache-dir, if given, takes precedence).",
			},
		},

		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			versioncli.CmdVersion(cipd.UserAgent),

			// Authentication related commands.
			{}, // These are spacers so that the commands appear in groups.
			authcli.SubcommandInfo(params.DefaultAuthOptions, "auth-info", true),
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
			cmdSetRef(params),
			cmdSetTag(params),

			// High level local write commands.
			{},
			cmdEnsure(params),
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
			cmdPuppetCheckUpdates(params),
		},
	}
}

// Main runs the CIPD CLI.
//
func Main(params Parameters, args []string) int {
	return subcommands.Run(GetApplication(params), fixflagpos.FixSubcommands(args))
}
