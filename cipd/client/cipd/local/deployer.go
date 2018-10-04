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

package local

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"
)

// TODO(vadimsh): How to handle path conflicts between two packages? Currently
// the last one installed wins.

// TODO(vadimsh): Use some sort of file lock to reduce a chance of corruption.

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
	Deployed    bool        // true if the package is deployed (perhaps partially)
	Pin         common.Pin  // the currently installed pin
	Subdir      string      // the site subdirectory where the package is installed
	Manifest    *Manifest   // instance's manifest, if available
	InstallMode InstallMode // validated install mode, if available

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

// RepairsParams is passed to RepairDeployed.
type RepairParams struct {
	// Instance holds the original package data.
	//
	// Must be present if ToRedeploy is not empty. Otherwise not used.
	Instance PackageInstance

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
	DeployInstance(ctx context.Context, subdir string, inst PackageInstance) (common.Pin, error)

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
	CheckDeployed(ctx context.Context, subdir, packageName string, paranoia common.ParanoidMode, manifest ManifestMode) (*DeployedPackage, error)

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
	RepairDeployed(ctx context.Context, subdir string, pin common.Pin, params RepairParams) error

	// TempFile returns os.File located in <base>/.cipd/tmp/*.
	//
	// The file is open for reading and writing.
	TempFile(ctx context.Context, prefix string) (*os.File, error)

	// CleanupTrash attempts to remove stale files.
	//
	// This is a best effort operation. Errors are logged (either at Debug or
	// Warning level, depending on severity of the trash state).
	CleanupTrash(ctx context.Context)
}

// NewDeployer return default Deployer implementation.
func NewDeployer(root string) Deployer {
	var err error
	if root == "" {
		err = fmt.Errorf("site root path is not provided")
	} else {
		root, err = filepath.Abs(filepath.Clean(root))
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

func (d errDeployer) DeployInstance(context.Context, string, PackageInstance) (common.Pin, error) {
	return common.Pin{}, d.err
}

func (d errDeployer) CheckDeployed(context.Context, string, string, common.ParanoidMode, ManifestMode) (*DeployedPackage, error) {
	return nil, d.err
}

func (d errDeployer) FindDeployed(context.Context) (out common.PinSliceBySubdir, err error) {
	return nil, d.err
}

func (d errDeployer) RemoveDeployed(context.Context, string, string) error { return d.err }

func (d errDeployer) RepairDeployed(context.Context, string, common.Pin, RepairParams) error {
	return d.err
}

func (d errDeployer) TempFile(context.Context, string) (*os.File, error) { return nil, d.err }

func (d errDeployer) CleanupTrash(context.Context) {}

////////////////////////////////////////////////////////////////////////////////
// Real deployer implementation.

// packagesDir is a subdirectory of site root to extract packages to.
const packagesDir = fs.SiteServiceDir + "/pkgs"

// currentSymlink is a name of a symlink that points to latest deployed version.
// Used on Linux and Mac.
const currentSymlink = "_current"

// currentTxt is a name of a text file with instance ID of latest deployed
// version. Used on Windows.
const currentTxt = "_current.txt"

// deployerImpl implements Deployer interface.
type deployerImpl struct {
	fs fs.FileSystem
}

func (d *deployerImpl) DeployInstance(ctx context.Context, subdir string, inst PackageInstance) (common.Pin, error) {
	if err := common.ValidateSubdir(subdir); err != nil {
		return common.Pin{}, err
	}

	pin := inst.Pin()
	logging.Infof(ctx, "Deploying %s into %s(/%s)", pin, d.fs.Root(), subdir)

	// Be paranoid (but not too much).
	if err := common.ValidatePin(pin, common.AnyHash); err != nil {
		return common.Pin{}, err
	}
	if _, err := d.fs.EnsureDirectory(ctx, filepath.Join(d.fs.Root(), subdir)); err != nil {
		return common.Pin{}, err
	}

	// Extract new version to the .cipd/pkgs/* guts. For "symlink" install mode it
	// is the final destination. For "copy" install mode it's a temp destination
	// and files will be moved to the site root later (in addToSiteRoot call).
	// ExtractFilesTxn knows how to build full paths and how to atomically extract
	// a package. No need to delete garbage if it fails.
	pkgPath, err := d.packagePath(ctx, subdir, pin.PackageName, true)
	if err != nil {
		return common.Pin{}, err
	}

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
	if err := ExtractFilesTxn(ctx, files, fs.NewDestination(destPath, d.fs), WithManifest); err != nil {
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
	installMode, err := checkInstallMode(newManifest.InstallMode)
	if err != nil {
		return common.Pin{}, err
	}

	// Remember currently deployed version (to remove it later). Do not freak out
	// if it's not there (prevInstanceID == "") or broken (err != nil).
	prevInstanceID, err := d.getCurrentInstanceID(pkgPath)
	prevManifest := Manifest{}
	if err == nil && prevInstanceID != "" {
		prevManifest, err = d.readManifest(ctx, filepath.Join(pkgPath, prevInstanceID))
	}
	if err != nil {
		logging.Warningf(ctx, "Previous version of the package is broken: %s", err)
		prevManifest = Manifest{} // to make sure prevManifest.Files == nil.
	}

	// Install all new files to the site root.
	err = d.addToSiteRoot(ctx, subdir, newManifest.Files, installMode, pkgPath, destPath)
	if err != nil {
		return common.Pin{}, err
	}

	// Mark installed instance as a current one. After this call the package is
	// considered installed and the function must not fail. All cleanup below is
	// best effort.
	if err = d.setCurrentInstanceID(ctx, pkgPath, pin.InstanceID); err != nil {
		return common.Pin{}, err
	}
	deleteFailedInstall = false

	// Wait for async cleanup to finish.
	wg := sync.WaitGroup{}
	defer wg.Wait()

	// When using 'copy' install mode all files (except .cipdpkg/*) are moved away
	// from 'destPath', leaving only an empty husk with directory structure.
	// Remove it to save some inodes.
	if installMode == InstallModeCopy {
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
			toKeep := map[string]bool{}
			for _, f := range newManifest.Files {
				toKeep[f.Name] = true
			}
			toKill := []FileInfo{}
			for _, f := range prevManifest.Files {
				if !toKeep[f.Name] {
					toKill = append(toKill, f)
				}
			}
			d.removeFromSiteRoot(ctx, subdir, toKill)
		}()
	}

	// Verify it's all right.
	state, err := d.CheckDeployed(ctx, subdir, pin.PackageName, common.NotParanoid, WithoutManifest)
	switch {
	case err != nil:
		logging.Errorf(ctx, "Failed to deploy %s: %s", pin, err)
		return common.Pin{}, err
	case !state.Deployed: // should not happen really...
		logging.Errorf(ctx, "Failed to deploy %s: the package is reported as not installed", pin)
		return common.Pin{}, fmt.Errorf("unknown error when deploying, see logs")
	case state.Pin.InstanceID != pin.InstanceID:
		err = fmt.Errorf("other instance (%s) was deployed concurrently", state.Pin.InstanceID)
		logging.Errorf(ctx, "Failed to deploy %s: %s", pin, err)
		return state.Pin, err
	default:
		return pin, nil
	}
}

func (d *deployerImpl) CheckDeployed(ctx context.Context, subdir, pkg string, par common.ParanoidMode, m ManifestMode) (out *DeployedPackage, err error) {
	if err = common.ValidateSubdir(subdir); err != nil {
		return
	}
	if err = common.ValidatePackageName(pkg); err != nil {
		return
	}
	if err = par.Validate(); err != nil {
		return
	}

	out = &DeployedPackage{Subdir: subdir}

	switch out.packagePath, err = d.packagePath(ctx, subdir, pkg, false); {
	case err != nil:
		out = nil
		return // this error is fatal, reinstalling probably won't help
	case out.packagePath == "":
		return // not fully deployed
	}

	current, err := d.getCurrentInstanceID(out.packagePath)
	switch {
	case err != nil:
		logging.Errorf(ctx, "Failed to figure out installed instance ID of %q in %q: %s", pkg, subdir, err)
		err = nil // this error MAY be recovered from by reinstalling
		return
	case current == "":
		return // not deployed
	default:
		out.instancePath = filepath.Join(out.packagePath, current)
	}

	// Read the manifest if asked or if doing any paranoid checks (they need the
	// list of files from the manifest).
	if m || par != common.NotParanoid {
		manifest, err := d.readManifest(ctx, out.instancePath)
		if err != nil {
			logging.Errorf(ctx, "Failed to read the manifest of %s: %s", pkg, err)
			return out, nil // this error MAY be recovered from by reinstalling
		}
		out.Manifest = &manifest
		out.InstallMode, err = checkInstallMode(manifest.InstallMode)
		if err != nil {
			return nil, err // this is fatal, the manifest has unrecognized mode
		}
	}

	// Yay, found something in a pretty healthy state.
	out.Deployed = true
	out.Pin = common.Pin{PackageName: pkg, InstanceID: current}

	// checkIntegrity needs DeployedPackage with Pin set, so call it last.
	if par != common.NotParanoid {
		out.ToRedeploy, out.ToRelink = d.checkIntegrity(ctx, out)
	}

	return
}

func (d *deployerImpl) FindDeployed(ctx context.Context) (common.PinSliceBySubdir, error) {
	// Directories with packages are direct children of .cipd/pkgs/.
	pkgs := filepath.Join(d.fs.Root(), filepath.FromSlash(packagesDir))
	infos, err := ioutil.ReadDir(pkgs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	found := common.PinMapBySubdir{}
	for _, info := range infos {
		if !info.IsDir() {
			continue
		}
		// Read the description and the 'current' link.
		pkgPath := filepath.Join(pkgs, info.Name())
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

	deployed, err := d.CheckDeployed(ctx, subdir, packageName, common.NotParanoid, WithManifest)
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
		d.removeFromSiteRoot(ctx, subdir, deployed.Manifest.Files)
	default:
		// The package was partially installed in the guts, but not into the site
		// root. We can just remove the guts thus forgetting about the package.
		logging.Warningf(ctx, "Package %s is partially installed, removing it", packageName)
	}
	return d.fs.EnsureDirectoryGone(ctx, deployed.packagePath)
}

func (d *deployerImpl) RepairDeployed(ctx context.Context, subdir string, pin common.Pin, params RepairParams) error {
	switch {
	case len(params.ToRedeploy) != 0 && params.Instance == nil:
		panic("if ToRedeploy is not empty, Instance must be given too")
	case params.Instance != nil && params.Instance.Pin() != pin:
		panic(fmt.Sprintf("expecting instance with pin %s, got %s", pin, params.Instance.Pin()))
	}

	// Note that we can slightly optimize the repairs of packages in 'copy'
	// install mode by extracting directly into the site root. But this
	// complicates the code and introduces a "unique" code path, not exercised by
	// DeployInstance. Instead we prefer a bit slower code that resembles
	// DeployInstance in its logic: it extracts everything into the instance
	// directory in the guts, and then symlinks/moves it to the site root.

	// Check that the package we are repairing is still installed and grab its
	// manifest (for the install mode and list of files).
	pkg, err := d.CheckDeployed(ctx, subdir, pin.PackageName, common.NotParanoid, WithManifest)
	switch {
	case err != nil:
		return err
	case !pkg.Deployed:
		return fmt.Errorf("the package %s is not deployed any more, refusing to recover", pin.PackageName)
	case pkg.Pin != pin:
		return fmt.Errorf("expected to find pin %s, got %s, refusing to recover", pin, pkg.Pin)
	case pkg.packagePath == "":
		panic("impossible, packagePath cannot be empty for deployed pkg")
	case pkg.instancePath == "":
		panic("impossible, instancePath cannot be empty for deployed pkg")
	}

	// manifest contains all files that should be present in a healthy deployment.
	manifest := make(map[string]FileInfo, len(pkg.Manifest.Files))
	for _, f := range pkg.Manifest.Files {
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
		// Without PackageInstance present we can repair only symlinks based on info
		// in the manifest.
		repairableFiles = map[string]fs.File{}
		for _, f := range pkg.Manifest.Files {
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
		if err := ExtractFiles(ctx, repair, fs.ExistingDestination(pkg.instancePath, d.fs), WithoutManifest); err != nil {
			return err
		}
	}

	// Finally relink/move everything into the site root.
	infos := make([]FileInfo, 0, len(params.ToRedeploy)+len(params.ToRelink))
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
	if err := d.addToSiteRoot(ctx, pkg.Subdir, infos, pkg.InstallMode, pkg.packagePath, pkg.instancePath); err != nil {
		return err
	}

	// Cleanup empty directories left in the guts after files have been moved
	// away, just like DeployInstance does. Best effort.
	if pkg.InstallMode == InstallModeCopy {
		removeEmptyTree(pkg.instancePath, func(string) bool { return true })
	}

	if failed {
		return fmt.Errorf("repair of %s failed, see logs", pkg.Pin.PackageName)
	}
	return nil
}

func (d *deployerImpl) TempFile(ctx context.Context, prefix string) (*os.File, error) {
	dir, err := d.fs.EnsureDirectory(ctx, filepath.Join(d.fs.Root(), fs.SiteServiceDir, "tmp"))
	if err != nil {
		return nil, err
	}
	return ioutil.TempFile(dir, prefix)
}

func (d *deployerImpl) TempDir(ctx context.Context, prefix string, mode os.FileMode) (string, error) {
	dir, err := d.fs.EnsureDirectory(ctx, filepath.Join(d.fs.Root(), fs.SiteServiceDir, "tmp"))
	if err != nil {
		return "", err
	}
	return fs.TempDir(dir, prefix, mode)
}

func (d *deployerImpl) CleanupTrash(ctx context.Context) {
	d.fs.CleanupTrash(ctx)
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
func (f *symlinkFile) Open() (io.ReadCloser, error)   { return nil, fmt.Errorf("can't open a symlink") }

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
		logging.Errorf(ctx, "Can't get absolute path of %q: %s", rel, err)
		return "", err
	}

	seenNumbers, curPkgs := d.resolveValidPackageDirs(ctx, abs)
	if cur, ok := curPkgs[Description{subdir, pkg}]; ok {
		return cur, nil
	}

	if !allocate {
		return "", nil
	}

	// we didn't find one, so we have to make one
	if _, err := d.fs.EnsureDirectory(ctx, abs); err != nil {
		logging.Errorf(ctx, "Cannot ensure packages directory: %s", err)
		return "", err
	}

	// take the last 2 components from the pkg path.
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
		logging.Errorf(ctx, "Cannot create new pkg tempdir: %s", err)
		return "", err
	}
	defer d.fs.EnsureDirectoryGone(ctx, tmpDir)
	err = d.fs.EnsureFile(ctx, filepath.Join(tmpDir, descriptionName), func(f *os.File) error {
		return writeDescription(&Description{Subdir: subdir, PackageName: pkg}, f)
	})
	if err != nil {
		logging.Errorf(ctx, "Cannot create new pkg description.json: %s", err)
		return "", err
	}

	// now we have to find a suitable index folder for it.
	for attempts := 0; attempts < 3; attempts++ {
		if attempts > 0 {
			// random sleep up to 1s to help avoid collisions between clients.
			time.Sleep(time.Duration(rand.Int31n(1000)) * time.Millisecond)
		}
		n := seenNumbers.smallestNewNum()
		seenNumbers.addNum(n)

		pkgPath := filepath.Join(abs, strconv.Itoa(n))
		// We use os.Rename instead of d.fs.Replace because we want it to fail if
		// the target directory already exists.
		switch err := os.Rename(tmpDir, pkgPath); le := err.(type) {
		case nil:
			return pkgPath, nil

		case *os.LinkError:
			if le.Err != syscall.ENOTEMPTY {
				logging.Errorf(ctx, "Error while creating pkg dir %s: %s", pkgPath, err)
				return "", err
			}

		default:
			logging.Errorf(ctx, "Unknown error while creating pkg dir %s: %s", pkgPath, err)
			return "", err
		}

		// rename failed with ENOTEMPTY, that means that another client wrote this
		// directory.
		description, err := d.readDescription(ctx, pkgPath)
		if err != nil {
			logging.Warningf(ctx, "Skipping %q: %s", pkgPath, err)
			continue
		}
		if description.PackageName == pkg && description.Subdir == subdir {
			return pkgPath, nil
		}
	}

	logging.Errorf(ctx, "Unable to find valid index for package %q in %s!", pkg, abs)
	return "", err
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
//   * a numeric set of all number-style directories seen.
//   * a map of Description (e.g. subdir + pkgname) to the correct pkg folder
//
// This also will delete (EnsureDirectoryGone) any folders or files in the pkgs
// directory which are:
//   * invalid (contain no description.json and no current instance symlink)
//   * duplicate (where multiple directories contain the same description.json)
//
// Duplicate detection always prefers the folder with the shortest path name
// that sorts alphabetically earlier.
func (d *deployerImpl) resolveValidPackageDirs(ctx context.Context, pkgsAbsDir string) (numbered numSet, all map[Description]string) {
	files, err := ioutil.ReadDir(pkgsAbsDir)
	if err != nil && !os.IsNotExist(err) {
		logging.Errorf(ctx, "Can't read packages dir %q: %s", pkgsAbsDir, err)
		return
	}

	allWithDups := map[Description][]string{}

	for _, f := range files {
		fullPkgPath := filepath.Join(pkgsAbsDir, f.Name())
		description, err := d.readDescription(ctx, fullPkgPath)
		if description == nil || err != nil {
			if err == nil {
				err = fmt.Errorf("missing description.json and current instance")
			}
			logging.Warningf(ctx, "removing junk directory: %q (%s)", fullPkgPath, err)
			if err := d.fs.EnsureDirectoryGone(ctx, fullPkgPath); err != nil {
				logging.Warningf(ctx, "while removing junk directory: %q (%s)", fullPkgPath, err)
			}
			continue
		}
		allWithDups[*description] = append(allWithDups[*description], fullPkgPath)
	}

	all = make(map[Description]string, len(allWithDups))
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
		bytes, err = ioutil.ReadFile(filepath.Join(packageDir, currentTxt))
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
		return "", err
	}
	if err = common.ValidateInstanceID(current, common.AnyHash); err != nil {
		return "", fmt.Errorf(
			"pointer to currently installed instance doesn't look like a valid instance id: %s", err)
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
	if runtime.GOOS == "windows" {
		return fs.EnsureFile(
			ctx, d.fs, filepath.Join(packageDir, currentTxt),
			strings.NewReader(instanceID))
	}
	return d.fs.EnsureSymlink(ctx, filepath.Join(packageDir, currentSymlink), instanceID)
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
func (d *deployerImpl) readDescription(ctx context.Context, pkgDir string) (desc *Description, err error) {
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
		return
	}

	// see if this is a pre 1.4 directory
	currentID, err := d.getCurrentInstanceID(pkgDir)
	if err != nil {
		return
	}

	if currentID == "" {
		logging.Warningf(ctx, "No current instance id in %s", pkgDir)
		err = nil
		return
	}

	manifest, err := d.readManifest(ctx, filepath.Join(pkgDir, currentID))
	if err != nil {
		return
	}

	desc = &Description{
		PackageName: manifest.PackageName,
	}
	// To handle the case where some other user owns these directories, all errors
	// from here to the end are treated as warnings.
	err = d.fs.EnsureFile(ctx, descriptionPath, func(f *os.File) error {
		return writeDescription(desc, f)
	})
	if err != nil {
		logging.Warningf(ctx, "Unable to create description.json: %s", err)
		err = nil
	}
	return
}

// readManifest reads package manifest given a path to a package instance
// (.cipd/pkgs/<name>/<instance id>).
func (d *deployerImpl) readManifest(ctx context.Context, instanceDir string) (Manifest, error) {
	manifestPath := filepath.Join(instanceDir, filepath.FromSlash(ManifestName))
	r, err := os.Open(manifestPath)
	if err != nil {
		return Manifest{}, err
	}
	defer r.Close()
	manifest, err := readManifest(r)
	if err != nil {
		return Manifest{}, err
	}
	// Older packages do not have Files section in the manifest, so reconstruct it
	// from actual files on disk.
	if len(manifest.Files) == 0 {
		if manifest.Files, err = scanPackageDir(ctx, instanceDir); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

// addToSiteRoot moves or symlinks files into the site root directory (depending
// on passed installMode).
func (d *deployerImpl) addToSiteRoot(ctx context.Context, subdir string, files []FileInfo, installMode InstallMode, pkgDir, srcDir string) error {
	for _, f := range files {
		// e.g. bin/tool
		relPath := filepath.FromSlash(f.Name)
		// e.g. <base>/<subdir>/bin/tool
		destAbs, err := d.fs.RootRelToAbs(filepath.Join(subdir, relPath))
		if err != nil {
			logging.Warningf(ctx, "Invalid relative path %q: %s", relPath, err)
			return err
		}
		if installMode == InstallModeSymlink {
			// e.g. <base>/.cipd/pkgs/name/_current/bin/tool
			targetAbs := filepath.Join(pkgDir, currentSymlink, relPath)
			// e.g. ../.cipd/pkgs/name/_current/bin/tool
			// has more `../` depending on subdir
			targetRel, err := filepath.Rel(filepath.Dir(destAbs), targetAbs)
			if err != nil {
				logging.Warningf(
					ctx, "Can't get relative path from %s to %s",
					filepath.Dir(destAbs), targetAbs)
				return err
			}
			if err = d.fs.EnsureSymlink(ctx, destAbs, targetRel); err != nil {
				logging.Warningf(ctx, "Failed to create symlink for %s", relPath)
				return err
			}
		} else if installMode == InstallModeCopy {
			// E.g. <base>/.cipd/pkgs/name/<id>/bin/tool.
			srcAbs := filepath.Join(srcDir, relPath)
			if err := d.fs.Replace(ctx, srcAbs, destAbs); err != nil {
				logging.Warningf(ctx, "Failed to move %s to %s: %s", srcAbs, destAbs, err)
				return err
			}
		} else {
			// Should not happen. ValidateInstallMode checks this.
			return fmt.Errorf("impossible state")
		}
	}
	return nil
}

// removeFromSiteRoot deletes files from the site root directory.
//
// Best effort. Logs errors and carries on.
func (d *deployerImpl) removeFromSiteRoot(ctx context.Context, subdir string, files []FileInfo) {
	dirsToCleanup := stringset.New(0)

	for _, f := range files {
		absPath, err := d.fs.RootRelToAbs(filepath.Join(subdir, filepath.FromSlash(f.Name)))
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
func (d *deployerImpl) isPresentInSite(ctx context.Context, subdir string, f FileInfo, followSymlinks bool) bool {
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
func (d *deployerImpl) isPresentInGuts(ctx context.Context, instDir string, f FileInfo) bool {
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
func (d *deployerImpl) checkIntegrity(ctx context.Context, pkg *DeployedPackage) (redeploy, relink []string) {
	logging.Debugf(ctx, "Checking integrity of %q deployment...", pkg.Pin.PackageName)

	// Examine files that are supposed to be installed.
	for _, f := range pkg.Manifest.Files {
		switch {
		case d.isPresentInSite(ctx, pkg.Subdir, f, true): // no need to repair
			continue
		case f.Symlink == "": // a regular file (not a symlink)
			switch {
			case pkg.InstallMode == InstallModeCopy:
				// In 'copy' mode regular files are stored in the site root directly.
				// If they are gone, we need to refetch the package to restore them.
				redeploy = append(redeploy, f.Name)
			case d.isPresentInGuts(ctx, pkg.instancePath, f):
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
			case !d.isPresentInSite(ctx, pkg.Subdir, f, false):
				// The symlink in the site root is gone, need to restore it.
				relink = append(relink, f.Name)
			case pkg.InstallMode == InstallModeSymlink && !d.isPresentInGuts(ctx, pkg.instancePath, f):
				// The symlink in the guts is gone, need to restore it.
				relink = append(relink, f.Name)
			default:
				// Both CIPD-managed symlinks are fine, nothing to restore.
			}
		}
	}

	return
}

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

// checkInstallMode validates the install mode and picks the correct default
// if no install mode is given.
func checkInstallMode(im InstallMode) (InstallMode, error) {
	switch {
	case runtime.GOOS == "windows":
		return InstallModeCopy, nil // Windows supports only 'copy' mode
	case im == "":
		return InstallModeSymlink, nil // default on other platforms
	}
	if err := ValidateInstallMode(im); err != nil {
		return "", err
	}
	return im, nil
}

// scanPackageDir finds a set of regular files (and symlinks) in a package
// instance directory and returns them as FileInfo structs (with slash-separated
// paths relative to dir directory). Skips package service directories (.cipdpkg
// and .cipd) since they contain package deployer gut files, not something that
// needs to be deployed.
func scanPackageDir(ctx context.Context, dir string) ([]FileInfo, error) {
	out := []FileInfo{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == PackageServiceDir || rel == fs.SiteServiceDir {
			return filepath.SkipDir
		}
		if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			symlink := ""
			ok := true
			if info.Mode()&os.ModeSymlink != 0 {
				symlink, err = os.Readlink(path)
				if err != nil {
					logging.Warningf(ctx, "Can't readlink %q, skipping: %s", path, err)
					ok = false
				}
			}
			if ok {
				out = append(out, FileInfo{
					Name:       filepath.ToSlash(rel),
					Size:       uint64(info.Size()),
					Executable: (info.Mode().Perm() & 0111) != 0,
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
			logging.Warningf(ctx, "Can't compute %q relative to %q - %s", dir, root, err)
			return true
		}

		// Here 'rel' has form 'A/B/C' or is '.' (but this is already handled).
		if rel != "." {
			path := root
			for _, chunk := range strings.Split(rel, string(filepath.Separator)) {
				path = filepath.Join(path, chunk)
				verboseEmpty.Add(path)
			}
		}

		return true
	})

	// Now we recursively walk through the root subtree, skipping trees we know
	// can't be empty.
	_, err := removeEmptyTree(root, func(candidate string) (shouldCheck bool) {
		return verboseEmpty.Has(candidate)
	})
	if err != nil {
		logging.Warningf(ctx, "Failed to cleanup empty directories under %q - %s", root, err)
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
