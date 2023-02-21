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

package python

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/vpython/api/vpython"
)

// Interpreter represents a system Python interpreter. It exposes the ability
// to use common functionality of that interpreter.
type Interpreter struct {
	// Python is the path to the system Python interpreter.
	Python string

	// cachedHash is the hash key of the Python binary used for on disk cache.
	cachedHash string
	// cachedVersion is the cached Version for this interpreter. It is populated
	// on the first GetVersion call.
	cachedVersion *Version
	cachedMu      sync.Mutex

	// If true, disable reading cache from file.
	fileCacheDisabled bool

	// testCommandHook, if not nil, is called on generated Command results prior
	// to returning them.
	testCommandHook func(*exec.Cmd)
}

// Normalize normalizes the Interpreter configuration by resolving relative
// paths into absolute paths.
func (i *Interpreter) Normalize() error {
	return filesystem.AbsPath(&i.Python)
}

// IsolatedCommand has an *exec.Cmd, as well as the temporary directory
// created for this Cmd.
type IsolatedCommand struct {
	*exec.Cmd
	dir string
}

// Cleanup must be called after the IsolatedCommand is no longer needed.
func (iso IsolatedCommand) Cleanup() {
	if err := os.RemoveAll(iso.dir); err != nil {
		panic(errors.Annotate(err, "removing IsolatedCommand's directory").Err())
	}
}

// MkIsolatedCommand returns a configurable exec.Cmd structure bound to this
// Interpreter.
//
// The supplied arguments have several Python isolation flags prepended to them
// to remove environmental factors such as:
//   - The user's "site.py".
//   - The current PYTHONPATH environment variable.
//   - The current working directory (i.e. avoids `import foo` picking up local
//     foo.py)
//   - Compiled ".pyc/.pyo" files.
//
// The caller MUST call IsolatedCommand.Cleanup when they no longer need the
// IsolatedCommand.
func (i *Interpreter) MkIsolatedCommand(c context.Context, target Target, args ...string) IsolatedCommand {
	// Isolate the supplied arguments.
	cl := CommandLine{
		Target: target,
		Args:   args,
	}
	cl.AddSingleFlag("B") // Don't compile "pyo" binaries.
	cl.AddSingleFlag("E") // Don't use PYTHON* environment variables.
	cl.AddSingleFlag("s") // Don't use user 'site.py'.
	cmd := exec.CommandContext(c, i.Python, cl.BuildArgs()...)
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		panic(errors.Annotate(err, "creating IsolatedCommand's directory").Err())
	}
	cmd.Dir = dir
	defer func() {
	}()
	if i.testCommandHook != nil {
		i.testCommandHook(cmd)
	}
	return IsolatedCommand{cmd, dir}
}

const versionCachePrefix = "VPythonPythonVersion"

// GetVersion runs the specified Python interpreter to extract its version
// from `platform.python_version` and maps it to a known specification version.
func (i *Interpreter) GetVersion(c context.Context) (v Version, err error) {
	i.cachedMu.Lock()
	defer i.cachedMu.Unlock()

	if i.cachedVersion != nil {
		v = *i.cachedVersion
		return
	}

	if cached, errCached := i.getCached(versionCachePrefix, &v); cached {
		i.cachedVersion = &v
		return
	} else if errCached != nil {
		logging.WithError(errCached).Warningf(c, "Get cached version from file failed")
	}

	cmd := i.MkIsolatedCommand(c, CommandTarget{
		"import platform, sys; sys.stdout.write(platform.python_version())",
	})
	defer cmd.Cleanup()

	out, err := cmd.Output()
	if err != nil {
		err = errors.Annotate(err, "").Err()
		return
	}

	if v, err = ParseVersion(string(out)); err != nil {
		return
	}
	if v.IsZero() {
		err = errors.Reason("unknown version output").Err()
		return
	}

	i.cachedVersion = &v
	if err := i.setCached(versionCachePrefix, v); err != nil {
		logging.WithError(err).Warningf(c, "Set cached version to file failed")
	}
	return
}

// GetRuntime returns all runtime info to identify a python installation.
// It use the same way as CPython to determine Prefix to avoid invoke python.
func (i *Interpreter) GetRuntime(c context.Context) (r *vpython.Runtime, err error) {
	// EvalSymlinks is generally broken on windows:
	// https://github.com/golang/go/issues/40180
	//
	// Not resolving the symlink on windows is unlikely to cause any problem
	// since the lib ships with python is at the same location of python binary
	// on windows, and we have binary hash in the cache identity to differentiate
	// Python installations.
	path := i.Python
	if runtime.GOOS != "windows" {
		path, err = filepath.EvalSymlinks(i.Python)
		if err != nil {
			return
		}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return
	}

	hash, err := getHash(path)
	if err != nil {
		return
	}

	version, err := i.GetVersion(c)
	if err != nil {
		return
	}

	prefix, err := cpythonSearchForPrefix(path, version)
	if err != nil {
		err = errors.Annotate(err, "").Err()
		return
	}

	return &vpython.Runtime{
		Path:    path,
		Hash:    hash,
		Version: version.String(),
		Prefix:  prefix,
		Arch:    runtime.GOARCH,
	}, nil
}

func (i *Interpreter) GetHash() (string, error) {
	if i.cachedHash != "" {
		return i.cachedHash, nil
	}
	h, err := getHash(i.Python)
	if err != nil {
		return "", errors.Annotate(err, "Calculate hash from python binary failed").Err()
	}
	i.cachedHash = h
	return i.cachedHash, nil
}

// getCached require cachedMu to be held
func (i *Interpreter) getCached(prefix string, v any) (found bool, err error) {
	if i.fileCacheDisabled {
		return false, nil
	}
	hash, err := i.GetHash()
	if err != nil {
		return
	}
	key := filepath.Join(os.TempDir(), prefix+hash)

	c, err := os.Open(key)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}
	defer c.Close()
	if err = json.NewDecoder(c).Decode(v); err != nil {
		return
	}
	found = true
	return
}

// setCached require cachedMu to be held
func (i *Interpreter) setCached(prefix string, v any) (err error) {
	if i.fileCacheDisabled {
		return
	}
	hash, err := i.GetHash()
	if err != nil {
		return
	}
	key := filepath.Join(os.TempDir(), prefix+hash)

	temp, err := os.CreateTemp("", "")
	if err != nil {
		return
	}
	defer func() {
		if errClose := temp.Close(); err == nil {
			err = errClose
		}
	}()
	if err = json.NewEncoder(temp).Encode(v); err != nil {
		return
	}
	return os.Rename(temp.Name(), key)
}

func CheckVersionCachedExist(path string) (found bool, err error) {
	h, err := getHash(path)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(os.TempDir(), versionCachePrefix+h)); err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return false, err
	}
	return true, nil
}

func getHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := crc32.NewIEEE()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	if _, err := hash.Write([]byte(path)); err != nil {
		return "", err
	}
	// Include GOARCH as part of cache for universal binary python on mac arm.
	// We can't tell if the env was used under rosetta 2 from path and binary.
	// NOTE: Vpython is not always the same architecture as cpython (e.g. vpython
	// is arm64 while cpython is amd64), but we only care when cpython is an
	// universal binary. In which case, the architecture which an universal
	// cpython runs on is depending on the architecture of vpython.
	if _, err := hash.Write([]byte(runtime.GOARCH)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%X", hash.Sum32()), nil
}

// cpythonSearchForPrefix translate the search_for_prefix in
// cpython/Modules/getpath.c to avoid invoke python for prefix.
// The only difference is it will ignore PYTHONHOME as we are assuming that
// it's running in an isolated environment.
func cpythonSearchForPrefix(path string, version Version) (prefix string, err error) {
	buildLandmark := filepath.Join("Modules", "Setup.local")

	// Check to see if path is in the build directory
	// Path: <path> / <build_landmark>
	finfo, err := os.Stat(filepath.Join(path, buildLandmark))
	isBuildDir := (err == nil) && finfo.Mode().IsRegular()

	if isBuildDir {
		// path is the build directory (buildLandmark exists), now also
		// check landmark using cpythonIsModule().
		// Path: <path> \ Lib \ <landmark>
		// e.g., C:\python3\Lib\<landmark>
		if cpythonIsModule(filepath.Join(path, "Lib")) {
			return path, nil
		}
	}

	if runtime.GOOS == "windows" {
		parent := path
		for {
			// Path: <path or substring> / <libDir> / < landmark >
			if cpythonIsModule(filepath.Join(parent, "lib")) {
				return parent, nil
			}

			if next := filepath.Dir(parent); parent != next {
				parent = next
			} else {
				break
			}
		}
	} else {
		// It is equal to "lib" on most platforms. On Fedora and SuSE, it is equal
		// to "lib64" on 64-bit platforms.
		for _, libDir := range []string{"lib", "lib64"} {
			// Search from path, until root is found
			parent := path
			for {
				// Path: <path or substring> / <libDir> / <PythonBase> / <landmark>
				// e.g., /usr/lib/python3.9/<landmark>
				if cpythonIsModule(filepath.Join(parent, libDir, version.PythonBase())) {
					return parent, nil
				}

				if next := filepath.Dir(parent); parent != next {
					parent = next
				} else {
					break
				}
			}
		}
	}

	// Use a default prefix
	return path, nil
}

// cpythonIsModule translate the ismodule in cpython/Modules/getpath.c
// Is module -- check for .pyc too
func cpythonIsModule(prefix string) bool {
	const moduleLandmark = "os.py"
	finfo, err := os.Stat(filepath.Join(prefix, moduleLandmark))
	if (err == nil) && finfo.Mode().IsRegular() {
		return true
	}

	// Check for the compiled version of prefix.
	const compiledLandmark = moduleLandmark + "c"
	finfo, err = os.Stat(filepath.Join(prefix, compiledLandmark))
	if (err == nil) && finfo.Mode().IsRegular() {
		return true
	}

	return false
}

// IsolateEnvironment mutates e to remove any environmental influence over
// the Python interpreter.
//
// If keepPythonPath is true, PYTHONPATH will not be cleared. This is used
// by the actual VirtualEnv Python invocation to preserve PYTHONPATH since it is
// a form of user input.
//
// If e is nil, no operation will be performed.
func IsolateEnvironment(e *environ.Env, keepPythonPath bool) {
	if e == nil {
		return
	}

	// Remove PYTHONPATH if instructed.
	if !keepPythonPath {
		e.Remove("PYTHONPATH")
	}

	// Remove PYTHONHOME from the environment. PYTHONHOME is used to set the
	// location of standard Python libraries, which we make a point of overriding.
	//
	// https://docs.python.org/2/using/cmdline.html#envvar-PYTHONHOME
	e.Remove("PYTHONHOME")

	// set PYTHONNOUSERSITE, which prevents a user's "site" configuration
	// from influencing Python startup. The system "site" should already be
	// ignored b/c we're using the VirtualEnv Python interpreter.
	e.Set("PYTHONNOUSERSITE", "1")
}
