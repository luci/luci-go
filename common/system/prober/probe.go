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

// Package prober exports Probe, which implements logic to identify a wrapper's
// wrapped target. In addition to basic PATH/filename lookup, Prober contains
// logic to ensure that the wrapper is not the same software as the current
// running instance, and enables optional hard-coded wrap target paths and
// runtime checks.
package prober

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
)

// CheckWrapperFunc is an optional function that can be implemented for a
// Prober to check if a candidate path is a wrapper.
type CheckWrapperFunc func(c context.Context, path string, env environ.Env) (isWrapper bool, err error)

// Probe can Locate a Target executable by probing the local system PATH.
//
// Target should be an executable name resolvable by exec.LookPath. On
// Windows systems, this may omit the executable extension (e.g., "bat", "exe")
// since that is augmented via the PATHEXT environment variable (see
// "probe_windows.go").
type Probe struct {
	// Target is the name of the target (as seen by exec.LookPath) that we are
	// searching for.
	Target string

	// RelativePathOverride is a series of forward-slash-delimited paths to
	// directories relative to the wrapper executable that will be checked
	// prior to checking PATH. This allows bundles (e.g., CIPD) that include both
	// the wrapper and a real implementation, to force the wrapper to use
	// the bundled implementation.
	RelativePathOverride []string

	// CheckWrapper, if not nil, is a function called on a candidate wrapper to
	// determine whether or not that candidate is valid.
	//
	// On success, it will return isWrapper, which will be true if path is a
	// wrapper instance and false if it is not. If an error occurred during
	// checking, the error should be returned and isWrapper will be ignored. If
	// a candidate is a wrapper, or if an error occurred during check, the
	// candidate will be discarded and the probe will continue.
	//
	// CheckWrapper should be lightweight and fast, as it may be called multiple
	// times.
	CheckWrapper CheckWrapperFunc

	// Self and Selfstat are resolved contain the path and FileInfo of the
	// currently running executable, respectively. They can both be resolved via
	// ResolveSelf, and may be empty if resolution has not been performed, or if
	// the current executable could not be resolved. They may also be set
	// explicitly, bypassing the need to perform resolution.
	Self     string
	SelfStat os.FileInfo

	// PathDirs, if not zero, contains the list of directories to search. If
	// zero, the os.PathListSeparator-delimited PATH environment variable will
	// be used.
	PathDirs []string
}

// ResolveSelf attempts to identify the current process. If successful, p's
// Self will be set to an absolute path reference to Self, and its SelfStat
// field will be set to the os.FileInfo for that path.
//
// If this process was invoked via symlink, the path to the symlink will be
// returned if possible.
func (p *Probe) ResolveSelf(argv0 string) error {
	if p.Self != "" {
		return nil
	}

	// Get the authoritative executable from the system.
	exec, err := os.Executable()
	if err != nil {
		return errors.Annotate(err, "failed to get executable").Err()
	}

	execStat, err := os.Stat(exec)
	if err != nil {
		return errors.Annotate(err, "failed to stat executable: %s", exec).Err()
	}

	// Before using "os.Executable" result, which is known to resolve symlinks on
	// Linux, try and identify via argv0.
	if argv0 != "" && filesystem.AbsPath(&argv0) == nil {
		if st, err := os.Stat(argv0); err == nil && os.SameFile(execStat, st) {
			// argv[0] is the same file as our executable, but may be an unresolved
			// symlink. Prefer it.
			p.Self, p.SelfStat = argv0, st
			return nil
		}
	}

	p.Self, p.SelfStat = exec, execStat
	return nil
}

// Locate attempts to locate the system's Target by traversing the available
// PATH.
//
// cached is the cached path, passed from wrapper to wrapper through the a
// State struct in the environment. This may be empty, if there was no cached
// path or if the cached path was invalid.
//
// env is the environment to operate with, and will not be modified during
// execution.
func (p *Probe) Locate(c context.Context, cached string, env environ.Env) (string, error) {
	// If we have a cached path, check that it exists and is executable and use it
	// if it is.
	if cached != "" {
		switch cachedStat, err := os.Stat(cached); {
		case err == nil:
			// Use the cached path. First, pass it through a sanity check to ensure
			// that it is not self.
			if p.SelfStat == nil || !os.SameFile(p.SelfStat, cachedStat) {
				logging.Debugf(c, "Using cached value: %s", cached)
				return cached, nil
			}
			logging.Debugf(c, "Cached value [%s] is this wrapper [%s]; ignoring.", cached, p.Self)

		case os.IsNotExist(err):
			// Our cached path doesn't exist, so we will have to look for a new one.

		case err != nil:
			// We couldn't check our cached path, so we will have to look for a new
			// one. This is an unexpected error, though, so emit it.
			logging.Debugf(c, "Failed to stat cached [%s]: %s", cached, err)
		}
	}

	// Get stats on our parent directory. This may fail; if so, we'll skip the
	// SameFile check.
	var selfDir string
	var selfDirStat os.FileInfo
	if p.Self != "" {
		selfDir = filepath.Dir(p.Self)

		var err error
		if selfDirStat, err = os.Stat(selfDir); err != nil {
			logging.Debugf(c, "Failed to stat self directory [%s]: %s", selfDir, err)
		}
	}

	// Walk through PATH. Our goal is to find the first program named Target that
	// isn't self and doesn't identify as a wrapper.
	pathDirs := p.PathDirs
	if pathDirs == nil {
		pathDirs = strings.Split(env.GetEmpty("PATH"), string(os.PathListSeparator))
	}

	// Build our list of directories to check for Target.
	checkDirs := make([]string, 0, len(pathDirs)+len(p.RelativePathOverride))
	if selfDir != "" {
		for _, rpo := range p.RelativePathOverride {
			checkDirs = append(checkDirs, filepath.Join(selfDir, filepath.FromSlash(rpo)))
		}
	}
	checkDirs = append(checkDirs, pathDirs...)

	// Iterate through each check directory and look for a Target candidate within
	// it.
	checked := make(map[string]struct{}, len(checkDirs))
	for _, dir := range checkDirs {
		if _, ok := checked[dir]; ok {
			continue
		}
		checked[dir] = struct{}{}

		path := p.checkDir(c, dir, selfDirStat, env)
		if path != "" {
			return path, nil
		}
	}

	return "", errors.Reason("could not find target in system").
		InternalReason("target(%s)/dirs(%v)", p.Target, pathDirs).Err()
}

// checkDir checks "checkDir" for our Target executable. It ignores
// executables whose target is the same file or shares the same parent directory
// as "self".
func (p *Probe) checkDir(c context.Context, dir string, selfDir os.FileInfo, env environ.Env) string {
	// If we have a self directory defined, ensure that "dir" isn't the same
	// directory. If it is, we will ignore this option, since we are looking for
	// something outside of the wrapper directory.
	if selfDir != nil {
		switch checkDirStat, err := os.Stat(dir); {
		case err == nil:
			// "dir" exists; if it is the same as "selfDir", we can ignore it.
			if os.SameFile(selfDir, checkDirStat) {
				logging.Debugf(c, "Candidate shares wrapper directory [%s]; skipping...", dir)
				return ""
			}

		case os.IsNotExist(err):
			logging.Debugf(c, "Candidate directory does not exist [%s]; skipping...", dir)
			return ""

		default:
			logging.Debugf(c, "Failed to stat candidate directory [%s]: %s", dir, err)
			return ""
		}
	}

	t, err := findInDir(p.Target, dir, env)
	if err != nil {
		return ""
	}

	// Make sure this file isn't the same as "self", if available.
	if p.SelfStat != nil {
		switch st, err := os.Stat(t); {
		case err == nil:
			if os.SameFile(p.SelfStat, st) {
				logging.Debugf(c, "Candidate [%s] is same file as wrapper; skipping...", t)
				return ""
			}

		case os.IsNotExist(err):
			// "t" no longer exists, so we can't use it.
			return ""

		default:
			logging.Debugf(c, "Failed to stat candidate path [%s]: %s", t, err)
			return ""
		}
	}

	if err := filesystem.AbsPath(&t); err != nil {
		logging.Debugf(c, "Failed to normalize candidate path [%s]: %s", t, err)
		return ""
	}

	// Try running the candidate command and confirm that it is not a wrapper.
	if p.CheckWrapper != nil {
		switch isWrapper, err := p.CheckWrapper(c, t, env); {
		case err != nil:
			logging.Debugf(c, "Failed to check if [%s] is a wrapper: %s", t, err)
			return ""

		case isWrapper:
			logging.Debugf(c, "Candidate is a wrapper: %s", t)
			return ""
		}
	}

	return t
}
