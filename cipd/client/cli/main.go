// Copyright 2014 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cli

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/osext"
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"

	"github.com/luci/luci-go/client/authcli"

	"github.com/luci/luci-go/cipd/client/cipd"
	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/cipd/client/cipd/ensure"
	"github.com/luci/luci-go/cipd/client/cipd/local"
	"github.com/luci/luci-go/cipd/version"
)

// TODO(vadimsh): Add some tests.

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
	// Tracking is what ref is being tracked by that package in the site root.
	Tracking string `json:"tracking,omitempty"`
	// Err is not empty if pin related operation failed. Pin is nil in that case.
	Err string `json:"error,omitempty"`
}

// describeOutput defines JSON format for 'cipd describe' output.
type describeOutput struct {
	cipd.InstanceInfo
	Refs []cipd.RefInfo `json:"refs"`
	Tags []cipd.TagInfo `json:"tags"`
}

// cipdSubcommand is a base of all CIPD subcommands. It defines some common
// flags, such as logging and JSON output parameters.
type cipdSubcommand struct {
	subcommands.CommandRunBase

	jsonOutput string
	verbose    bool
}

// ModifyContext implements cli.ContextModificator.
func (c *cipdSubcommand) ModifyContext(ctx context.Context) context.Context {
	if c.verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	}
	return ctx
}

// registerBaseFlags registers common flags used by all subcommands.
func (c *cipdSubcommand) registerBaseFlags() {
	c.Flags.StringVar(&c.jsonOutput, "json-output", "", "Path to write operation results to.")
	c.Flags.BoolVar(&c.verbose, "verbose", false, "Enable more logging.")
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
		missing := []string{}
		for _, f := range unset {
			missing = append(missing, f.Name)
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
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s.\n", err)
	}
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

	e = ioutil.WriteFile(c.jsonOutput, out, 0600)
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
		fmt.Println("No packages.")
	} else {
		printPinsAndError(pins)
	}
	ret := c.done(pins, err)
	if hasErrors(pins) && ret == 0 {
		return 1
	}
	return ret
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

// clientOptions defines command line arguments related to CIPD client creation.
// Subcommands that need a CIPD client embed it.
type clientOptions struct {
	authFlags  authcli.Flags
	serviceURL string
	cacheDir   string
}

func (opts *clientOptions) registerFlags(f *flag.FlagSet, params Parameters) {
	f.StringVar(&opts.serviceURL, "service-url", params.ServiceURL,
		"Backend URL. If provided via an 'ensure file', the URL in the file takes precedence.")
	f.StringVar(&opts.cacheDir, "cache-dir", "",
		fmt.Sprintf("Directory for shared cache (can also be set by %s env var).", CIPDCacheDir))
	opts.authFlags.Register(f, params.DefaultAuthOptions)
}

func (opts *clientOptions) makeCipdClient(ctx context.Context, root string) (cipd.Client, error) {
	authOpts, err := opts.authFlags.Options()
	if err != nil {
		return nil, err
	}
	client, err := auth.NewAuthenticator(ctx, auth.OptionalLogin, authOpts).Client()
	if err != nil {
		return nil, err
	}
	ua := cipd.UserAgent
	if prefix := cli.LookupEnv(ctx, CIPDHTTPUserAgentPrefix); prefix.Exists {
		ua = fmt.Sprintf("%s/%s", prefix.Value, ua)
	}
	cacheDir := opts.cacheDir
	if cacheDir == "" {
		if cacheDirEnv := cli.LookupEnv(ctx, CIPDCacheDir); cacheDirEnv.Exists {
			cacheDir = cacheDirEnv.Value
			if cacheDir != "" && !filepath.IsAbs(cacheDir) {
				return nil, fmt.Errorf("Bad %s: not an absolute path - %s", CIPDCacheDir, cacheDir)
			}
		}
	}
	return cipd.NewClient(cipd.ClientOptions{
		ServiceURL:          opts.serviceURL,
		Root:                root,
		UserAgent:           ua,
		CacheDir:            cacheDir,
		AuthenticatedClient: client,
		AnonymousClient:     http.DefaultClient,
	})
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
	return strings.Join(chunks, " ")
}

// Set is called by 'flag' package when parsing command line options.
func (vars *packageVars) Set(value string) error {
	// <key>:<value> pair.
	chunks := strings.Split(value, ":")
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
	packageName string
	inputDir    string
	installMode local.InstallMode

	// Deflate compression level (if [1-9]) or 0 to disable compression.
	//
	// Default is 1 (fastest).
	compressionLevel int
}

func (opts *inputOptions) registerFlags(f *flag.FlagSet) {
	opts.vars = packageVars{}

	// Interface to accept package definition file.
	f.StringVar(&opts.packageDef, "pkg-def", "", "*.yaml file that defines what to put into the package.")
	f.Var(&opts.vars, "pkg-var", "Variables accessible from package definition file.")

	// Interface to accept a single directory (alternative to -pkg-def).
	f.StringVar(&opts.packageName, "name", "", "Package name (unused with -pkg-def).")
	f.StringVar(&opts.inputDir, "in", "", "Path to a directory with files to package (unused with -pkg-def).")
	f.Var(&opts.installMode, "install-mode",
		"How the package should be installed: \"copy\" or \"symlink\" (unused with -pkg-def).")

	// Options for the builder.
	f.IntVar(&opts.compressionLevel, "compression-level", 5,
		"Deflate compression level [0-9]: 0 - disable, 1 - best speed, 9 - best compression.")
}

// prepareInput processes inputOptions by collecting all files to be added to
// a package and populating BuildInstanceOptions. Caller is still responsible to
// fill out Output field of BuildInstanceOptions.
func (opts *inputOptions) prepareInput() (local.BuildInstanceOptions, error) {
	empty := local.BuildInstanceOptions{}

	if opts.compressionLevel < 0 || opts.compressionLevel > 9 {
		return empty, makeCLIError("invalid -compression-level: must be in [0-9] set")
	}

	// Handle -name and -in if defined. Do not allow -pkg-def and -pkg-var in that case.
	if opts.inputDir != "" {
		if opts.packageName == "" {
			return empty, makeCLIError("missing required flag: -name")
		}
		if opts.packageDef != "" {
			return empty, makeCLIError("-pkg-def and -in can not be used together")
		}
		if len(opts.vars) != 0 {
			return empty, makeCLIError("-pkg-var and -in can not be used together")
		}

		// Simply enumerate files in the directory.
		var files []local.File
		files, err := local.ScanFileSystem(opts.inputDir, opts.inputDir, nil)
		if err != nil {
			return empty, err
		}
		return local.BuildInstanceOptions{
			Input:            files,
			PackageName:      opts.packageName,
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
			return empty, makeCLIError("-install-mode is ignored if -pkd-def is used")
		}

		// Parse the file, perform variable substitution.
		f, err := os.Open(opts.packageDef)
		if err != nil {
			return empty, err
		}
		defer f.Close()
		pkgDef, err := local.LoadPackageDef(f, opts.vars)
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
		return local.BuildInstanceOptions{
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
	opts.refs = []string{}
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
	opts.tags = []string{}
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
	out := []string{}
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
			return pinInfo{pkg, nil, "", err.Error()}
		}
		return pinInfo{pkg, &pin, "", ""}
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
				if p.Tracking == "" {
					fmt.Printf("  %s\n", p.Pin)
				} else {
					fmt.Printf("  %s (tracking %q)\n", p.Pin, p.Tracking)
				}
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

func hasErrors(pinMap map[string][]pinInfo) bool {
	for _, pins := range pinMap {
		for _, p := range pins {
			if p.Err != "" {
				return true
			}
		}
	}
	return false
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
			c.Opts.clientOptions.registerFlags(&c.Flags, params)
			c.Opts.uploadOptions.registerFlags(&c.Flags)
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
	err = buildInstanceFile(ctx, f.Name(), opts.inputOptions)
	if err != nil {
		return common.Pin{}, err
	}
	return registerInstanceFile(ctx, f.Name(), &registerOpts{
		refsOptions:   opts.refsOptions,
		tagsOptions:   opts.tagsOptions,
		clientOptions: opts.clientOptions,
		uploadOptions: opts.uploadOptions,
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
			c.clientOptions.registerFlags(&c.Flags, params)
			c.Flags.StringVar(&c.rootDir, "root", "<path>", "Path to an installation site root directory.")
			c.Flags.StringVar(&c.ensureFile, "list", "<path>", "(DEPRECATED) A synonym for -ensure-file.")
			c.Flags.StringVar(&c.ensureFile, "ensure-file", "<path>",
				(`An "ensure" file. See syntax described here: ` +
					`https://godoc.org/github.com/luci/luci-go/cipd/client/cipd/ensure.` +
					` Providing '-' will read from stdin.`))
			return c
		},
	}
}

type ensureRun struct {
	cipdSubcommand
	clientOptions

	rootDir    string
	ensureFile string
}

func (c *ensureRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	currentPins, _, err := ensurePackages(ctx, c.rootDir, c.ensureFile, false, c.clientOptions)
	return c.done(currentPins, err)
}

func ensurePackages(ctx context.Context, root string, desiredStateFile string, dryRun bool, clientOpts clientOptions) (common.PinSliceBySubdir, cipd.ActionMap, error) {
	var err error
	var f io.ReadCloser
	if desiredStateFile == "-" {
		f = os.Stdin
	} else {
		if f, err = os.Open(desiredStateFile); err != nil {
			return nil, nil, err
		}
	}
	defer f.Close()

	ensureFile, err := ensure.ParseFile(f)
	if err != nil {
		return nil, nil, err
	}

	// Prefer the ServiceURL from the file (if set), and log a warning if the user
	// provided one on the commandline that doesn't match the one in the file.
	if ensureFile.ServiceURL != "" {
		if clientOpts.serviceURL != "" && clientOpts.serviceURL != ensureFile.ServiceURL {
			logging.Warningf(ctx, "serviceURL in ensure file != serviceURL on CLI (%q v %q). Using %q from file.",
				ensureFile.ServiceURL, clientOpts.serviceURL, ensureFile.ServiceURL)
		}
		clientOpts.serviceURL = ensureFile.ServiceURL
	}

	client, err := clientOpts.makeCipdClient(ctx, root)
	if err != nil {
		return nil, nil, err
	}

	client.BeginBatch(ctx)
	defer client.EndBatch(ctx)

	resolved, err := ensureFile.Resolve(func(pkg, vers string) (common.Pin, error) {
		return client.ResolveVersion(ctx, pkg, vers)
	})
	if err != nil {
		return nil, nil, err
	}

	actions, err := client.EnsurePackages(ctx, resolved.PackagesBySubdir, dryRun)
	if err != nil {
		return nil, actions, err
	}

	return resolved.PackagesBySubdir, actions, nil
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
			c.clientOptions.registerFlags(&c.Flags, params)
			c.Flags.StringVar(&c.rootDir, "root", "<path>", "Path to an installation site root directory.")
			c.Flags.StringVar(&c.ensureFile, "list", "<path>", "(DEPRECATED) A synonym for -ensure-file.")
			c.Flags.StringVar(&c.ensureFile, "ensure-file", "<path>",
				(`An "ensure" file. See syntax described here: ` +
					`https://godoc.org/github.com/luci/luci-go/cipd/client/cipd/ensure`))
			return c
		},
	}
}

type checkUpdatesRun struct {
	cipdSubcommand
	clientOptions

	rootDir    string
	ensureFile string
}

func (c *checkUpdatesRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	_, actions, err := ensurePackages(ctx, c.rootDir, c.ensureFile, true, c.clientOptions)
	if err != nil {
		ret := c.done(actions, err)
		if errors.IsTransient(err) {
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
			c.clientOptions.registerFlags(&c.Flags, params)
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
	client, err := clientOpts.makeCipdClient(ctx, "")
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
			c.clientOptions.registerFlags(&c.Flags, params)
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

func describeInstance(ctx context.Context, pkg, version string, clientOpts clientOptions) (*describeOutput, error) {
	client, err := clientOpts.makeCipdClient(ctx, "")
	if err != nil {
		return nil, err
	}

	// Grab instance ID.
	pin, err := client.ResolveVersion(ctx, pkg, version)
	if err != nil {
		return nil, err
	}

	wg := sync.WaitGroup{}

	// Fetch who and when registered it.
	var info cipd.InstanceInfo
	var infoErr error
	wg.Add(1)
	go func() {
		info, infoErr = client.FetchInstanceInfo(ctx, pin)
		wg.Done()
	}()

	// Fetch the list of refs pointing to the instance.
	var refs []cipd.RefInfo
	var refsErr error
	wg.Add(1)
	go func() {
		refs, refsErr = client.FetchInstanceRefs(ctx, pin, nil)
		wg.Done()
	}()

	// Fetch the list of attached tags.
	var tags []cipd.TagInfo
	var tagsErr error
	wg.Add(1)
	go func() {
		tags, tagsErr = client.FetchInstanceTags(ctx, pin, nil)
		wg.Done()
	}()

	wg.Wait()

	if infoErr != nil {
		return nil, infoErr
	}
	if refsErr != nil {
		return nil, refsErr
	}
	if tagsErr != nil {
		return nil, tagsErr
	}

	fmt.Printf("Package:       %s\n", info.Pin.PackageName)
	fmt.Printf("Instance ID:   %s\n", info.Pin.InstanceID)
	fmt.Printf("Registered by: %s\n", info.RegisteredBy)
	fmt.Printf("Registered at: %s\n", info.RegisteredTs)
	if len(refs) != 0 {
		fmt.Printf("Refs:\n")
		for _, t := range refs {
			fmt.Printf("  %s\n", t.Ref)
		}
	} else {
		fmt.Printf("Refs:          none\n")
	}
	if len(tags) != 0 {
		fmt.Printf("Tags:\n")
		for _, t := range tags {
			fmt.Printf("  %s\n", t.Tag)
		}
	} else {
		fmt.Printf("Tags:          none\n")
	}

	return &describeOutput{info, refs, tags}, nil
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
			c.clientOptions.registerFlags(&c.Flags, params)
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
	ctx := cli.GetContext(a, c, env)
	return c.doneWithPins(setRefOrTag(ctx, &setRefOrTagArgs{
		clientOptions: c.clientOptions,
		packagePrefix: args[0],
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
	client, err := args.clientOptions.makeCipdClient(ctx, "")
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
	if pm := map[string][]pinInfo{"": pins}; hasErrors(pm) {
		printPinsAndError(pm)
		return nil, fmt.Errorf("can't find %q version in all packages, aborting", args.version)
	}

	// Prepare for the next batch call.
	packages := []string{}
	pinsToUse := map[string]common.Pin{}
	for _, p := range pins {
		packages = append(packages, p.Pkg)
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
			c.clientOptions.registerFlags(&c.Flags, params)
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
	ctx := cli.GetContext(a, c, env)
	return c.done(setRefOrTag(ctx, &setRefOrTagArgs{
		clientOptions: c.clientOptions,
		packagePrefix: args[0],
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
			c.clientOptions.registerFlags(&c.Flags, params)
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
	path := ""
	if len(args) == 1 {
		path = args[0]
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(listPackages(ctx, path, c.recursive, c.showHidden, c.clientOptions))
}

func listPackages(ctx context.Context, path string, recursive, showHidden bool, clientOpts clientOptions) ([]string, error) {
	client, err := clientOpts.makeCipdClient(ctx, "")
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
		UsageLine: "search [package] -tag key:value [options]",
		ShortDesc: "searches for package instances by tag",
		LongDesc:  "Searches for package instances by tag, optionally constrained by package name.",
		CommandRun: func() subcommands.CommandRun {
			c := &searchRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params)
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
	if !c.checkArgs(args, 0, 1) {
		return 1
	}
	if len(c.tags) != 1 {
		return c.done(nil, makeCLIError("exactly one -tag must be provided"))
	}
	packageName := ""
	if len(args) == 1 {
		packageName = args[0]
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(searchInstances(ctx, packageName, c.tags[0], c.clientOptions))
}

func searchInstances(ctx context.Context, packageName, tag string, clientOpts clientOptions) ([]common.Pin, error) {
	client, err := clientOpts.makeCipdClient(ctx, "")
	if err != nil {
		return nil, err
	}
	pins, err := client.SearchInstances(ctx, tag, packageName)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		fmt.Println("No matching packages.")
	} else {
		fmt.Println("Packages:")
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
			c.clientOptions.registerFlags(&c.Flags, params)
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
	ctx := cli.GetContext(a, c, env)
	return c.done(listACL(ctx, args[0], c.clientOptions))
}

func listACL(ctx context.Context, packagePath string, clientOpts clientOptions) (map[string][]cipd.PackageACL, error) {
	client, err := clientOpts.makeCipdClient(ctx, "")
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
	listRoleACL("Counter Writers", byRole["COUNTER_WRITER"])

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
			c.clientOptions.registerFlags(&c.Flags, params)
			c.Flags.Var(&c.owner, "owner", "Users or groups to grant OWNER role.")
			c.Flags.Var(&c.writer, "writer", "Users or groups to grant WRITER role.")
			c.Flags.Var(&c.reader, "reader", "Users or groups to grant READER role.")
			c.Flags.Var(&c.counterWriter, "counter-writer", "Users or groups to grant COUNTER_WRITER role.")
			c.Flags.Var(&c.revoke, "revoke", "Users or groups to remove from all roles.")
			return c
		},
	}
}

type editACLRun struct {
	cipdSubcommand
	clientOptions

	owner         principalsList
	writer        principalsList
	reader        principalsList
	counterWriter principalsList
	revoke        principalsList
}

func (c *editACLRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(nil, editACL(ctx, args[0], c.owner, c.writer, c.reader, c.counterWriter, c.revoke, c.clientOptions))
}

func editACL(ctx context.Context, packagePath string, owners, writers, readers, counterWriters, revoke principalsList, clientOpts clientOptions) error {
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
	makeChanges(cipd.GrantRole, "COUNTER_WRITER", counterWriters)

	makeChanges(cipd.RevokeRole, "OWNER", revoke)
	makeChanges(cipd.RevokeRole, "WRITER", revoke)
	makeChanges(cipd.RevokeRole, "READER", revoke)
	makeChanges(cipd.RevokeRole, "COUNTER_WRITER", revoke)

	if len(changes) == 0 {
		return nil
	}

	client, err := clientOpts.makeCipdClient(ctx, "")
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
			c.Flags.StringVar(&c.outputFile, "out", "<path>", "Path to a file to write the final package to.")
			return c
		},
	}
}

type buildRun struct {
	cipdSubcommand
	inputOptions

	outputFile string
}

func (c *buildRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	err := buildInstanceFile(ctx, c.outputFile, c.inputOptions)
	if err != nil {
		return c.done(nil, err)
	}
	return c.done(inspectInstanceFile(ctx, c.outputFile, false))
}

func buildInstanceFile(ctx context.Context, instanceFile string, inputOpts inputOptions) error {
	// Read the list of files to add to the package.
	buildOpts, err := inputOpts.prepareInput()
	if err != nil {
		return err
	}

	// Prepare the destination, update build options with io.Writer to it.
	out, err := os.OpenFile(instanceFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	buildOpts.Output = out

	// Build the package.
	err = local.BuildInstance(ctx, buildOpts)
	out.Close()
	if err != nil {
		os.Remove(instanceFile)
		return err
	}
	return nil
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
			c.Flags.StringVar(&c.rootDir, "root", "<path>", "Path to an installation site root directory.")
			return c
		},
	}
}

type deployRun struct {
	cipdSubcommand

	rootDir string
}

func (c *deployRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(deployInstanceFile(ctx, c.rootDir, args[0]))
}

func deployInstanceFile(ctx context.Context, root string, instanceFile string) (common.Pin, error) {
	inst, err := local.OpenInstanceFile(ctx, instanceFile, "")
	if err != nil {
		return common.Pin{}, err
	}
	defer inst.Close()
	inspectInstance(ctx, inst, false)

	d := local.NewDeployer(root)
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
			c.clientOptions.registerFlags(&c.Flags, params)
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
	ctx := cli.GetContext(a, c, env)
	return c.done(fetchInstanceFile(ctx, args[0], c.version, c.outputPath, c.clientOptions))
}

func fetchInstanceFile(ctx context.Context, packageName, version, instanceFile string, clientOpts clientOptions) (common.Pin, error) {
	client, err := clientOpts.makeCipdClient(ctx, "")
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

	// Verify it (by checking that instanceID matches the file content).
	out.Close()
	ok = true
	inst, err := local.OpenInstanceFile(ctx, instanceFile, pin.InstanceID)
	if err != nil {
		os.Remove(instanceFile)
		return common.Pin{}, err
	}
	defer inst.Close()
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
			return c
		},
	}
}

type inspectRun struct {
	cipdSubcommand
}

func (c *inspectRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(inspectInstanceFile(ctx, args[0], true))
}

func inspectInstanceFile(ctx context.Context, instanceFile string, listFiles bool) (common.Pin, error) {
	inst, err := local.OpenInstanceFile(ctx, instanceFile, "")
	if err != nil {
		return common.Pin{}, err
	}
	defer inst.Close()
	inspectInstance(ctx, inst, listFiles)
	return inst.Pin(), nil
}

func inspectInstance(ctx context.Context, inst local.PackageInstance, listFiles bool) {
	fmt.Printf("Instance: %s\n", inst.Pin())
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
				fmt.Printf(" F %s\n", f.Name())
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
			c.Opts.clientOptions.registerFlags(&c.Flags, params)
			c.Opts.uploadOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type registerOpts struct {
	refsOptions
	tagsOptions
	clientOptions
	uploadOptions
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
	return c.done(registerInstanceFile(ctx, args[0], &c.Opts))
}

func registerInstanceFile(ctx context.Context, instanceFile string, opts *registerOpts) (common.Pin, error) {
	inst, err := local.OpenInstanceFile(ctx, instanceFile, "")
	if err != nil {
		return common.Pin{}, err
	}
	defer inst.Close()
	client, err := opts.clientOptions.makeCipdClient(ctx, "")
	if err != nil {
		return common.Pin{}, err
	}
	inspectInstance(ctx, inst, false)
	err = client.RegisterInstance(ctx, inst, opts.uploadOptions.verificationTimeout)
	if err != nil {
		return common.Pin{}, err
	}
	err = client.AttachTagsWhenReady(ctx, inst.Pin(), opts.tagsOptions.tags)
	if err != nil {
		return common.Pin{}, err
	}
	for _, ref := range opts.refsOptions.refs {
		err = client.SetRefWhenReady(ctx, ref, inst.Pin())
		if err != nil {
			return common.Pin{}, err
		}
	}
	return inst.Pin(), nil
}

////////////////////////////////////////////////////////////////////////////////
// 'pkg-delete' subcommand.

func cmdDelete(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-delete <package name>",
		ShortDesc: "removes the package from the package repository on the backend",
		LongDesc: "Removes all instances of the package, all its tags and refs.\n" +
			"There's no confirmation and no undo. Be careful.",
		CommandRun: func() subcommands.CommandRun {
			c := &deleteRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params)
			return c
		},
	}
}

type deleteRun struct {
	cipdSubcommand
	clientOptions
}

func (c *deleteRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(nil, deletePackage(ctx, args[0], &c.clientOptions))
}

func deletePackage(ctx context.Context, packageName string, opts *clientOptions) error {
	client, err := opts.makeCipdClient(ctx, "")
	if err != nil {
		return err
	}
	switch err = client.DeletePackage(ctx, packageName); {
	case err == nil:
		return nil
	case err == cipd.ErrPackageNotFound:
		fmt.Printf("Package %q doesn't exist. Already deleted?\n", packageName)
		return nil // not a failure, to make "cipd pkg-delete ..." idempotent
	default:
		return err
	}
}

////////////////////////////////////////////////////////////////////////////////
// 'counter-write' subcommand.

func cmdCounterWrite(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "counter-write -version <version> <package name> [-increment <counter> | -touch <counter>]",
		ShortDesc: "updates a named counter associated with the given package version",
		LongDesc: "Updates a named counter associated with the given package version\n" +
			"If used with -increment the counter will be incremented by 1 and its timestamp\n" +
			"updated.  The counter will be created with an initial value of 1 if it does not\n" +
			"exist.\n" +
			"If used with -touch the timestamp will be updated without changing the value.\n" +
			"The counter will be created with an initial value of 0 if it does not exist.",
		CommandRun: func() subcommands.CommandRun {
			c := &counterWriteRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params)
			c.Flags.StringVar(&c.version, "version", "<version>", "Version of the package to modify.")
			c.Flags.StringVar(&c.increment, "increment", "", "Name of the counter to increment.")
			c.Flags.StringVar(&c.touch, "touch", "", "Name of the counter to touch.")
			return c
		},
	}
}

type counterWriteRun struct {
	cipdSubcommand
	clientOptions

	version   string
	increment string
	touch     string
}

func (c *counterWriteRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}

	var counter string
	var delta int

	switch {
	case c.increment == "" && c.touch == "":
		return c.done(nil, makeCLIError("one of -increment or -touch must be used"))
	case c.increment != "" && c.touch != "":
		return c.done(nil, makeCLIError("-increment and -touch can not be used together"))
	case c.increment != "":
		delta = 1
		counter = c.increment
	case c.touch != "":
		delta = 0
		counter = c.touch
	}

	ctx := cli.GetContext(a, c, env)
	return c.done(nil, writeCounter(ctx, args[0], c.version, counter, delta, &c.clientOptions))
}

func writeCounter(ctx context.Context, pkg, version, counter string, delta int, opts *clientOptions) error {
	client, err := opts.makeCipdClient(ctx, "")
	if err != nil {
		return err
	}

	// Grab instance ID.
	pin, err := client.ResolveVersion(ctx, pkg, version)
	if err != nil {
		return err
	}

	return client.IncrementCounter(ctx, pin, counter, delta)
}

////////////////////////////////////////////////////////////////////////////////
// 'counter-read' subcommand.

func cmdCounterRead(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "counter-read -version <version> <package name> <counter> [<counter> ...]",
		ShortDesc: "fetches one or more counters for the given package version",
		LongDesc:  "Fetches one or more counters for the given package version",
		CommandRun: func() subcommands.CommandRun {
			c := &counterReadRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params)
			c.Flags.StringVar(&c.version, "version", "<version>", "Version of the package to modify.")
			return c
		},
	}
}

type counterReadRun struct {
	cipdSubcommand
	clientOptions

	version string
}

func (c *counterReadRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 2, -1) {
		return 1
	}

	ctx := cli.GetContext(a, c, env)
	ret, err := readCounters(ctx, args[0], c.version, args[1:], &c.clientOptions)
	return c.done(ret, err)
}

type counterReadResult struct {
	cipd.Counter
	Error error `json:"error,omitempty"`
}

func readCounters(ctx context.Context, pkg, version string, counters []string, opts *clientOptions) ([]counterReadResult, error) {
	client, err := opts.makeCipdClient(ctx, "")
	if err != nil {
		return nil, err
	}

	// Grab instance ID.
	pin, err := client.ResolveVersion(ctx, pkg, version)
	if err != nil {
		return nil, err
	}

	// Read the counters in parallel.
	results := make(chan cipd.Counter)
	errs := make(chan error)
	defer close(results)
	defer close(errs)

	for _, counter := range counters {
		go func(counter string) {
			result, err := client.ReadCounter(ctx, pin, counter)
			if err == nil {
				results <- result
			} else {
				errs <- err
			}
		}(counter)
	}

	fmt.Printf("Package:       %s\n", pin.PackageName)
	fmt.Printf("Instance ID:   %s\n", pin.InstanceID)

	remaining := len(counters)
	var ret []counterReadResult
	var lastErr error
	for remaining > 0 {
		select {
		case result := <-results:
			ret = append(ret, counterReadResult{Counter: result})
			fmt.Printf("\n")
			fmt.Printf("Counter:       %s\n", result.Name)
			fmt.Printf("Value:         %d\n", result.Value)
			if !result.CreatedTS.IsZero() {
				fmt.Printf("Created at:    %s\n", result.CreatedTS)
			}
			if !result.UpdatedTS.IsZero() {
				fmt.Printf("Last updated:  %s\n", result.UpdatedTS)
			}
			remaining--
		case err := <-errs:
			ret = append(ret, counterReadResult{Error: err})
			lastErr = err
			fmt.Printf("\n")
			fmt.Printf("Failed to read counter: %s\n", err)
			remaining--
		}
	}

	return ret, lastErr
}

////////////////////////////////////////////////////////////////////////////////
// 'selfupdate' subcommand.

func cmdSelfUpdate(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "selfupdate -version <version>",
		ShortDesc: "updates the current cipd client binary",
		LongDesc:  "does an in-place upgrade to the current cipd binary",
		CommandRun: func() subcommands.CommandRun {
			s := &selfupdateRun{}
			s.registerBaseFlags()
			s.clientOptions.registerFlags(&s.Flags, params)
			s.Flags.StringVar(&s.version, "version", "", "Version of the client to update to.")
			return s
		},
	}
}

type selfupdateRun struct {
	cipdSubcommand
	clientOptions

	version string
}

func (s *selfupdateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !s.checkArgs(args, 0, 0) {
		return 1
	}
	if s.version == "" {
		s.printError(makeCLIError("-version is required"))
		return 1
	}
	ctx := cli.GetContext(a, s, env)
	exePath, err := osext.Executable()
	if err != nil {
		s.printError(err)
		return 1
	}
	clientCacheDir := filepath.Join(filepath.Dir(exePath), ".cipd_client_cache")
	s.clientOptions.cacheDir = clientCacheDir
	fs := local.NewFileSystem(filepath.Dir(exePath), filepath.Join(clientCacheDir, "trash"))
	defer fs.CleanupTrash(ctx)
	return s.doSelfUpdate(ctx, exePath, fs)
}

func executableSHA1(exePath string) (string, error) {
	file, err := os.Open(exePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha1.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *selfupdateRun) doSelfUpdate(ctx context.Context, exePath string, fs local.FileSystem) int {
	if err := common.ValidateInstanceVersion(s.version); err != nil {
		s.printError(err)
		return 1
	}

	curExeHash, err := executableSHA1(exePath)
	if err != nil {
		s.printError(err)
		return 1
	}

	client, err := s.clientOptions.makeCipdClient(ctx, filepath.Dir(exePath))
	if err != nil {
		s.printError(err)
		return 1
	}
	if err := client.MaybeUpdateClient(ctx, fs, s.version, curExeHash, exePath); err != nil {
		s.printError(err)
		return 1
	}

	return 0
}

////////////////////////////////////////////////////////////////////////////////
// Flag hacks, see 'Main' doc.

func splitCmdLine(args []string) (cmd string, flags []string, pos []string) {
	// No subcomand, just flags.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", args, nil
	}
	// Pick subcommand, than collect all positional args up to a first flag.
	cmd = args[0]
	firstFlagIdx := -1
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			firstFlagIdx = i
			break
		}
	}
	// No flags at all.
	if firstFlagIdx == -1 {
		return cmd, nil, args[1:]
	}
	return cmd, args[firstFlagIdx:], args[1:firstFlagIdx]
}

func fixFlagsPosition(args []string) []string {
	// 'flags' package requires positional arguments to be after flags. This is
	// very inconvenient choice, it makes commands like "set-ref" look awkward:
	// Compare "set-ref -ref=abc -version=def package/name" to more natural
	// "set-ref package/name -ref=abc -version=def". Reshuffle arguments to put
	// all positional args at the end of the command line.
	cmd, flags, positional := splitCmdLine(args)
	newArgs := []string{}
	if cmd != "" {
		newArgs = append(newArgs, cmd)
	}
	newArgs = append(newArgs, flags...)
	newArgs = append(newArgs, positional...)
	return newArgs
}

////////////////////////////////////////////////////////////////////////////////
// Main.

// Environment variable definitions
const (
	CIPDHTTPUserAgentPrefix = "CIPD_HTTP_USER_AGENT_PREFIX"
	CIPDCacheDir            = "CIPD_CACHE_DIR"
)

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
			CIPDHTTPUserAgentPrefix: {
				Advanced:  true,
				ShortDesc: "Optional http User-Agent prefix.",
			},
			CIPDCacheDir: {
				ShortDesc: "Directory with shared instance and tags cache " +
					"(-cache-dir, if given, takes precedence).",
			},
		},

		Commands: []*subcommands.Command{
			subcommands.CmdHelp,
			version.SubcommandVersion,

			// User friendly subcommands that operates within a site root. Implemented
			// in friendly.go.
			cmdInit(params),
			cmdInstall(params),
			cmdInstalled(params),

			// Authentication related commands.
			authcli.SubcommandInfo(params.DefaultAuthOptions, "auth-info", true),
			authcli.SubcommandLogin(params.DefaultAuthOptions, "auth-login", false),
			authcli.SubcommandLogout(params.DefaultAuthOptions, "auth-logout", false),

			// High level commands.
			cmdListPackages(params),
			cmdSearch(params),
			cmdCreate(params),
			cmdEnsure(params),
			cmdResolve(params),
			cmdDescribe(params),
			cmdSetRef(params),
			cmdSetTag(params),
			cmdSelfUpdate(params),

			// ACLs.
			cmdListACL(params),
			cmdEditACL(params),

			// Counters.
			cmdCounterWrite(params),
			cmdCounterRead(params),

			// Low level pkg-* commands.
			cmdBuild(),
			cmdDeploy(),
			cmdFetch(params),
			cmdInspect(),
			cmdRegister(params),
			cmdDelete(params),

			// Low level misc commands.
			cmdPuppetCheckUpdates(params),
		},
	}
}

// Main runs the CIPD CLI.
//
// It's like subcommands.Run(GetApplication(...), args) except it allows
// flag arguments and positional arguments to be mixed, which makes some CIPD
// subcommand invocations look more natural.
//
// Compare:
//   * Default: cipd set-ref -ref=abc -version=def package/name
//   * Improved: cipd set-ref package/name -ref=abc -version=def
//
// Much better.
func Main(params Parameters, args []string) int {
	return subcommands.Run(GetApplication(params), fixFlagsPosition(args))
}
