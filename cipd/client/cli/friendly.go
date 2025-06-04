// Copyright 2015 The LUCI Authors.
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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/deployer"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// Site root path resolution.

// findSiteRoot returns a directory R such as R/.cipd exists and p is inside
// R or p is R. Returns empty string if no such directory.
func findSiteRoot(p string) string {
	for {
		if isSiteRoot(p) {
			return p
		}
		// Dir returns "/" (or C:\\) when it encounters the root directory. This is
		// the only case when the return value of Dir(...) ends with separator.
		parent := filepath.Dir(p)
		if parent[len(parent)-1] == filepath.Separator {
			// It is possible disk root has .cipd directory, check it.
			if isSiteRoot(parent) {
				return parent
			}
			return ""
		}
		p = parent
	}
}

// optionalSiteRoot takes a path to a site root or an empty string. If some
// path is given, it normalizes it and ensures that it is indeed a site root
// directory. If empty string is given, it discovers a site root for current
// directory.
func optionalSiteRoot(siteRoot string) (string, error) {
	if siteRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", cipderr.IO.Apply(errors.Fmt("resolving current working directory: %w", err))
		}
		siteRoot = findSiteRoot(cwd)
		if siteRoot == "" {
			return "", cipderr.BadArgument.Apply(errors.Fmt("directory %s is not in a site root, use 'init' to create one", cwd))
		}
		return siteRoot, nil
	}
	siteRoot, err := filepath.Abs(siteRoot)
	if err != nil {
		return "", cipderr.BadArgument.Apply(errors.Fmt("bad site root path: %w", err))
	}
	if !isSiteRoot(siteRoot) {
		return "", cipderr.BadArgument.Apply(errors.Fmt("directory %s doesn't look like a site root, use 'init' to create one", siteRoot))
	}
	return siteRoot, nil
}

// isSiteRoot returns true if <p>/.cipd exists.
func isSiteRoot(p string) bool {
	fi, err := os.Stat(filepath.Join(p, fs.SiteServiceDir))
	return err == nil && fi.IsDir()
}

////////////////////////////////////////////////////////////////////////////////
// Config file parsing.

// installationSiteConfig is stored in .cipd/config.json.
type installationSiteConfig struct {
	// ServiceURL is https://<hostname> of a backend to use by default.
	ServiceURL string `json:",omitempty"`
	// DefaultVersion is what version to install if not specified.
	DefaultVersion string `json:",omitempty"`
	// TrackedVersions is mapping package name -> version to use in 'update'.
	TrackedVersions map[string]string `json:",omitempty"`
	// CacheDir contains shared cache.
	CacheDir string `json:",omitempty"`
}

// read loads JSON from given path.
func (c *installationSiteConfig) read(path string) error {
	*c = installationSiteConfig{}
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()
	return json.NewDecoder(r).Decode(c)
}

// write dumps JSON to given path.
func (c *installationSiteConfig) write(path string) error {
	blob, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, blob, 0666)
}

// readConfig reads config, returning default one if missing.
//
// The returned config may have ServiceURL set to "" due to previous buggy
// version of CIPD not setting it up correctly.
func readConfig(siteRoot string) (installationSiteConfig, error) {
	path := filepath.Join(siteRoot, fs.SiteServiceDir, "config.json")
	c := installationSiteConfig{}
	if err := c.read(path); err != nil && !os.IsNotExist(err) {
		return c, cipderr.IO.Apply(errors.Fmt("failed to read site root config: %w", err))
	}
	return c, nil
}

////////////////////////////////////////////////////////////////////////////////
// High level wrapper around site root.

// installationSite represents a site root directory with config and optional
// cipd.Client instance configured to install packages into that root.
type installationSite struct {
	siteRoot          string                  // path to a site root directory
	defaultServiceURL string                  // set during construction
	cfg               *installationSiteConfig // parsed .cipd/config.json file
	client            cipd.Client             // initialized by initClient()
}

// getInstallationSite finds site root directory, reads config and constructs
// installationSite object.
//
// If siteRoot is "", will find a site root based on the current directory,
// otherwise will use siteRoot. Doesn't create any new files or directories,
// just reads what's on disk.
func getInstallationSite(siteRoot, defaultServiceURL string) (*installationSite, error) {
	siteRoot, err := optionalSiteRoot(siteRoot)
	if err != nil {
		return nil, err
	}
	cfg, err := readConfig(siteRoot)
	if err != nil {
		return nil, err
	}
	if cfg.ServiceURL == "" {
		cfg.ServiceURL = defaultServiceURL
	}
	return &installationSite{siteRoot, defaultServiceURL, &cfg, nil}, nil
}

// initInstallationSite creates new site root directory on disk.
//
// It does a bunch of sanity checks (like whether rootDir is empty) that are
// skipped if 'force' is set to true.
func initInstallationSite(rootDir, defaultServiceURL string, force bool) (*installationSite, error) {
	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, cipderr.BadArgument.Apply(errors.
			Fmt("bad root path: %w", err))
	}

	// rootDir is inside an existing site root?
	existing := findSiteRoot(rootDir)
	if existing != "" {
		msg := fmt.Sprintf("directory %s is already inside a site root (%s)", rootDir, existing)
		if !force {
			return nil, cipderr.BadArgument.Apply(errors.New(msg))
		}
		fmt.Fprintf(os.Stderr, "Warning: %s.\n", msg)
	}

	// Attempting to use in a non empty directory?
	entries, err := os.ReadDir(rootDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, cipderr.IO.Apply(errors.Fmt("bad site root dir: %w", err))
	}
	if len(entries) != 0 {
		msg := fmt.Sprintf("directory %s is not empty", rootDir)
		if !force {
			return nil, cipderr.BadArgument.Apply(errors.New(msg))
		}
		fmt.Fprintf(os.Stderr, "Warning: %s.\n", msg)
	}

	// Good to go.
	if err = os.MkdirAll(filepath.Join(rootDir, fs.SiteServiceDir), 0777); err != nil {
		return nil, cipderr.IO.Apply(errors.Fmt("creating site root dir: %w", err))
	}
	site, err := getInstallationSite(rootDir, defaultServiceURL)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Site root initialized at %s.\n", rootDir)
	return site, nil
}

// initClient initializes cipd.Client to use to talk to backend.
//
// Can be called only once. Use it directly via site.client.
func (site *installationSite) initClient(ctx context.Context, authFlags authcli.Flags) (err error) {
	if site.client != nil {
		return cipderr.BadArgument.Apply(errors.New("client is already initialized"))
	}
	clientOpts := clientOptions{
		authFlags:  authFlags,
		serviceURL: site.cfg.ServiceURL,
		cacheDir:   site.cfg.CacheDir,
		rootDir:    site.siteRoot,
	}
	site.client, err = clientOpts.makeCIPDClient(ctx)
	return err
}

// modifyConfig reads config file, calls callback to mutate it, then writes
// it back.
func (site *installationSite) modifyConfig(cb func(cfg *installationSiteConfig) error) error {
	path := filepath.Join(site.siteRoot, fs.SiteServiceDir, "config.json")
	c := installationSiteConfig{}
	if err := c.read(path); err != nil && !os.IsNotExist(err) {
		return cipderr.IO.Apply(errors.Fmt("reading site root config: %w", err))
	}
	if err := cb(&c); err != nil {
		return err
	}
	// Fix broken config that doesn't have ServiceURL set. It is required now.
	if c.ServiceURL == "" {
		c.ServiceURL = site.defaultServiceURL
	}
	if err := c.write(path); err != nil {
		return cipderr.IO.Apply(errors.Fmt("writing site root config: %w", err))
	}
	return nil
}

// installedPackages discovers versions of packages installed in the site.
//
// If pkgs is empty array, it returns list of all installed packages.
func (site *installationSite) installedPackages(ctx context.Context) (map[string][]pinInfo, error) {
	d := deployer.New(site.siteRoot)

	allPins, err := d.FindDeployed(ctx)
	if err != nil {
		return nil, err
	}
	output := make(map[string][]pinInfo, len(allPins))
	for subdir, pins := range allPins {
		output[subdir] = make([]pinInfo, len(pins))
		for i, pin := range pins {
			cpy := pin
			output[subdir][i] = pinInfo{
				Pkg:      pin.PackageName,
				Pin:      &cpy,
				Tracking: site.cfg.TrackedVersions[pin.PackageName],
			}
		}
	}
	return output, nil
}

// installPackage installs (or updates) a package.
func (site *installationSite) installPackage(ctx context.Context, pkgName, version string, paranoid cipd.ParanoidMode) (*pinInfo, error) {
	if site.client == nil {
		return nil, cipderr.BadArgument.Apply(errors.
			New("client is not initialized"))
	}

	// Figure out what exactly (what instance ID) to install.
	if version == "" {
		version = site.cfg.DefaultVersion
	}
	if version == "" {
		version = "latest"
	}
	resolved, err := site.client.ResolveVersion(ctx, pkgName, version)
	if err != nil {
		return nil, err
	}

	// Install it by constructing an ensure file with all already installed
	// packages plus the one we are installing (into the root "" subdir).
	deployed, err := site.client.FindDeployed(ctx)
	if err != nil {
		return nil, err
	}

	found := false
	root := deployed[""]
	for idx := range root {
		if root[idx].PackageName == resolved.PackageName {
			root[idx] = resolved // upgrading the existing package
			found = true
		}
	}
	if !found {
		if deployed == nil {
			deployed = common.PinSliceBySubdir{}
		}
		deployed[""] = append(deployed[""], resolved) // install a new one
	}

	actions, err := site.client.EnsurePackages(ctx, deployed, &cipd.EnsureOptions{
		Paranoia: paranoid,
	})
	if err != nil {
		return nil, err
	}

	if actions.Empty() {
		fmt.Printf("Package %s is up-to-date.\n", pkgName)
	}

	// Update config saying what version to track. Remove tracking if an exact
	// instance ID was requested.
	trackedVersion := ""
	if version != resolved.InstanceID {
		trackedVersion = version
	}
	err = site.modifyConfig(func(cfg *installationSiteConfig) error {
		if cfg.TrackedVersions == nil {
			cfg.TrackedVersions = map[string]string{}
		}
		if cfg.TrackedVersions[pkgName] != trackedVersion {
			if trackedVersion == "" {
				fmt.Printf("Package %s is now pinned to %q.\n", pkgName, resolved.InstanceID)
			} else {
				fmt.Printf("Package %s is now tracking %q.\n", pkgName, trackedVersion)
			}
		}
		if trackedVersion == "" {
			delete(cfg.TrackedVersions, pkgName)
		} else {
			cfg.TrackedVersions[pkgName] = trackedVersion
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Success.
	return &pinInfo{
		Pkg:      pkgName,
		Pin:      &resolved,
		Tracking: trackedVersion,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Common command line flags.

// siteRootOptions defines command line flag for specifying existing site root
// directory. 'init' subcommand is NOT using it, since it creates a new site
// root, not reusing an existing one.
type siteRootOptions struct {
	rootDir string
}

func (opts *siteRootOptions) registerFlags(f *flag.FlagSet) {
	f.StringVar(
		&opts.rootDir, "root", "", "Path to an installation site root directory. "+
			"If omitted will try to discover it by examining parent directories.")
}

////////////////////////////////////////////////////////////////////////////////
// 'init' subcommand.

func cmdInit(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "init [root dir] [options]",
		ShortDesc: "sets up a new site root directory to install packages into",
		LongDesc: "Sets up a new site root directory to install packages into.\n\n" +
			"Uses current working directory by default.\n" +
			"Unless -force is given, the new site root directory should be empty (or " +
			"do not exist at all) and not be under some other existing site root. " +
			"The command will create <root>/.cipd subdirectory with some " +
			"configuration files. This directory is used by CIPD client to keep " +
			"track of what is installed in the site root.",
		CommandRun: func() subcommands.CommandRun {
			c := &initRun{}
			c.registerBaseFlags()
			c.Flags.BoolVar(&c.force, "force", false, "Create the site root even if the directory is not empty or already under another site root directory.")
			c.Flags.StringVar(&c.serviceURL, "service-url", params.ServiceURL, "Backend URL. Will be put into the site config and used for subsequent 'install' commands.")
			c.Flags.StringVar(&c.cacheDir, "cache-dir", "", "Directory for shared cache")
			return c
		},
	}
}

type initRun struct {
	cipdSubcommand

	force      bool
	serviceURL string
	cacheDir   string
}

func (c *initRun) Run(a subcommands.Application, args []string, _ subcommands.Env) int {
	if !c.checkArgs(args, 0, 1) {
		return 1
	}
	rootDir := "."
	if len(args) == 1 {
		rootDir = args[0]
	}
	site, err := initInstallationSite(rootDir, c.serviceURL, c.force)
	if err != nil {
		return c.done(nil, err)
	}
	err = site.modifyConfig(func(cfg *installationSiteConfig) error {
		cfg.ServiceURL = c.serviceURL
		cfg.CacheDir = c.cacheDir
		return nil
	})
	return c.done(site.siteRoot, err)
}

////////////////////////////////////////////////////////////////////////////////
// 'install' subcommand.

func cmdInstall(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "install <package> [<version>] [options]",
		ShortDesc: "installs or updates a package",
		LongDesc:  "Installs or updates a package.",
		CommandRun: func() subcommands.CommandRun {
			c := &installRun{defaultServiceURL: params.ServiceURL}
			c.registerBaseFlags()
			c.authFlags.Register(&c.Flags, params.DefaultAuthOptions)
			c.siteRootOptions.registerFlags(&c.Flags)
			c.Flags.BoolVar(&c.force, "force", false, "Check all package files and present and reinstall them if missing.")
			return c
		},
	}
}

type installRun struct {
	cipdSubcommand
	authFlags authcli.Flags
	siteRootOptions

	defaultServiceURL string // used only if the site config has ServiceURL == ""
	force             bool   // if true use CheckPresence paranoid mode
}

func (c *installRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 2) {
		return 1
	}

	// Pkg and version to install.
	pkgName, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	version := ""
	if len(args) == 2 {
		version = args[1]
	}

	paranoid := cipd.NotParanoid
	if c.force {
		paranoid = cipd.CheckPresence
	}

	// Auto initialize site root directory if necessary. Don't be too aggressive
	// about it though (do not use force=true). Will do anything only if
	// c.rootDir points to an empty directory.
	var site *installationSite
	rootDir, err := optionalSiteRoot(c.rootDir)
	if err == nil {
		site, err = getInstallationSite(rootDir, c.defaultServiceURL)
	} else {
		site, err = initInstallationSite(c.rootDir, c.defaultServiceURL, false)
		if err != nil {
			err = errors.Fmt("can't auto initialize cipd site root, use 'init': %w", err)
		}
	}
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	if err = site.initClient(ctx, c.authFlags); err != nil {
		return c.done(nil, err)
	}
	defer site.client.Close(ctx)
	site.client.BeginBatch(ctx)
	defer site.client.EndBatch(ctx)
	return c.done(site.installPackage(ctx, pkgName, version, paranoid))
}

////////////////////////////////////////////////////////////////////////////////
// 'installed' subcommand.

func cmdInstalled(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "installed [options]",
		ShortDesc: "lists packages installed in the site root",
		LongDesc:  "Lists packages installed in the site root.",
		CommandRun: func() subcommands.CommandRun {
			c := &installedRun{defaultServiceURL: params.ServiceURL}
			c.registerBaseFlags()
			c.siteRootOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type installedRun struct {
	cipdSubcommand
	siteRootOptions

	defaultServiceURL string // used only if the site config has ServiceURL == ""
}

func (c *installedRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	site, err := getInstallationSite(c.rootDir, c.defaultServiceURL)
	if err != nil {
		return c.done(nil, err)
	}
	ctx := cli.GetContext(a, c, env)
	return c.doneWithPinMap(site.installedPackages(ctx))
}
