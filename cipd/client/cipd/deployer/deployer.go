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

package deployer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/internal/retry"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// TODO(vadimsh): How to handle path conflicts between two packages? Currently
// the last one installed wins.

// File system layout of a site directory <base> for "symlink" install method:
// <base>/.cipd/pkgs/
//   <arbitrary index>/
//     description.json
//     _current -> symlink to fea3ab83440e9dfb813785e16d4101f331ed44f4
//     fea3ab83440e9dfb813785e16d4101f331ed44f4/
//       bin/
//         tool
//         ...
//       ...
// bin/
//    tool -> symlink to ../.cipd/pkgs/<package name digest>/_current/bin/tool
//    ...
//
// Where <arbitrary index> is chosen to be the smallest number available for
// this installation (installing more packages gets higher numbers, removing
// packages and installing new ones will reuse the smallest ones).
//
// Some efforts are made to make sure that during the deployment a window of
// inconsistency in the file system is as small as possible.
//
// For "copy" install method everything is much simpler: files are directly
// copied to the site root directory and .cipd/pkgs/* contains only metadata,
// such as description and manifest files with a list of extracted files (to
// know what to uninstall).

// DeployedPackage represents a state of the deployed (or partially deployed)
// package, as returned by CheckDeployed.
//
// ToRedeploy is populated only when CheckDeployed is called in a paranoid mode,
// and the package needs repairs.
type DeployedPackage struct {
	Deployed          bool            // true if the package is deployed (perhaps partially)
	Pin               common.Pin      // the currently installed pin
	Subdir            string          // the site subdirectory where the package is installed
	Manifest          *pkg.Manifest   // instance's manifest, if available
	InstallMode       pkg.InstallMode // validated non-empty install mode requested by instance, if available
	ActualInstallMode pkg.InstallMode // validated non-empty install mode of actual deployment, if available

	// ToRedeploy is a list of files that needs to be reextracted from the
	// original package and relinked into the site root.
	ToRedeploy []string

	// ToRelink is a list of files that needs to be relinked into the site root.
	//
	// They are already present in the .cipd/* guts, so there's no need to fetch
	// the original package to get them.
	ToRelink []string

	// packagePath is a native path to the package directory in the CIPD guts, as
	// returned by deployed.packagePath(...) e.g. .cipd/pkgs/<index>.
	//
	// Set only if it can be identified. May be set even if Deployed is false, in
	// case the package was partially deployed (e.g. the gut directory already
	// exists, but the instance hasn't been extracted there yet).
	packagePath string

	// instancePath is a native path to the guts where the package instance is
	// located, e.g. .cipd/pkgs/<index>/<digest>.
	//
	// Same caveats as for packagePath apply.
	instancePath string
}

// RepairParams is passed to RepairDeployed.
type RepairParams struct {
	// Instance holds the original package data.
	//
	// Must be present if ToRedeploy is not empty. Otherwise not used.
	Instance pkg.Instance

	// ToRedeploy is a list of files that needs to be extracted from the instance
	// and relinked into the site root.
	ToRedeploy []string

	// ToRelink is a list of files that just needs to be relinked into the site
	// root.
	ToRelink []string
}

// Deployer knows how to unzip and place packages into site root directory.
type Deployer interface {
	// DeployInstance installs an instance of a package into the given subdir of
	// the root.
	//
	// It unpacks the package into <base>/.cipd/pkgs/*, and rearranges
	// symlinks to point to unpacked files. It tries to make it as "atomic" as
	// possible. Returns information about the deployed instance.
	//
	// Due to a historical bug, if inst contains any files which are intended to
	// be deployed to `.cipd/*`, they will not be extracted and you'll see
	// warnings logged.
	DeployInstance(ctx context.Context, subdir string, inst pkg.Instance, overrideInstallMode pkg.InstallMode, maxThreads int) (common.Pin, error)

	// CheckDeployed checks whether a given package is deployed at the given
	// subdir.
	//
	// Returns an error if it can't check the package state for some reason.
	// Otherwise returns the state of the package. In particular, if the package
	// is not deployed, returns DeployedPackage{Deployed: false}.
	//
	// Depending on the paranoia mode will also verify that package's files are
	// correctly installed into the site root and will return a list of files
	// that needs to be redeployed (as part of DeployedPackage).
	//
	// If manifest is set to WithManifest, will also fetch and return the instance
	// manifest and install mode. This is optional, since not all callers need it,
	// and it is pretty heavy operation. Any paranoid mode implies WithManifest
	// too.
	CheckDeployed(ctx context.Context, subdir, packageName string, paranoia ParanoidMode, manifest pkg.ManifestMode) (*DeployedPackage, error)

	// FindDeployed returns a list of packages deployed to a site root.
	//
	// It just does a shallow examination of the metadata directory, without
	// paranoid checks that all installed packages are free from corruption.
	FindDeployed(ctx context.Context) (out common.PinSliceBySubdir, err error)

	// RemoveDeployed deletes a package from a subdir given its name.
	RemoveDeployed(ctx context.Context, subdir, packageName string) error

	// RepairDeployed attempts to restore broken deployed instance.
	//
	// Use CheckDeployed first to figure out what parts of the package need
	// repairs.
	//
	// 'pin' indicates an instances that is supposed to be installed in the given
	// subdir. If there's no such package there or its version is different from
	// the one specified in the pin, returns an error.
	RepairDeployed(ctx context.Context, subdir string, pin common.Pin, overrideInstallMode pkg.InstallMode, maxThreads int, params RepairParams) error

	// FS returns an fs.FileSystem rooted at the deployer root dir.
	FS() fs.FileSystem
}

// New return default Deployer implementation.
func New(root string) Deployer {
	var err error
	if root == "" {
		err = errors.Reason("site root path is not provided").Tag(cipderr.BadArgument).Err()
	} else {
		root, err = filepath.Abs(filepath.Clean(root))
		if err != nil {
			err = errors.Annotate(err, "bad site root path").Tag(cipderr.BadArgument).Err()
		}
	}
	if err != nil {
		return errDeployer{err}
	}
	trashDir := filepath.Join(root, fs.SiteServiceDir, "trash")
	return &deployerImpl{fs.NewFileSystem(root, trashDir)}
}

////////////////////////////////////////////////////////////////////////////////
// Implementation that returns error on all requests.

type errDeployer struct{ err error }

func (d errDeployer) DeployInstance(context.Context, string, pkg.Instance, pkg.InstallMode, int) (common.Pin, error) {
	return common.Pin{}, d.err
}

func (d errDeployer) CheckDeployed(context.Context, string, string, ParanoidMode, pkg.ManifestMode) (*DeployedPackage, error) {
	return nil, d.err
}

func (d errDeployer) FindDeployed(context.Context) (out common.PinSliceBySubdir, err error) {
	return nil, d.err
}

func (d errDeployer) RemoveDeployed(context.Context, string, string) error { return d.err }

func (d errDeployer) RepairDeployed(context.Context, string, common.Pin, pkg.InstallMode, int, RepairParams) error {
	return d.err
}

func (d errDeployer) FS() fs.FileSystem { return nil }

////////////////////////////////////////////////////////////////////////////////
// Real deployer implementation.

const (
	// descriptionName is a name of the description file inside the package.
	descriptionName = "description.json"

	// packagesDir is a subdirectory of site root to extract packages to.
	packagesDir = fs.SiteServiceDir + "/pkgs"

	// currentSymlink is a name of a symlink that points to latest deployed version.
	// Used on Linux and Mac.
	currentSymlink = "_current"

	// currentTxt is a name of a text file with instance ID of latest deployed
	// version. Used on Windows.
	currentTxt = "_current.txt"

	// fsLockName is name of a per-package lock file in .cipd/pkgs/<index>/.
	fsLockName = ".lock"

	// fsLockGiveUpDuration is how long to wait for a FS lock before giving up.
	fsLockGiveUpDuration = 20 * time.Minute
)

// description defines the structure of the description.json file located at
// .cipd/pkgs/<foo>/description.json.
type description struct {
	Subdir      string `json:"subdir,omitempty"`
	PackageName string `json:"package_name,omitempty"`
}

// matches is true if subdir and pkg is what's in the description.
//
// nil description doesn't match anything.
func (d *description) matches(subdir, pkg string) bool {
	if d == nil {
		return false
	}
	return d.Subdir == subdir && d.PackageName == pkg
}

// readDescription reads and decodes description JSON from io.Reader.
func readDescription(r io.Reader) (desc *description, err error) {
	blob, err := io.ReadAll(r)
	if err == nil {
		err = json.Unmarshal(blob, &desc)
	}
	return desc, err
}

// writeDescription encodes and writes description JSON to io.Writer.
func writeDescription(d *description, w io.Writer) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// deployerImpl implements Deployer interface.
type deployerImpl struct {
	fs fs.FileSystem
}

func (d *deployerImpl) DeployInstance(ctx context.Context, subdir string, inst pkg.Instance, overrideInstallMode pkg.InstallMode, maxThreads int) (pin common.Pin, err error) {
	if err = common.ValidateSubdir(subdir); err != nil {
		return common.Pin{}, err
	}

	startTS := clock.Now(ctx)

	pin = inst.Pin()
	copyOverrideMsg := ""
	if overrideInstallMode != "" {
		copyOverrideMsg = fmt.Sprintf(" (%s-mode override)", overrideInstallMode)
	}
	logging.Infof(ctx, "Deploying %s into %s(/%s)%s", pin, d.fs.Root(), subdir, copyOverrideMsg)

	// Be paranoid (but not too much).
	if err = common.ValidatePin(pin, common.AnyHash); err != nil {
		return common.Pin{}, err
	}
	if _, err = d.fs.EnsureDirectory(ctx, filepath.Join(d.fs.Root(), subdir)); err != nil {
		return common.Pin{}, errors.Annotate(err, "creating destination subdir").Tag(cipderr.IO).Err()
	}

	// Extract new version to the .cipd/pkgs/* guts. For "symlink" install mode it
	// is the final destination. For "copy" install mode it's a temp destination
	// and files will be moved to the site root later (in addToSiteRoot call).

	// Allocate '.cipd/pkgs/<index>' directory for the (subdir, PackageName) pair
	// or grab an existing one, if any. This operation is atomic.
	pkgPath, err := d.packagePath(ctx, subdir, pin.PackageName, true)
	if err != nil {
		return common.Pin{}, err
	}

	// Concurrently messing with a single pkgPath directory doesn't work well,
	// use exclusive file system lock. In practice it means we aren't allowing
	// installing instances of the *same* package concurrently. Installing
	// different packages at the same time is still allowed.
	unlock, err := d.lockPkg(ctx, pkgPath)
	if err != nil {
		return common.Pin{}, errors.Annotate(err, "failed to acquire FS lock").Tag(cipderr.IO).Err()
	}
	defer unlock()

	// Skip extracting .cipd/* guts if they mistakenly ended up inside the
	// package. Extracting them clobbers REAL guts.
	files := make([]fs.File, 0, len(inst.Files()))
	for _, f := range inst.Files() {
		if name := f.Name(); strings.HasPrefix(name, fs.SiteServiceDir+"/") {
			logging.Warningf(ctx, "[non-fatal] ignoring internal file: %s", name)
		} else {
			files = append(files, f)
		}
	}

	// Unzip the package into the final destination inside .cipd/* guts.
	destPath := filepath.Join(pkgPath, pin.InstanceID)
	if _, err := reader.ExtractFilesTxn(ctx, files, fs.NewDestination(destPath, d.fs), maxThreads, pkg.WithManifest, overrideInstallMode); err != nil {
		return common.Pin{}, err
	}

	// We want to cleanup 'destPath' if something is not right with it.
	deleteFailedInstall := true
	defer func() {
		if deleteFailedInstall {
			logging.Warningf(ctx, "Deploy aborted, cleaning up %s", destPath)
			d.fs.EnsureDirectoryGone(ctx, destPath)
		}
	}()

	// Read and sanity check the manifest.
	newManifest, err := d.readManifest(ctx, destPath)
	if err != nil {
		return common.Pin{}, err
	}

	installMode := newManifest.ActualInstallMode
	if installMode, err = pkg.PickInstallMode(installMode); err != nil {
		return common.Pin{}, err
	}

	// Remember currently deployed version (to remove it later). Do not freak out
	// if it's not there (prevInstanceID == "") or broken (err != nil).
	prevInstanceID, err := d.getCurrentInstanceID(pkgPath)
	prevManifest := pkg.Manifest{}
	if err == nil && prevInstanceID != "" {
		prevManifest, err = d.readManifest(ctx, filepath.Join(pkgPath, prevInstanceID))
	}
	if err != nil {
		logging.Warningf(ctx, "Previous version of the package is broken: %s", err)
		prevManifest = pkg.Manifest{} // to make sure prevManifest.Files == nil.
	}

	// Install all new files to the site root, collect a set of paths (files and
	// directories) that should exist now, to make sure removeFromSiteRoot doesn't
	// delete them later. This is important when updating files to directories:
	// removeFromSiteRoot should not try to delete a directory that used to be
	// a file.
	logging.Infof(ctx, "Moving files to their final destination...")
	keep, err := d.addToSiteRoot(ctx, subdir, newManifest.Files, installMode, pkgPath, destPath)
	if err != nil {
		return common.Pin{}, errors.Annotate(err, "adding files to the site root").Tag(cipderr.IO).Err()
	}

	// Mark installed instance as a current one. After this call the package is
	// considered installed and the function must not fail. All cleanup below is
	// best effort.
	if err = d.setCurrentInstanceID(ctx, pkgPath, pin.InstanceID); err != nil {
		return common.Pin{}, err
	}
	deleteFailedInstall = false

	// Wait for async cleanup to finish.
	logging.Infof(ctx, "Cleaning up...")
	wg := sync.WaitGroup{}
	origPin := pin // to prevent `return common.Pin{}, err` from clobbering it
	defer func() {
		wg.Wait()
		if err == nil {
			logging.Infof(ctx, "Deployed %s in %.1fs", origPin, clock.Since(ctx, startTS).Seconds())
		} else {
			logging.Errorf(ctx, "Failed to deploy %s: %s", origPin, err)
		}
	}()

	// When using 'copy' install mode all files (except .cipdpkg/*) are moved away
	// from 'destPath', leaving only an empty husk with directory structure.
	// Remove it to save some inodes.
	if installMode == pkg.InstallModeCopy {
		wg.Add(1)
		go func() {
			defer wg.Done()
			removeEmptyTree(destPath, func(string) bool { return true })
		}()
	}

	// Remove old instance directory completely.
	if prevInstanceID != "" && prevInstanceID != pin.InstanceID {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.fs.EnsureDirectoryGone(ctx, filepath.Join(pkgPath, prevInstanceID))
		}()
	}

	// Remove no longer present files from the site root directory.
	if len(prevManifest.Files) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.removeFromSiteRoot(ctx, subdir, prevManifest.Files, keep)
		}()
	}

	// Verify it's all right.
	state, err := d.CheckDeployed(ctx, subdir, pin.PackageName, NotParanoid, pkg.WithoutManifest)
	switch {
	case err != nil:
		return common.Pin{}, err
	case !state.Deployed: // should not happen really...
		return common.Pin{}, errors.Reason("the package is reported as not installed, see logs").Tag(cipderr.IO).Err()
	case state.Pin.InstanceID != pin.InstanceID:
		return state.Pin, errors.Reason("other instance (%s) was deployed concurrently", state.Pin.InstanceID).Tag(cipderr.Stale).Err()
	default:
		return pin, nil
	}
}

func (d *deployerImpl) CheckDeployed(ctx context.Context, subdir, pkgname string, par ParanoidMode, m pkg.ManifestMode) (out *DeployedPackage, err error) {
	if err = common.ValidateSubdir(subdir); err != nil {
		return
	}
	if err = common.ValidatePackageName(pkgname); err != nil {
		return
	}
	if err = par.Validate(); err != nil {
		return
	}

	out = &DeployedPackage{Subdir: subdir}

	switch out.packagePath, err = d.packagePath(ctx, subdir, pkgname, false); {
	case err != nil:
		out = nil
		return // this error is fatal, reinstalling probably won't help
	case out.packagePath == "":
		logging.Errorf(ctx, "Failed to figure out packagePath of %q in %q: no matching .cipd/pkgs/*/description.json", pkgname, subdir)
		return // not fully deployed
	}

	current, err := d.getCurrentInstanceID(out.packagePath)
	switch {
	case err != nil:
		logging.Errorf(ctx, "Failed to figure out installed instance ID of %q in %q: %s", pkgname, subdir, err)
		err = nil // this error MAY be recovered from by reinstalling
		return
	case current == "":
		logging.Errorf(ctx, "Failed to figure out installed instance ID of %q in %q: missing _current", pkgname, subdir)
		return // not deployed
	default:
		out.instancePath = filepath.Join(out.packagePath, current)
	}

	// Read the manifest if asked or if doing any paranoid checks (they need the
	// list of files from the manifest).
	if m || par != NotParanoid {
		manifest, err := d.readManifest(ctx, out.instancePath)
		if err != nil {
			logging.Errorf(ctx, "Failed to read the manifest of %s: %s", pkgname, err)
			return out, nil // this error MAY be recovered from by reinstalling
		}
		out.Manifest = &manifest
		out.InstallMode, err = pkg.PickInstallMode(manifest.InstallMode)
		if err != nil {
			return nil, err // this is fatal, the manifest has unrecognized mode
		}
		if manifest.ActualInstallMode == "" {
			// this manifest is from an earlier version of cipd which didn't record
			// actual install mode; these versions of cipd always used the mode
			// specified by manifest.InstallMode or the platform default, so
			// PickInstallMode already did all the work for us.
			out.ActualInstallMode = out.InstallMode
		} else {
			out.ActualInstallMode, err = pkg.PickInstallMode(manifest.ActualInstallMode)
			if err != nil {
				return nil, err // this is fatal, the manifest has unrecognized mode
			}
		}
	}

	// Yay, found something in a pretty healthy state.
	out.Deployed = true
	out.Pin = common.Pin{PackageName: pkgname, InstanceID: current}

	// checkIntegrity needs DeployedPackage with Pin set, so call it last.
	if par != NotParanoid {
		out.ToRedeploy, out.ToRelink = d.checkIntegrity(ctx, out, par)
	}

	return
}

func (d *deployerImpl) FindDeployed(ctx context.Context) (common.PinSliceBySubdir, error) {
	// Directories with packages are direct children of .cipd/pkgs/.
	pkgs := filepath.Join(d.fs.Root(), filepath.FromSlash(packagesDir))
	entries, err := os.ReadDir(pkgs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Annotate(err, "scanning packages directory").Tag(cipderr.IO).Err()
	}

	found := common.PinMapBySubdir{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Read the description and the 'current' link.
		pkgPath := filepath.Join(pkgs, entry.Name())
		desc, err := d.readDescription(ctx, pkgPath)
		if err != nil || desc == nil {
			continue
		}
		currentID, err := d.getCurrentInstanceID(pkgPath)
		if err != nil || currentID == "" {
			continue
		}

		// Ignore duplicate entries, they can appear if someone messes with pkgs/*
		// structure manually.
		if _, ok := found[desc.Subdir][desc.PackageName]; !ok {
			if _, ok := found[desc.Subdir]; !ok {
				found[desc.Subdir] = common.PinMap{}
			}
			found[desc.Subdir][desc.PackageName] = currentID
		}
	}

	return found.ToSlice(), nil
}

func (d *deployerImpl) RemoveDeployed(ctx context.Context, subdir, packageName string) error {
	logging.Infof(ctx, "Removing %s from %s(/%s)", packageName, d.fs.Root(), subdir)

	deployed, err := d.CheckDeployed(ctx, subdir, packageName, NotParanoid, pkg.WithManifest)
	switch {
	case err != nil:
		// This error is unrecoverable, we can't even figure out whether the package
		// is installed. The state is too broken to mess with it.
		return err
	case deployed.packagePath == "":
		// No gut directory for the package => the package is not installed at all.
		if deployed.Deployed {
			panic("impossible")
		}
		return nil
	case deployed.Deployed:
		d.removeFromSiteRoot(ctx, subdir, deployed.Manifest.Files, nil)
	default:
		// The package was partially installed in the guts, but not into the site
		// root. We can just remove the guts thus forgetting about the package.
		logging.Warningf(ctx, "Package %s is partially installed, removing it", packageName)
	}
	if err := d.fs.EnsureDirectoryGone(ctx, deployed.packagePath); err != nil {
		return errors.Annotate(err, "removing the deployed package directory").Tag(cipderr.IO).Err()
	}
	return nil
}

func (d *deployerImpl) RepairDeployed(ctx context.Context, subdir string, pin common.Pin, overrideInstallMode pkg.InstallMode, maxThreads int, params RepairParams) error {
	switch {
	case len(params.ToRedeploy) != 0 && params.Instance == nil:
		return errors.Reason("if ToRedeploy is not empty, Instance must be given too").Tag(cipderr.BadArgument).Err()
	case params.Instance != nil && params.Instance.Pin() != pin:
		return errors.Reason("expecting instance with pin %s, got %s", pin, params.Instance.Pin()).Tag(cipderr.BadArgument).Err()
	}

	// Note that we can slightly optimize the repairs of packages in 'copy'
	// install mode by extracting directly into the site root. But this
	// complicates the code and introduces a "unique" code path, not exercised by
	// DeployInstance. Instead we prefer a bit slower code that resembles
	// DeployInstance in its logic: it extracts everything into the instance
	// directory in the guts, and then symlinks/moves it to the site root.

	// Check that the package we are repairing is still installed and grab its
	// manifest (for the install mode and list of files).
	p, err := d.CheckDeployed(ctx, subdir, pin.PackageName, NotParanoid, pkg.WithManifest)
	switch {
	case err != nil:
		return err
	case !p.Deployed:
		return errors.Reason("the package %s is not deployed any more, refusing to recover", pin.PackageName).Tag(cipderr.Stale).Err()
	case p.Pin != pin:
		return errors.Reason("expected to find pin %s, got %s, refusing to recover", pin, p.Pin).Tag(cipderr.Stale).Err()
	case p.packagePath == "":
		panic("impossible, packagePath cannot be empty for deployed pkg")
	case p.instancePath == "":
		panic("impossible, instancePath cannot be empty for deployed pkg")
	}

	installMode := p.InstallMode
	if overrideInstallMode != "" {
		installMode = overrideInstallMode
	}

	// See the comment about locking in DeployInstance.
	unlock, err := d.lockPkg(ctx, p.packagePath)
	if err != nil {
		return errors.Annotate(err, "failed to acquire FS lock").Tag(cipderr.IO).Err()
	}
	defer unlock()

	// manifest contains all files that should be present in a healthy deployment.
	manifest := make(map[string]pkg.FileInfo, len(p.Manifest.Files))
	for _, f := range p.Manifest.Files {
		manifest[f.Name] = f
	}

	// repairableFiles is subset of files in 'manifest' we are able to repair,
	// given data we have.
	var repairableFiles map[string]fs.File
	if params.Instance != nil {
		repairableFiles = make(map[string]fs.File, len(params.Instance.Files()))
		for _, f := range params.Instance.Files() {
			// Ignore files not in the manifest (usually .cipd/* guts skipped during
			// the initial deployment)
			if _, ok := manifest[f.Name()]; ok {
				repairableFiles[f.Name()] = f
			}
		}
	} else {
		// Without pkg.Instance present we can repair only symlinks based on info
		// in the manifest.
		repairableFiles = map[string]fs.File{}
		for _, f := range p.Manifest.Files {
			if f.Symlink != "" {
				repairableFiles[f.Name] = &symlinkFile{name: f.Name, target: f.Symlink}
			}
		}
	}

	// Names of files we want to extract into the instance gut directory. Note
	// that ToRelink set MAY include symlinks we need to restore in the guts, if
	// the package originally had symlinks.
	broken := make([]string, 0, len(params.ToRedeploy))
	broken = append(broken, params.ToRedeploy...)
	for _, name := range params.ToRelink {
		if manifest[name].Symlink != "" {
			broken = append(broken, name)
		}
	}

	failed := false

	// Collect corresponding []fs.File entries and extract them into the gut
	// directory. This restores all broken files and symlinks there, but doesn't
	// yet link them to the site root.
	repair := make([]fs.File, 0, len(broken))
	for _, name := range broken {
		if f := repairableFiles[name]; f != nil {
			repair = append(repair, f)
		} else {
			logging.Errorf(ctx, "Can't repair %q, the source is not available", name)
			failed = true
		}
	}
	if len(repair) != 0 {
		logging.Infof(ctx, "Repairing %d files...", len(repair))
		dest := fs.ExistingDestination(p.instancePath, d.fs)
		if _, err := reader.ExtractFiles(ctx, repair, dest, maxThreads, pkg.WithoutManifest, overrideInstallMode); err != nil {
			return err
		}
	}

	// Finally relink/move everything into the site root.
	infos := make([]pkg.FileInfo, 0, len(params.ToRedeploy)+len(params.ToRelink))
	add := func(name string) {
		if info, ok := manifest[name]; ok {
			infos = append(infos, info)
		} else {
			logging.Errorf(ctx, "Can't relink %q, unknown file", name)
			failed = true
		}
	}
	for _, name := range params.ToRedeploy {
		add(name)
	}
	for _, name := range params.ToRelink {
		add(name)
	}
	logging.Infof(ctx, "Relinking %d files...", len(infos))
	if _, err := d.addToSiteRoot(ctx, p.Subdir, infos, installMode, p.packagePath, p.instancePath); err != nil {
		return errors.Annotate(err, "adding files to the site root").Tag(cipderr.IO).Err()
	}

	// Cleanup empty directories left in the guts after files have been moved
	// away, just like DeployInstance does. Best effort.
	if installMode == pkg.InstallModeCopy {
		_, _ = removeEmptyTree(p.instancePath, func(string) bool { return true })
	}

	if failed {
		return errors.Reason("repair of %s failed, see logs", p.Pin.PackageName).Tag(cipderr.Stale).Err()
	}
	return nil
}

func (d *deployerImpl) TempDir(ctx context.Context, prefix string, mode os.FileMode) (string, error) {
	dir, err := d.fs.EnsureDirectory(ctx, filepath.Join(d.fs.Root(), fs.SiteServiceDir, "tmp"))
	if err != nil {
		return "", errors.Annotate(err, "creating temp directory").Tag(cipderr.IO).Err()
	}
	tmp, err := fs.TempDir(dir, prefix, mode)
	if err != nil {
		return "", errors.Annotate(err, "creating temp directory").Tag(cipderr.IO).Err()
	}
	return tmp, nil
}

func (d *deployerImpl) FS() fs.FileSystem {
	return d.fs
}

////////////////////////////////////////////////////////////////////////////////
// Symlink fs.File implementation used by RepairDeployed.

type symlinkFile struct {
	name   string
	target string
}

func (f *symlinkFile) Name() string                   { return f.name }
func (f *symlinkFile) Size() uint64                   { return 0 }
func (f *symlinkFile) Executable() bool               { return false }
func (f *symlinkFile) Writable() bool                 { return false }
func (f *symlinkFile) ModTime() time.Time             { return time.Time{} }
func (f *symlinkFile) Symlink() bool                  { return true }
func (f *symlinkFile) SymlinkTarget() (string, error) { return f.target, nil }
func (f *symlinkFile) WinAttrs() fs.WinAttrs          { return 0 }
func (f *symlinkFile) Open() (io.ReadCloser, error) {
	return nil, errors.Reason("can't open a symlink").Tag(cipderr.IO).Err()
}

////////////////////////////////////////////////////////////////////////////////
// Utility methods.

type numSet sort.IntSlice

func (s *numSet) addNum(n int) {
	idx := sort.IntSlice((*s)).Search(n)
	if idx == len(*s) {
		// it's insertion point is off the end of the slice
		*s = append(*s, n)
	} else if (*s)[idx] != n {
		// it's insertion point is inside the slice, but is not present.
		*s = append(*s, 0)
		copy((*s)[idx+1:], (*s)[idx:])
		(*s)[idx] = n
	}
	// it's already present in the slice
}

func (s *numSet) smallestNewNum() int {
	prev := -1
	for _, n := range *s {
		if n-1 != prev {
			return prev + 1
		}
		prev = n
	}
	return prev + 1
}

// packagePath returns a path to a package directory in .cipd/pkgs/.
//
// This will scan all directories under pkgs, looking for a description.json. If
// an old-style package folder is encountered (e.g. has an instance folder and
// current manifest, but doesn't have a description.json), the description.json
// will be added.
//
// If no suitable path is found and allocate is true, this will create a new
// directory with an accompanying description.json. Otherwise this returns "".
func (d *deployerImpl) packagePath(ctx context.Context, subdir, pkg string, allocate bool) (string, error) {
	if err := common.ValidateSubdir(subdir); err != nil {
		return "", err
	}

	if err := common.ValidatePackageName(pkg); err != nil {
		return "", err
	}

	rel := filepath.FromSlash(packagesDir)
	abs, err := d.fs.RootRelToAbs(rel)
	if err != nil {
		return "", errors.Annotate(err, "resolving absolute path of %q", rel).Tag(cipderr.IO).Err()
	}

	seenNumbers, curPkgs := d.resolveValidPackageDirs(ctx, abs)
	if cur, ok := curPkgs[description{subdir, pkg}]; ok {
		return cur, nil
	}

	if !allocate {
		return "", nil
	}

	// We didn't find one, so we have to make one.
	if _, err := d.fs.EnsureDirectory(ctx, abs); err != nil {
		return "", errors.Annotate(err, "creating packages directory").Tag(cipderr.IO).Err()
	}

	// Take the last 2 components from the pkg path.
	pkgParts := strings.Split(pkg, "/")
	prefix := ""
	if len(pkgParts) > 2 {
		prefix = strings.Join(pkgParts[len(pkgParts)-2:], "_")
	} else {
		prefix = strings.Join(pkgParts, "_")
	}

	// 0777 allows umask to take effect
	tmpDir, err := d.TempDir(ctx, prefix, 0777)
	if err != nil {
		return "", errors.Annotate(err, "creating new package temp dir").Tag(cipderr.IO).Err()
	}
	defer d.fs.EnsureDirectoryGone(ctx, tmpDir)
	err = d.fs.EnsureFile(ctx, filepath.Join(tmpDir, descriptionName), func(f *os.File) error {
		return writeDescription(&description{Subdir: subdir, PackageName: pkg}, f)
	})
	if err != nil {
		return "", errors.Annotate(err, "creating new package description.json").Tag(cipderr.IO).Err()
	}

	// Now we have to find a suitable index folder for it.
	waiter, cancel := retry.Waiter(ctx, "Collision when picking an index", time.Minute)
	defer cancel()
	for {
		// Pick the next possible candidate by enumerating them until there's an
		// empty "slot" or we discover an existing directory with our package.
		var pkgPath string
		for {
			n := seenNumbers.smallestNewNum()
			seenNumbers.addNum(n)
			pkgPath = filepath.Join(abs, strconv.Itoa(n))
			if _, err := os.Stat(pkgPath); err != nil {
				break // either doesn't exist or broken, will find out below
			}
			// Maybe someone else created a slot for us already.
			if desc, _ := d.readDescription(ctx, pkgPath); desc.matches(subdir, pkg) {
				return pkgPath, nil
			}
		}

		// Try to occupy the slot. We use os.Rename instead of d.fs.Replace because
		// we want it to fail if the target directory already exists.
		switch err := os.Rename(tmpDir, pkgPath); {
		case err == nil:
			return pkgPath, nil

		// Note that IsAccessDenied is Windows-specific check. ERROR_ACCESS_DENIED
		// happens on Windows instead of ENOTEMPTY, see the following explanation:
		// https://github.com/golang/go/issues/14527#issuecomment-189755676
		case !fs.IsNotEmpty(err) && !fs.IsAccessDenied(err):
			return "", errors.Annotate(err, "unknown error when creating pkg dir %q", pkgPath).Tag(cipderr.IO).Err()
		}

		// os.Rename failed with ENOTEMPTY, that means that another client wrote
		// this directory. Maybe it is what we really want.
		switch desc, err := d.readDescription(ctx, pkgPath); {
		case desc.matches(subdir, pkg):
			return pkgPath, nil
		case err != nil:
			logging.Warningf(ctx, "Skipping %q: %s", pkgPath, err)
		}

		// Either description.json is garbage or it's some other package. Pick
		// another slot a bit later, to give a chance for the concurrent writers to
		// proceed too.
		if err := waiter(); err != nil {
			return "", errors.Annotate(err, "unable to find valid index for package %q in %s", pkg, abs).Tag(cipderr.IO).Err()
		}
	}
}

// lockFS grabs an exclusive file system lock and returns a callback that
// releases it.
//
// If the lock is already taken, calls waiter(), assuming it will sleep a bit.
//
// Retries either until the lock is successfully acquired or waiter() returns
// an error.
//
// Initialized in init() of a *.go file that actually implements the locking,
// if any.
var lockFS func(path string, waiter func() error) (unlock func() error, err error)

// lockPkg takes an exclusive file system lock around a single package.
//
// Noop if 'lockFS' is nil (i.e. there's no locking implementation available).
func (d *deployerImpl) lockPkg(ctx context.Context, pkgPath string) (unlock func(), err error) {
	if lockFS == nil {
		return func() {}, nil
	}
	waiter, cancel := retry.Waiter(ctx, "Could not acquire FS lock", fsLockGiveUpDuration)
	defer cancel()
	unlocker, err := lockFS(filepath.Join(pkgPath, fsLockName), waiter)
	if err != nil {
		return nil, err
	}
	return func() {
		if uerr := unlocker(); uerr != nil {
			logging.Warningf(ctx, "Failed to release FS lock: %s", err)
		}
	}, nil
}

type byLenThenAlpha []string

func (b byLenThenAlpha) Len() int      { return len(b) }
func (b byLenThenAlpha) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byLenThenAlpha) Less(i, j int) bool {
	return sortby.Chain{
		func(i, j int) bool { return len(b[i]) < len(b[j]) },
		func(i, j int) bool { return b[i] < b[j] },
	}.Use(i, j)
}

// resolveValidPackageDirs scans the .cipd/pkgs dir and returns:
//   - a numeric set of all number-style directories seen.
//   - a map of description (e.g. subdir + pkgname) to the correct pkg folder
//
// This also will delete (EnsureDirectoryGone) any folders or files in the pkgs
// directory which are:
//   - invalid (contain no description.json and no current instance symlink)
//   - duplicate (where multiple directories contain the same description.json)
//
// Duplicate detection always prefers the folder with the shortest path name
// that sorts alphabetically earlier.
func (d *deployerImpl) resolveValidPackageDirs(ctx context.Context, pkgsAbsDir string) (numbered numSet, all map[description]string) {
	files, err := os.ReadDir(pkgsAbsDir)
	if err != nil && !os.IsNotExist(err) {
		logging.Errorf(ctx, "Can't read packages dir %q: %s", pkgsAbsDir, err)
		return
	}

	allWithDups := map[description][]string{}

	for _, f := range files {
		fullPkgPath := filepath.Join(pkgsAbsDir, f.Name())
		description, err := d.readDescription(ctx, fullPkgPath)
		if description == nil || err != nil {
			if err == nil {
				err = errors.Reason("missing description.json and current instance").Err()
			}
			logging.Warningf(ctx, "removing junk directory: %q (%s)", fullPkgPath, err)
			if err := d.fs.EnsureDirectoryGone(ctx, fullPkgPath); err != nil {
				logging.Warningf(ctx, "while removing junk directory: %q (%s)", fullPkgPath, err)
			}
			continue
		}
		allWithDups[*description] = append(allWithDups[*description], fullPkgPath)
	}

	all = make(map[description]string, len(allWithDups))
	for desc, possibilities := range allWithDups {
		sort.Sort(byLenThenAlpha(possibilities))

		// keep track of all non-deleted numeric children of .cipd/pkgs
		if n, err := strconv.Atoi(filepath.Base(possibilities[0])); err == nil {
			numbered.addNum(n)
		}

		all[desc] = possibilities[0]

		if len(possibilities) == 1 {
			continue
		}
		for _, extra := range possibilities[1:] {
			logging.Warningf(ctx, "removing duplicate directory: %q", extra)
			if err := d.fs.EnsureDirectoryGone(ctx, extra); err != nil {
				logging.Warningf(ctx, "while removing duplicate directory: %q (%s)", extra, err)
			}
		}
	}

	return
}

// getCurrentInstanceID returns instance ID of currently installed instance
// given a path to a package directory (.cipd/pkgs/<name>).
//
// It returns ("", nil) if no package is installed there.
func (d *deployerImpl) getCurrentInstanceID(packageDir string) (string, error) {
	var current string
	var err error
	if runtime.GOOS == "windows" {
		var bytes []byte
		bytes, err = os.ReadFile(filepath.Join(packageDir, currentTxt))
		if err == nil {
			current = strings.TrimSpace(string(bytes))
		}
	} else {
		current, err = os.Readlink(filepath.Join(packageDir, currentSymlink))
	}
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Annotate(err, "reading the current instance ID").Tag(cipderr.IO).Err()
	}
	if err = common.ValidateInstanceID(current, common.AnyHash); err != nil {
		return "", errors.Annotate(err, "pointer to currently installed instance doesn't look like a valid instance ID").Tag(cipderr.IO).Err()
	}
	return current, nil
}

// setCurrentInstanceID changes a pointer to currently installed instance ID.
//
// It takes a path to a package directory (.cipd/pkgs/<name>) as input.
func (d *deployerImpl) setCurrentInstanceID(ctx context.Context, packageDir, instanceID string) error {
	if err := common.ValidateInstanceID(instanceID, common.AnyHash); err != nil {
		return err
	}
	var err error
	if runtime.GOOS == "windows" {
		err = fs.EnsureFile(
			ctx, d.fs, filepath.Join(packageDir, currentTxt),
			strings.NewReader(instanceID))
	} else {
		err = d.fs.EnsureSymlink(ctx, filepath.Join(packageDir, currentSymlink), instanceID)
	}
	if err != nil {
		return errors.Annotate(err, "writing the current instance ID").Tag(cipderr.IO).Err()
	}
	return nil
}

// readDescription reads the package description.json given a path to a package
// directory.
//
// As a backwards-compatibility measure, it will also upgrade CIPD < 1.4 folders
// to contain a description.json. Previous to 1.4, package folders only had
// instance subfolders, and the current instances' manifest was used to
// determine the package name. Versions prior to 1.4 also installed all packages
// at the base (subdir ""), hence the implied subdir location here.
//
// Returns (nil, nil) if no description.json exists and there are no instance
// folders present.
func (d *deployerImpl) readDescription(ctx context.Context, pkgDir string) (*description, error) {
	descriptionPath := filepath.Join(pkgDir, descriptionName)
	r, err := os.Open(descriptionPath)
	switch {
	case os.IsNotExist(err):
		// try fixup
		break
	case err == nil:
		defer r.Close()
		return readDescription(r)
	default:
		return nil, err
	}

	// see if this is a pre 1.4 directory
	currentID, err := d.getCurrentInstanceID(pkgDir)
	if err != nil {
		return nil, err
	}

	if currentID == "" {
		logging.Warningf(ctx, "No current instance id in %s", pkgDir)
		return nil, nil
	}

	manifest, err := d.readManifest(ctx, filepath.Join(pkgDir, currentID))
	if err != nil {
		return nil, err
	}

	desc := &description{
		PackageName: manifest.PackageName,
	}
	// To handle the case where some other user owns these directories, all errors
	// from here to the end are treated as warnings.
	err = d.fs.EnsureFile(ctx, descriptionPath, func(f *os.File) error {
		return writeDescription(desc, f)
	})
	if err != nil {
		logging.Warningf(ctx, "Unable to create description.json: %s", err)
	}
	return desc, nil
}

// readManifest reads package manifest given a path to a package instance
// (.cipd/pkgs/<name>/<instance id>).
func (d *deployerImpl) readManifest(ctx context.Context, instanceDir string) (pkg.Manifest, error) {
	manifestPath := filepath.Join(instanceDir, filepath.FromSlash(pkg.ManifestName))
	r, err := os.Open(manifestPath)
	if err != nil {
		return pkg.Manifest{}, errors.Annotate(err, "opening manifest file").Tag(cipderr.IO).Err()
	}
	defer r.Close()
	manifest, err := pkg.ReadManifest(r)
	if err != nil {
		return pkg.Manifest{}, err
	}
	// Older packages do not have Files section in the manifest, so reconstruct it
	// from actual files on disk.
	if len(manifest.Files) == 0 {
		if manifest.Files, err = scanPackageDir(ctx, instanceDir); err != nil {
			return pkg.Manifest{}, errors.Annotate(err, "scanning package directory").Tag(cipderr.IO).Err()
		}
	}
	return manifest, nil
}

// addToSiteRoot moves or symlinks files into the site root directory (depending
// on passed installMode).
//
// On success returns a set of all file system paths that should be exempt from
// deletion in removeFromSiteRoot.
//
// This is important when a file is replaced with a directory during an in-place
// upgrade. Such file is no longer directly listed in the package manifest,
// nonetheless it exists as a directory and must not be removed.
//
// Similarly when a directory is replaced with a symlink, we should not attempt
// to delete paths from within this directory (these paths now point to
// completely different files).
func (d *deployerImpl) addToSiteRoot(ctx context.Context, subdir string, files []pkg.FileInfo, installMode pkg.InstallMode, pkgDir, srcDir string) (*pathTree, error) {
	caseSens, err := d.fs.CaseSensitive()
	if err != nil {
		return nil, errors.Annotate(err, "checking file system case sensitivity").Tag(cipderr.IO).Err()
	}

	// Build a set of all added paths (including intermediary directories). It
	// will be used to ensure all intermediary file system nodes are directories,
	// not symlinks (more about this below).
	touched := newPathTree(caseSens, len(files))
	for _, f := range files {
		// Mark the file and all its parents and children as alive. For file "a/b/c"
		// this adds ["a", "a/b", "a/b/c", "a/b/c/*"] to the set.
		//
		// Marking all children as alive is important for the case when a regular
		// directory "a/b/c" (e.g. with a file "a/b/c/d" inside) is converted to
		// a symlink "a/b/c". In this case "a/b/c/d" is technically no longer part
		// of the package, and removeFromSiteRoot will attempt to remove it, and may
		// even succeed (by following "a/b/c" symlink and removing some completely
		// different "d").
		touched.add(filepath.Join(subdir, filepath.FromSlash(f.Name)))
	}

	// Names of files in CIPD package manifests NEVER have symlinks as part of
	// paths. If some intermediary node in pathTree is currently a symlink on
	// the disk, it means we are upgrading this symlink to be a directory. Remove
	// such symlinks right away. If we don't do this, fs.Replace below will follow
	// these symlinks and create files in wrong places.
	touched.visitIntermediatesBF(subdir, func(rootRel string) bool {
		abs, err := d.fs.RootRelToAbs(rootRel)
		if err != nil {
			logging.Warningf(ctx, "Invalid relative path %q: %s", rootRel, err)
			return true
		}
		if fi, err := os.Lstat(abs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(abs); err != nil {
				logging.Warningf(ctx, "Failed to delete symlink %q: %s", rootRel, err)
			}
		}
		return true
	})

	// Finally create all leaf files.
	for _, f := range files {
		// Native path relative to the subdir, e.g. bin/tool
		relPath := filepath.FromSlash(f.Name)
		// Native absolute path, e.g. <base>/<subdir>/bin/tool
		destAbs, err := d.fs.RootRelToAbs(filepath.Join(subdir, relPath))
		if err != nil {
			logging.Warningf(ctx, "Invalid relative path %q: %s", relPath, err)
			return nil, err
		}
		switch installMode {
		case pkg.InstallModeSymlink:
			// e.g. <base>/.cipd/pkgs/name/_current/bin/tool
			targetAbs := filepath.Join(pkgDir, currentSymlink, relPath)
			// e.g. ../.cipd/pkgs/name/_current/bin/tool
			// has more `../` depending on subdir
			targetRel, err := filepath.Rel(filepath.Dir(destAbs), targetAbs)
			if err != nil {
				logging.Warningf(
					ctx, "Can't get relative path from %s to %s",
					filepath.Dir(destAbs), targetAbs)
				return nil, err
			}
			if err = d.fs.EnsureSymlink(ctx, destAbs, targetRel); err != nil {
				logging.Warningf(ctx, "Failed to create symlink for %s", relPath)
				return nil, err
			}
		case pkg.InstallModeCopy:
			// E.g. <base>/.cipd/pkgs/name/<id>/bin/tool.
			srcAbs := filepath.Join(srcDir, relPath)
			if err := d.fs.Replace(ctx, srcAbs, destAbs); err != nil {
				logging.Warningf(ctx, "Failed to move %s to %s: %s", srcAbs, destAbs, err)
				return nil, err
			}
		default:
			// Should not happen. ValidateInstallMode checks this.
			panic("impossible state")
		}
	}
	return touched, nil
}

// removeFromSiteRoot deletes files from the site root directory unless they
// are present in the 'keep' set.
//
// Best effort. Logs errors and carries on.
func (d *deployerImpl) removeFromSiteRoot(ctx context.Context, subdir string, files []pkg.FileInfo, keep *pathTree) {
	dirsToCleanup := stringset.New(0)

	for _, f := range files {
		rootRel := filepath.Join(subdir, filepath.FromSlash(f.Name))
		if keep.has(rootRel) {
			continue
		}
		absPath, err := d.fs.RootRelToAbs(rootRel)
		if err != nil {
			logging.Warningf(ctx, "Refusing to remove %q: %s", f.Name, err)
			continue
		}
		if err := d.fs.EnsureFileGone(ctx, absPath); err != nil {
			logging.Warningf(ctx, "Failed to remove a file from the site root: %s", err)
		} else {
			dirsToCleanup.Add(filepath.Dir(absPath))
		}
	}

	if dirsToCleanup.Len() != 0 {
		subdirAbs, err := d.fs.RootRelToAbs(subdir)
		if err != nil {
			logging.Warningf(ctx, "Can't resolve relative %q to absolute path: %s", subdir, err)
		} else {
			removeEmptyTrees(ctx, subdirAbs, dirsToCleanup)
		}
	}
}

// isPresentInSite checks whether the given file is installed in the site root.
//
// Optionally follows symlinks.
//
// If the file can't be checked for some reason, logs the error and returns
// false.
func (d *deployerImpl) isPresentInSite(ctx context.Context, subdir string, f pkg.FileInfo, followSymlinks bool) bool {
	absPath, err := d.fs.RootRelToAbs(filepath.Join(subdir, filepath.FromSlash(f.Name)))
	if err != nil {
		panic(err) // should not happen for files present in installed packages
	}

	stat := d.fs.Lstat
	if followSymlinks {
		stat = d.fs.Stat
	}

	// TODO(vadimsh): Use result of this call to check the correctness of file
	// mode of the deployed file. Also use ModTime to detect modifications to the
	// file content in higher paranoia modes.
	switch _, err = stat(ctx, absPath); {
	case err == nil:
		return true
	case os.IsNotExist(err):
		return false
	default:
		logging.Warningf(ctx, "Failed to check presence of %q, assuming it needs repair: %s", f.Name, err)
		return false
	}
}

// isPresentInGuts checks whether the given file exists in .cipd/* guts.
//
// Doesn't follow symlinks.
//
// If the file can't be checked for some reason, logs the error and returns
// false.
func (d *deployerImpl) isPresentInGuts(ctx context.Context, instDir string, f pkg.FileInfo) bool {
	absPath, err := d.fs.RootRelToAbs(filepath.Join(instDir, filepath.FromSlash(f.Name)))
	if err != nil {
		panic(err) // should not happen for files present in installed packages
	}

	switch _, err = d.fs.Lstat(ctx, absPath); {
	case err == nil:
		return true
	case os.IsNotExist(err):
		return false
	default:
		logging.Warningf(ctx, "Failed to check presence of %q, assuming it needs repair: %s", f.Name, err)
		return false
	}
}

// checkIntegrity verifies the given deployed package is correctly installed and
// returns a list of files to relink and to redeploy if something is broken.
//
// See DeployedPackage struct for definition of "relink" and "redeploy".
func (d *deployerImpl) checkIntegrity(ctx context.Context, p *DeployedPackage, mode ParanoidMode) (redeploy, relink []string) {
	logging.Debugf(ctx, "Checking integrity of %q deployment in %q mode...", p.Pin.PackageName, mode)

	// TODO(vadimsh): Understand mode == CheckIntegrity.

	// Examine files that are supposed to be installed.
	for _, f := range p.Manifest.Files {
		switch {
		case d.isPresentInSite(ctx, p.Subdir, f, true): // no need to repair
			continue
		case f.Symlink == "": // a regular file (not a symlink)
			switch {
			case p.InstallMode == pkg.InstallModeCopy:
				// In 'copy' mode regular files are stored in the site root directly.
				// If they are gone, we need to refetch the package to restore them.
				redeploy = append(redeploy, f.Name)
			case d.isPresentInGuts(ctx, p.instancePath, f):
				// This is 'symlink' mode and the original file in .cipd guts exist. We
				// only need to relink the file then to repair it.
				relink = append(relink, f.Name)
			default:
				// This is 'symlink' mode, but the original file in .cipd guts is gone,
				// so we need to refetch it.
				redeploy = append(redeploy, f.Name)
			}
		case !filepath.IsAbs(filepath.FromSlash(f.Symlink)): // a relative symlink
			// We can restore it right away, all necessary information is in the
			// manifest. Note that CIPD packages cannot have invalid relative
			// symlinks, so if we see a broken one we know it is a corruption.
			relink = append(relink, f.Name)
		default:
			// This is a broken absolute symlink. Several possibilities here:
			//  1. The symlink file itself in the site root is missing.
			//  2. This is 'symlink' mode, and the symlink in .cipd/guts is missing.
			//  3. Both CIPD-managed symlinks exist and the external file is missing.
			//     Such symlink is NOT broken. If we try to "repair" it, we end up in
			//     the same state anyway: absolute symlinks point to files outside of
			//     our control.
			switch {
			case !d.isPresentInSite(ctx, p.Subdir, f, false):
				// The symlink in the site root is gone, need to restore it.
				relink = append(relink, f.Name)
			case p.InstallMode == pkg.InstallModeSymlink && !d.isPresentInGuts(ctx, p.instancePath, f):
				// The symlink in the guts is gone, need to restore it.
				relink = append(relink, f.Name)
			default:
				// Both CIPD-managed symlinks are fine, nothing to restore.
			}
		}
	}

	return redeploy, relink
}

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

// pathTree is a tree of native file system paths relative to the deployer root.
//
// It is not a full representation of the file system, just a subset of paths
// involved during deployment.
//
// Think of it as a tree where each node (intermediate and leafs) is a path and
// leaf nodes additionally represent infinite subtrees rooted at these leafs.
//
// For example: ["a", "a/b", "a/b/c", "a/b/c/*"]. Here ["a", "a/b"] are
// intermediate nodes, "a/b/c" is a leaf and anything under "a/b/c" is also
// considered to be part of the tree (e.g. has("a/b/c/d") returns true).
//
// All methods assume paths have been properly cleaned already and do not have
// "." or "..".
type pathTree struct {
	caseSensitive bool // true to treat file case as important

	nodes stringset.Set // intermediate tree nodes
	leafs stringset.Set // leafs (also roots of their infinite subtrees)
}

// newPathTree initializes the path tree, allocating the given capacity.
func newPathTree(caseSensitive bool, capacity int) *pathTree {
	return &pathTree{
		caseSensitive: caseSensitive,
		nodes:         stringset.New(capacity / 5), // educated guess
		leafs:         stringset.New(capacity),     // exact
	}
}

// add adds a native path 'rel', all its parents and all its children to the
// path tree.
func (p *pathTree) add(rel string) {
	if !p.caseSensitive {
		rel = strings.ToLower(rel)
	}
	p.leafs.Add(rel)
	parentDirs(rel, func(par string) bool {
		p.nodes.Add(par)
		return true
	})
}

// has returns true if a native path 'rel' is in the tree.
//
// nil pathTree is considered empty.
func (p *pathTree) has(rel string) bool {
	if p == nil {
		return false
	}

	if !p.caseSensitive {
		rel = strings.ToLower(rel)
	}

	// It matches some added entry exactly?
	if p.leafs.Has(rel) {
		return true
	}
	// Was added as a parent of some entry?
	if p.nodes.Has(rel) {
		return true
	}

	// Maybe it has some added entry as its parent?
	found := false
	parentDirs(rel, func(par string) bool {
		found = p.leafs.Has(par)
		return !found
	})
	return found
}

// visitIntermediatesBF calls cb() for non-leaf nodes in the tree that are not
// a part of subdir in breadth-first order (i.e. smallest paths come first).
//
// On case-insensitive file systems, all visited nodes are in lowercase,
// regardless in what case they were added to the tree.
//
// nil pathTree is considered empty.
func (p *pathTree) visitIntermediatesBF(subdir string, cb func(string) bool) {
	if p == nil {
		return
	}

	// Normalize the subdir to be used for strings.HasPrefix(...) check. Note that
	// if 'subdir' is "" or "." (e.g. when installing a package into the site
	// root), filepath.Clean(...) returns ".", which screws up HasPrefix checks
	// below (since paths we are checking do not have leading "./"). Convert "."
	// to "" to handle this.
	subdir = filepath.Clean(subdir)
	if subdir == "." {
		subdir = ""
	} else {
		subdir += string(filepath.Separator)
		if !p.caseSensitive {
			subdir = strings.ToLower(subdir)
		}
	}

	nodes := p.nodes.ToSlice()
	sort.Strings(nodes)
	for _, path := range nodes {
		if strings.HasPrefix(path, subdir) && !cb(path) {
			return
		}
	}
}

// parentDirs takes a native path "a/b/c/d" and calls 'cb' with "a", "a/b"
// and "a/b/c".
//
// Purely lexicographical operation.
//
// Stops if the callback returns false. Note that it doesn't visit 'rel' itself.
func parentDirs(rel string, cb func(p string) bool) {
	for i, r := range rel {
		if r == filepath.Separator && !cb(rel[:i]) {
			return
		}
	}
}

// scanPackageDir finds a set of regular files (and symlinks) in a package
// instance directory and returns them as FileInfo structs (with slash-separated
// paths relative to dir directory). Skips package service directories (.cipdpkg
// and .cipd) since they contain package deployer gut files, not something that
// needs to be deployed.
func scanPackageDir(ctx context.Context, dir string) ([]pkg.FileInfo, error) {
	var out []pkg.FileInfo
	err := filepath.WalkDir(dir, func(path string, entry iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == pkg.ServiceDir || rel == fs.SiteServiceDir {
			return iofs.SkipDir
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()

		if mode.IsRegular() || mode&os.ModeSymlink != 0 {
			symlink := ""
			ok := true
			if mode&os.ModeSymlink != 0 {
				symlink, err = os.Readlink(path)
				if err != nil {
					logging.Warningf(ctx, "Can't readlink %q, skipping: %s", path, err)
					ok = false
				}
			}
			if ok {
				out = append(out, pkg.FileInfo{
					Name:       filepath.ToSlash(rel),
					Size:       uint64(info.Size()),
					Executable: (mode.Perm() & 0111) != 0,
					Symlink:    symlink,
				})
			}
		}
		return nil
	})
	return out, err
}

// removeEmptyTrees recursively removes empty directory subtrees after some
// files have been removed.
//
// It tries to avoid enumerating entire directory tree and instead recurses
// only into directories with potentially empty subtrees. They are indicated by
// 'empty' set with absolute paths to directories that had files removed from
// them (so they MAY be empty now, but not necessarily).
//
// All paths are absolute, using native separators.
//
// Best effort, logs errors.
func removeEmptyTrees(ctx context.Context, root string, empty stringset.Set) {
	// If directory 'A/B/C' has potentially empty subtree, then so do 'A/B' and
	// 'A' and '.'. Expand 'empty' set according to these rules. Note that 'root'
	// itself is always is this set.
	verboseEmpty := stringset.New(empty.Len())
	verboseEmpty.Add(root)
	empty.Iter(func(dir string) bool {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			// Note: this should never really happen, since there are checks outside
			// of this function.
			logging.Warningf(ctx, "Can't compute %q relative to %q: %s", dir, root, err)
			return true
		}

		// Here 'rel' has form 'A/B/C' or is '.' (but this is already handled).
		if rel != "." {
			for i, r := range rel {
				if r == filepath.Separator && i > 0 {
					verboseEmpty.Add(filepath.Join(root, rel[:i]))
				}
			}
			verboseEmpty.Add(filepath.Join(root, rel))
		}

		return true
	})

	// Now we recursively walk through the root subtree, skipping trees we know
	// can't be empty.
	_, err := removeEmptyTree(root, func(candidate string) (shouldCheck bool) {
		return verboseEmpty.Has(candidate)
	})
	if err != nil {
		logging.Warningf(ctx, "Failed to cleanup empty directories under %q: %s", root, err)
	}
}

// removeEmptyTree recursively removes an empty directory tree.
//
// 'path' must point to a directory (not a regular file, not a symlink).
//
// Returns true if deleted 'path' along with its (empty) subtree. Stops on first
// encountered error.
func removeEmptyTree(path string, shouldCheck func(string) bool) (deleted bool, err error) {
	if !shouldCheck(path) {
		return false, nil
	}

	// 'Remove' will delete the directory if it is already empty.
	if err := os.Remove(path); err == nil || os.IsNotExist(err) {
		return true, nil
	}

	// Otherwise need to recurse into it.
	fd, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // someone deleted it already, this is OK
		}
		return false, err
	}

	closed := false
	defer func() {
		if !closed {
			fd.Close()
		}
	}()

	total := 0
	removed := 0
	for {
		infos, err := fd.Readdir(100)
		if err == io.EOF || len(infos) == 0 {
			break
		}
		if err != nil {
			return false, err
		}
		total += len(infos)
		for _, info := range infos {
			if info.IsDir() {
				abs := filepath.Join(path, info.Name())
				switch rmed, err := removeEmptyTree(abs, shouldCheck); {
				case err != nil:
					return false, err
				case rmed:
					removed++
				}
			}
		}
	}

	// Close directory, because windows won't remove opened directory.
	fd.Close()
	closed = true

	// The directory is definitely not empty, since we skipped some stuff.
	if total != removed {
		return false, nil
	}

	// The directory is most likely empty now, unless someone concurrently put
	// files there. Unfortunately it is not trivial to detect this specific
	// condition in a cross-platform way. So assume Remove() errors (other than
	// IsNotExit) are due to that.
	err = os.Remove(path)
	return err == nil || os.IsNotExist(err), nil
}
