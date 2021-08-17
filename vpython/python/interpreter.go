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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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
	cachedRuntime *vpython.Runtime
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
//	- The user's "site.py".
//	- The current PYTHONPATH environment variable.
//	- The current working directory (i.e. avoids `import foo` picking up local
//	  foo.py)
//	- Compiled ".pyc/.pyo" files.
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

// GetRuntime runs the specified Python interpreter to extract its path and
// prefix. It's safe to cache prefix as we run the script in isolated
// environment. In such case prefix will be determined by build parament or
// backtracking up the path.
func (i *Interpreter) GetRuntime(c context.Context) (r vpython.Runtime, err error) {
	const cachePrefix = "VPythonPythonPrefix"
	const script = `` +
		`import json, sys;` +
		`json.dump({` +
		`'path': sys.executable,` +
		`'prefix': sys.prefix,` +
		`}, sys.stdout)`

	i.cachedMu.Lock()
	defer i.cachedMu.Unlock()

	if i.cachedRuntime != nil {
		r = vpython.Runtime{
			Path:   i.cachedRuntime.GetPath(),
			Prefix: i.cachedRuntime.GetPrefix(),
		}
		return
	}

	if cached, errCached := i.getCached(cachePrefix, &r); cached {
		i.cachedRuntime = &r
		return
	} else if errCached != nil {
		logging.WithError(errCached).Warningf(c, "Get cached runtime from file failed")
	}

	// Probe the runtime information.
	var stdout, stderr bytes.Buffer
	cmd := i.MkIsolatedCommand(c, CommandTarget{Command: script})
	defer cmd.Cleanup()

	cmd.Stdout = &stdout
	if err = cmd.Run(); err != nil {
		logging.WithError(err).Errorf(c, "Failed to get runtime information:\n%s", stderr.Bytes())
		err = errors.Annotate(err, "failed to get runtime information").Err()
		return
	}

	if err = json.Unmarshal(stdout.Bytes(), &r); err != nil {
		err = errors.Annotate(err, "could not unmarshal output: %q", stdout.Bytes()).Err()
		return
	}

	i.cachedRuntime = &r
	if err := i.setCached(cachePrefix, &r); err != nil {
		logging.WithError(err).Warningf(c, "Set cached runtime to file failed")
	}
	return
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
func (i *Interpreter) getCached(prefix string, v interface{}) (found bool, err error) {
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
func (i *Interpreter) setCached(prefix string, v interface{}) (err error) {
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
	return fmt.Sprintf("%X", hash.Sum32()), nil
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
