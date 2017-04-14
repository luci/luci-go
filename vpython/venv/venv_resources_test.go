// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danjacques/gofslock/fslock"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/cipd/client/cipd"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/system/filesystem"
	"github.com/luci/luci-go/common/testing/testfs"
	"github.com/luci/luci-go/hardcoded/chromeinfra"
	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/python"
	"github.com/luci/luci-go/vpython/wheel"
)

const testDataDir = "test_data"

// remoteFiles is the set of remote files to acquire.
var remoteFiles = []struct {
	// install installs this remote file into the test environment.
	install func(te *testingLoader, path string)

	// name is the name of the file.
	name string
	// contentHash is the SHA256 has of the content.
	contentHash string

	// cipdPackage, if not empty, is the name of the CIPD package that contains
	// this file.
	cipdPackage string
	// cipdVersion is the version string of the CIPD package.
	cipdVersion string

	// urls, if not empty, is a set of remote URLs where this file can be
	// downloaded from.
	urls []string
}{
	{
		install:     func(tl *testingLoader, path string) { tl.virtualEnvZIP = path },
		name:        "virtualenv-15.1.0.zip",
		contentHash: "f7682a57c98a10d32474b4c1df75478dea9a0802c140335c0269a6ec3af46201",
		cipdPackage: "infra/test-data/vpython/virtualenv",
		cipdVersion: "version:15.1.0",
		urls: []string{
			"https://github.com/pypa/virtualenv/archive/15.1.0.zip",
		},
	},
}

// testingLoader is a map of a CIPD package name to the root directory
// that it should be loaded from.
type testingLoader struct {
	cacheDir string

	virtualEnvZIP string

	pantsWheelPath string
	shirtWheelPath string
}

// loadTestEnvironment sets up the test environment for the VirtualEnv tests.
//
// This environment includes the acquisition and construction of binary data
// that will be used to perform the VirtualEnv test suite, namely:
//
//	- Building test wheel files from source.
//	- Downloading the testing VirtualEnv package.
//
// This online setup is preferred to actually checking these binary files into
// Git, as it offers more versatility and doesn't clutter Git with binary junk.
//
// To optimize repeated test re-executions, withTestEnvironment will also cache
// the downloaded artifacts in a cache directory. All artifacts will be verified
// by their SHA256 hashes, which will be baked into the source here.
func loadTestEnvironment(ctx context.Context, t *testing.T) (*testingLoader, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to get working directory").Err()
	}

	cacheDir := filepath.Join(wd, ".venv_test_cache")
	if err := filesystem.MakeDirs(cacheDir); err != nil {
		return nil, errors.Annotate(err).Reason("failed to create cache dir").Err()
	}

	tl := testingLoader{
		cacheDir: cacheDir,
	}
	return &tl, tl.withCacheLock(t, func() error {
		return tl.ensureRemoteFilesLocked(ctx, t)
	})
}

func (tl *testingLoader) withCacheLock(t *testing.T, fn func() error) error {
	lockPath := filepath.Join(tl.cacheDir, ".lock")
	blocker := func() error {
		t.Logf("Cache [%s] is currently locked; sleeping...", lockPath)
		time.Sleep(1 * time.Second)
		return nil
	}
	return fslock.WithBlocking(lockPath, blocker, func() error {
		return fn()
	})
}

func (tl *testingLoader) ensureWheels(ctx context.Context, t *testing.T, py *python.Interpreter, tdir string) error {
	var err error
	if tl.pantsWheelPath, err = tl.buildWheelLocked(t, py, "pants-1.2-py2.py3-none-any.whl", tdir); err != nil {
		return err
	}
	if tl.shirtWheelPath, err = tl.buildWheelLocked(t, py, "shirt-3.14-py2.py3-none-any.whl", tdir); err != nil {
		return err
	}
	return nil
}

func (tl *testingLoader) Resolve(c context.Context, e *vpython.Environment) error {

	e.Spec.Virtualenv.Version = "resolved"
	for _, wheel := range e.Spec.Wheel {
		wheel.Version = "resolved"
	}
	return nil
}

func (tl *testingLoader) Ensure(c context.Context, root string, packages []*vpython.Spec_Package) error {
	for _, pkg := range packages {
		if err := tl.installPackage(pkg.Name, root); err != nil {
			return err
		}
	}
	return nil
}

func (tl *testingLoader) installPackage(name, root string) error {
	switch name {
	case "foo/bar/virtualenv":
		return unzip(tl.virtualEnvZIP, root)
	case "foo/bar/shirt":
		return copyFileIntoDir(tl.shirtWheelPath, root)
	case "foo/bar/pants":
		return copyFileIntoDir(tl.pantsWheelPath, root)

	default:
		return errors.Reason("don't know how to install %(package)q").
			D("package", name).
			Err()
	}
}

func (tl *testingLoader) buildWheelLocked(t *testing.T, py *python.Interpreter, name, outDir string) (string, error) {
	ctx := context.Background()
	w, err := wheel.ParseName(name)
	if err != nil {
		return "", errors.Annotate(err).Reason("failed to parse wheel name %(name)q").
			D("name", name).
			Err()
	}

	outWheelPath := filepath.Join(outDir, w.String())
	switch _, err := os.Stat(outWheelPath); {
	case err == nil:
		t.Logf("Using cached wheel for %q: %s", name, outWheelPath)
		return outWheelPath, nil

	case os.IsNotExist(err):
		// Will build a new wheel.
		break

	default:
		return "", errors.Annotate(err).Reason("failed to stat wheel path [%(path)s]").
			D("path", outWheelPath).
			Err()
	}

	srcDir := filepath.Join(testDataDir, w.Distribution+".src")

	// Create a bootstrap wheel-generating VirtualEnv!
	cfg := Config{
		MaxHashLen: 1, // Only going to be 1 enviroment.
		BaseDir:    filepath.Join(outDir, ".env"),
		Python:     py.Python,
		Package: vpython.Spec_Package{
			Name:    "foo/bar/virtualenv",
			Version: "whatever",
		},
		Loader: tl,
		Spec:   &vpython.Spec{},

		// Testing parameters for this bootstrap wheel-building environment.
		testPreserveInstallationCapability: true,
		testLeaveReadWrite:                 true,
	}

	// Build the wheel in a temporary directory, then copy it into outDir. This
	// will stop wheel builds from stepping on each other or inheriting each
	// others' state accidentally.
	err = testfs.WithTempDir(t, "vpython_venv_wheel", func(tdir string) error {
		buildDir := filepath.Join(tdir, "build")
		if err := filesystem.MakeDirs(buildDir); err != nil {
			return err
		}

		distDir := filepath.Join(tdir, "dist")
		if err := filesystem.MakeDirs(distDir); err != nil {
			return err
		}

		// Use an empty bootstrap VirtualEnv to build the wheel. This guarantees
		// that we actually have "setuptools" and "wheel" packages, which are
		// required for building wheels, and not necessarily present in their
		// expected forms on all systems.
		err := With(ctx, cfg, true, func(ctx context.Context, env *Env) error {
			cmd := env.InterpreterCommand()
			cmd.WorkDir = srcDir
			err := cmd.Run(ctx, "setup.py", "--no-user-cfg", "bdist_wheel",
				"--bdist-dir", buildDir,
				"--dist-dir", distDir)
			if err != nil {
				return errors.Annotate(err).Reason("failed to build wheel").Err()
			}
			return nil
		})
		if err != nil {
			return errors.Annotate(err).Reason("failed to build wheel").Err()
		}

		// Assert that the expected wheel file was generated, and copy it into
		// outDir.
		wheelPath := filepath.Join(distDir, w.String())
		if _, err := os.Stat(wheelPath); err != nil {
			return errors.Annotate(err).Reason("failed to generate wheel").Err()
		}
		if err := copyFileIntoDir(wheelPath, outDir); err != nil {
			return errors.Annotate(err).Reason("failed to install wheel").Err()
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	t.Logf("Generated wheel file %q: %s", name, outWheelPath)
	return outWheelPath, nil
}

func (tl *testingLoader) ensureRemoteFilesLocked(ctx context.Context, t *testing.T) error {
MainLoop:
	for _, rf := range remoteFiles {
		cachePath := filepath.Join(tl.cacheDir, rf.name)

		// Check if the remote file is already cached.
		err := getCachedFileLocked(t, cachePath, rf.contentHash)
		if err == nil {
			t.Logf("Remote file [%s] is already cached: [%s]", rf.name, cachePath)
			rf.install(tl, cachePath)
			continue MainLoop
		}
		t.Logf("Remote file [%s] is not cached: %s", rf.name, err)

		// Download from CIPD.
		if rf.cipdPackage != "" {
			err := cacheFromCIPDLocked(ctx, t, cachePath, rf.name, rf.contentHash, rf.cipdPackage, rf.cipdVersion)
			if err == nil {
				t.Logf("Cached remote file [%s] from CIPD source: [%s]", rf.name, cachePath)
				rf.install(tl, cachePath)
				continue MainLoop
			}
			t.Logf("Failed to load from CIPD package %q @%q: %s", rf.cipdPackage, rf.cipdVersion, err)
		}

		// Download from URL.
		for _, url := range rf.urls {
			err := cacheFromURLLocked(t, cachePath, rf.contentHash, url)
			if err == nil {
				t.Logf("Cached remote file [%s] from URL [%s]: [%s]", rf.name, url, cachePath)
				rf.install(tl, cachePath)
				continue MainLoop
			}
			t.Logf("Failed to load from URL %q: %s", url, err)
		}

		return errors.Reason("failed to acquire remote file %(name)q").
			D("name", rf.name).
			Err()
	}

	return nil
}

func getCachedFileLocked(t *testing.T, cachePath, hash string) error {
	return validateHash(t, cachePath, hash, true)
}

func validateHash(t *testing.T, path, hash string, deleteIfInvalid bool) error {
	fd, err := os.Open(path)
	if err != nil {
		return errors.Annotate(err).Reason("failed to open file").Err()
	}
	defer fd.Close()

	h := sha256.New()
	if _, err := io.Copy(h, fd); err != nil {
		return errors.Annotate(err).Reason("failed to hash file").Err()
	}

	if err := hashesEqual(h, hash); err != nil {
		t.Logf("File [%s] has invalid hash: %s", path, err)

		if deleteIfInvalid {
			if err := os.Remove(path); err != nil {
				t.Logf("Failed to delete invalid hash file [%s]: %s", path, err)
			}
		}
		return err
	}

	return nil
}

func hashesEqual(h hash.Hash, expected string) error {
	if v := hex.EncodeToString(h.Sum(nil)); v != expected {
		return errors.Reason("hash %(actual)q doesn't match expected %(expected)q").
			D("actual", v).
			D("expected", expected).
			Err()
	}
	return nil
}

var testCIPDClientOptions = cipd.ClientOptions{
	ServiceURL: chromeinfra.CIPDServiceURL,
	UserAgent:  "vpython venv tests",
}

func cacheFromCIPDLocked(ctx context.Context, t *testing.T, cachePath, name, hash, pkg, version string) error {
	return testfs.WithTempDir(t, "vpython_venv_cipd", func(tdir string) error {
		opts := testCIPDClientOptions
		opts.Root = tdir

		client, err := cipd.NewClient(opts)
		if err != nil {
			return errors.Annotate(err).Reason("failed to create CIPD client").Err()
		}

		pin, err := client.ResolveVersion(ctx, pkg, version)
		if err != nil {
			return errors.Annotate(err).Reason("failed to resolve CIPD version for %(pkg)s @%(version)s").
				D("pkg", pkg).
				D("version", version).
				Err()
		}

		if err := client.FetchAndDeployInstance(ctx, "", pin); err != nil {
			return errors.Annotate(err).Reason("failed to fetch/deploy CIPD package").Err()
		}

		path := filepath.Join(opts.Root, name)
		if err := validateHash(t, path, hash, false); err != nil {
			// Do not export the invalid path.
			return err
		}

		if err := copyFile(path, cachePath, nil); err != nil {
			return errors.Annotate(err).Reason("failed to install CIPD package file").Err()
		}

		return nil
	})
}

func cacheFromURLLocked(t *testing.T, cachePath, hash, url string) (err error) {
	resp, err := http.Get(url)
	if err != nil {
		t.Logf("Failed to GET file from URL [%s]: %s", url, err)
	}
	defer resp.Body.Close()

	fd, err := os.Create(cachePath)
	if err != nil {
		t.Logf("Failed to create output file [%s]: %s", cachePath, err)
	}
	defer func() {
		if closeErr := fd.Close(); closeErr != nil && err == nil {
			err = errors.Annotate(closeErr).Reason("failed to close file").Err()
		}
	}()

	h := sha256.New()
	tr := io.TeeReader(resp.Body, h)
	if _, err := io.Copy(fd, tr); err != nil {
		return errors.Annotate(err).Reason("failed to download").Err()
	}

	if err = hashesEqual(h, hash); err != nil {
		return
	}
	return nil
}

func unzip(src, dst string) error {
	fd, err := zip.OpenReader(src)
	if err != nil {
		return errors.Annotate(err).Reason("failed to open ZIP reader").Err()
	}
	defer fd.Close()

	for _, f := range fd.File {
		path := filepath.Join(dst, filepath.FromSlash(f.Name))
		fi := f.FileInfo()

		// Unzip this entry.
		if fi.IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return errors.Annotate(err).Reason("failed to mkdir").Err()
			}
		} else {
			if err := copyFileOpener(f.Open, path, fi); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFileIntoDir(src, dstDir string) error {
	return copyFile(src, filepath.Join(dstDir, filepath.Base(src)), nil)
}

func copyFile(src, dst string, fi os.FileInfo) error {
	opener := func() (io.ReadCloser, error) { return os.Open(src) }
	return copyFileOpener(opener, dst, fi)
}

func copyFileOpener(opener func() (io.ReadCloser, error), dst string, fi os.FileInfo) (err error) {
	sfd, err := opener()
	if err != nil {
		return errors.Annotate(err).Reason("failed to open source").Err()
	}
	defer sfd.Close()

	dfd, err := os.Create(dst)
	if err != nil {
		return errors.Annotate(err).Reason("failed to create destination").Err()
	}
	defer func() {
		if closeErr := dfd.Close(); closeErr != nil && err == nil {
			err = errors.Annotate(closeErr).Reason("failed to close destination").Err()
		}
	}()

	if _, err := io.Copy(dfd, sfd); err != nil {
		return errors.Annotate(err).Reason("failed to copy file").Err()
	}
	if fi != nil {
		if err := os.Chmod(dst, fi.Mode()); err != nil {
			return errors.Annotate(err).Reason("failed to chmod").Err()
		}
	}
	return nil
}
