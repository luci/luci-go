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

// Package filesystem implements a file system backend for the config client.
//
// May be useful during local development.
//
// # Layout
//
// A "Config Folder" has the following format:
//   - ./services/<servicename>/...
//   - ./projects/<projectname>.json
//   - ./projects/<projectname>/...
//
// Where `...` indicates any arbitrary path-to-a-file, and <brackets> indicate
// a single non-slash-containing filesystem path token. "services", "projects",
// ".json", and slashes are all literal text.
//
// # This package allows two modes of operation
//
// # Symlink Mode
//
// This mode allows you to simulate the evolution of multiple configuration
// versions during the duration of your test. Lay out your entire directory
// structure like:
//
//   - ./current -> ./v1
//   - ./v1/config_folder/...
//   - ./v2/config_folder/...
//
// During the execution of your app, you can change ./current from v1 to v2 (or
// any other version), and that will be reflected in the config client's
// Revision field. That way you may "simulate" atomic changes in the
// configuration. You would pass the path to `current` as the basePath in the
// constructor of New.
//
// # Sloppy Version Mode
//
// The folder will be scanned each time a config file is accessed, and the
// Revision will be derived based on the current content of all config files.
// Some inconsistencies are possible if configs change during the directory
// rescan (thus "sloppiness" of this mode). This is good if you just want to
// be able to easily modify configs manually during the development without
// restarting the server or messing with symlinks.
//
// # Quirks
//
// This implementation is quite dumb, and will scan the entire directory each
// time configs are accessed, caching the whole thing in memory (content, hashes
// and metadata) and never cleaning it up. This means that if you keep editing
// the files, more and more stuff will accumulate in memory.
package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/config"
)

// ProjectConfiguration is the struct that will be used to read the
// `projectname.json` config file, if any is specified for a given project.
type ProjectConfiguration struct {
	Name string
	URL  string
}

type lookupKey struct {
	revision  string
	configSet configSet
	path      luciPath
}

type filesystemImpl struct {
	sync.RWMutex
	scannedConfigs

	basePath nativePath
	islink   bool

	contentRevisionsScanned stringset.Set
}

type scannedConfigs struct {
	contentHashMap    map[string]string
	contentRevPathMap map[lookupKey]*config.Config
	contentRevProject map[lookupKey]*config.Project
}

func newScannedConfigs() scannedConfigs {
	return scannedConfigs{
		contentHashMap:    map[string]string{},
		contentRevPathMap: map[lookupKey]*config.Config{},
		contentRevProject: map[lookupKey]*config.Project{},
	}
}

// setRevision updates 'revision' fields of all objects owned by scannedConfigs.
func (c *scannedConfigs) setRevision(revision string) {
	newRevPathMap := make(map[lookupKey]*config.Config, len(c.contentRevPathMap))
	for k, v := range c.contentRevPathMap {
		k.revision = revision
		v.Revision = revision
		newRevPathMap[k] = v
	}
	c.contentRevPathMap = newRevPathMap

	newRevProject := make(map[lookupKey]*config.Project, len(c.contentRevProject))
	for k, v := range c.contentRevProject {
		k.revision = revision
		newRevProject[k] = v
	}
	c.contentRevProject = newRevProject
}

// deriveRevision generates a revision string from data in contentHashMap.
func deriveRevision(c *scannedConfigs) string {
	keys := make([]string, 0, len(c.contentHashMap))
	for k := range c.contentHashMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	hsh := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(hsh, "%s\n%s\n", k, c.contentHashMap[k])
	}
	digest := hsh.Sum(nil)
	return hex.EncodeToString(digest[:])[:40]
}

// New returns an implementation of the config service which reads configuration
// from the local filesystem. `basePath` may be one of two things:
//   - A folder containing the following:
//     ./services/servicename/...               # service confinguations
//     ./projects/projectname.json              # project information configuration
//     ./projects/projectname/...               # project configurations
//   - A symlink to a folder as organized above:
//     -> /path/to/revision/folder
//
// If a symlink is used, all Revision fields will be the 'revision' portion of
// that path. If a non-symlink path is isued, the Revision fields will be
// derived based on the contents of the files in the directory.
//
// Any unrecognized paths will be ignored. If basePath is not a link-to-folder,
// and not a folder, this will panic.
//
// Every read access will scan each revision exactly once. If you want to make
// changes, rename the folder and re-link it.
func New(basePath string) (config.Interface, error) {
	basePath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}

	inf, err := os.Lstat(basePath)
	if err != nil {
		return nil, err
	}

	ret := &filesystemImpl{
		basePath:                nativePath(basePath),
		islink:                  (inf.Mode() & os.ModeSymlink) != 0,
		scannedConfigs:          newScannedConfigs(),
		contentRevisionsScanned: stringset.New(1),
	}

	if ret.islink {
		if inf, err = os.Stat(basePath); err != nil {
			return nil, err
		}
		if !inf.IsDir() {
			return nil, errors.Fmt("filesystem.New(%q): does not link to a directory", basePath)
		}
		if len(ret.basePath.explode()) < 1 {
			return nil, errors.Fmt("filesystem.New(%q): not enough tokens in path", basePath)
		}
	} else if !inf.IsDir() {
		return nil, errors.Fmt("filesystem.New(%q): not a directory", basePath)
	}
	return ret, nil
}

func (fs *filesystemImpl) resolveBasePath() (realPath nativePath, revision string, err error) {
	if fs.islink {
		realPath, err = fs.basePath.readlink()
		if err != nil && err.(*os.PathError).Err != os.ErrInvalid {
			return
		}
		toks := realPath.explode()
		revision = toks[len(toks)-1]
		return
	}
	return fs.basePath, "", nil
}

func parsePath(rel nativePath) (cs configSet, path luciPath, ok bool) {
	toks := rel.explode()

	const jsonExt = ".json"

	if toks[0] == "services" {
		cs = newConfigSet(toks[:2]...)
		path = newLUCIPath(toks[2:]...)
		ok = true
	} else if toks[0] == "projects" {
		ok = true
		if len(toks) == 2 && strings.HasSuffix(toks[1], jsonExt) {
			cs = newConfigSet(toks[0], toks[1][:len(toks[1])-len(jsonExt)])
		} else {
			cs = newConfigSet(toks[:2]...)
			path = newLUCIPath(toks[2:]...)
		}
	}
	return
}

func scanDirectory(realPath nativePath) (*scannedConfigs, error) {
	ret := newScannedConfigs()

	err := filepath.Walk(realPath.s(), func(rawPath string, info os.FileInfo, err error) error {
		path := nativePath(rawPath)

		if err != nil {
			return err
		}

		if !info.IsDir() {
			rel, err := realPath.rel(path)
			if err != nil {
				return err
			}

			cs, cfgPath, ok := parsePath(rel)
			if !ok {
				return nil
			}
			lk := lookupKey{"", cs, cfgPath}

			data, err := path.read()
			if err != nil {
				return err
			}

			if cfgPath == "" { // this is the project configuration file
				proj := &ProjectConfiguration{}
				if err := json.Unmarshal(data, proj); err != nil {
					return err
				}
				toks := cs.explode()
				parsedURL, err := url.ParseRequestURI(proj.URL)
				if err != nil {
					return err
				}
				ret.contentRevProject[lk] = &config.Project{
					ID:       toks[1],
					Name:     proj.Name,
					RepoType: "FILESYSTEM",
					RepoURL:  parsedURL,
				}
				return nil
			}

			content := string(data)

			hsh := sha256.Sum256(data)
			hexHsh := "v1:" + hex.EncodeToString(hsh[:])[:40]

			ret.contentHashMap[hexHsh] = content

			ret.contentRevPathMap[lk] = &config.Config{
				Meta: config.Meta{
					ConfigSet:   config.Set(cs.s()),
					Path:        cfgPath.s(),
					ContentHash: hexHsh,
					ViewURL:     "file://./" + filepath.ToSlash(cfgPath.s()),
				},
				Content: content,
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	for lk := range ret.contentRevPathMap {
		cs := lk.configSet
		if cs.isProject() {
			pk := lookupKey{"", cs, ""}
			if ret.contentRevProject[pk] == nil {
				id := cs.id()
				ret.contentRevProject[pk] = &config.Project{
					ID:       id,
					Name:     id,
					RepoType: "FILESYSTEM",
				}
			}
		}
	}

	return &ret, nil
}

func (fs *filesystemImpl) scanHeadRevision() (string, error) {
	realPath, revision, err := fs.resolveBasePath()
	if err != nil {
		return "", err
	}

	// Using symlinks? The revision is derived from the symlink target name,
	// do not rescan it all the time.
	if revision != "" {
		if err := fs.scanSymlinkedRevision(realPath, revision); err != nil {
			return "", err
		}
		return revision, nil
	}

	// If using regular directory, rescan it to find if anything changed.
	return fs.scanCurrentRevision(realPath)
}

func (fs *filesystemImpl) scanSymlinkedRevision(realPath nativePath, revision string) error {
	fs.RLock()
	done := fs.contentRevisionsScanned.Has(revision)
	fs.RUnlock()
	if done {
		return nil
	}

	fs.Lock()
	defer fs.Unlock()

	scanned, err := scanDirectory(realPath)
	if err != nil {
		return err
	}
	fs.slurpScannedConfigs(revision, scanned)
	return nil
}

func (fs *filesystemImpl) scanCurrentRevision(realPath nativePath) (string, error) {
	// Forbid parallel scans to avoid hitting the disk too hard.
	//
	// TODO(vadimsh): Can use some sort of rate limiting instead if this code is
	// ever used in production.
	fs.Lock()
	defer fs.Unlock()

	scanned, err := scanDirectory(realPath)
	if err != nil {
		return "", err
	}

	revision := deriveRevision(scanned)
	if fs.contentRevisionsScanned.Has(revision) {
		return revision, nil // no changes to configs
	}
	fs.slurpScannedConfigs(revision, scanned)
	return revision, nil
}

func (fs *filesystemImpl) slurpScannedConfigs(revision string, scanned *scannedConfigs) {
	scanned.setRevision(revision)
	for k, v := range scanned.contentHashMap {
		fs.contentHashMap[k] = v
	}
	for k, v := range scanned.contentRevPathMap {
		fs.contentRevPathMap[k] = v
	}
	for k, v := range scanned.contentRevProject {
		fs.contentRevProject[k] = v
	}
	fs.contentRevisionsScanned.Add(revision)
}

func (fs *filesystemImpl) GetConfig(ctx context.Context, cfgSet config.Set, cfgPath string, metaOnly bool) (*config.Config, error) {
	cs := configSet{luciPath(cfgSet)}
	path := luciPath(cfgPath)

	if err := cs.validate(); err != nil {
		return nil, err
	}

	revision, err := fs.scanHeadRevision()
	if err != nil {
		return nil, err
	}

	lk := lookupKey{revision, cs, path}

	fs.RLock()
	ret, ok := fs.contentRevPathMap[lk]
	fs.RUnlock()
	if ok {
		c := *ret
		if metaOnly {
			c.Content = ""
		}
		return &c, nil
	}
	return nil, config.ErrNoConfig
}

func (fs *filesystemImpl) GetConfigs(ctx context.Context, cfgSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
	cs := configSet{luciPath(cfgSet)}
	if err := cs.validate(); err != nil {
		return nil, err
	}

	out := map[string]config.Config{}
	err := fs.iterContentRevPath(func(lk lookupKey, cfg *config.Config) {
		if lk.configSet == cs && (filter == nil || filter(cfg.Path)) {
			c := *cfg
			if metaOnly {
				c.Content = ""
			}
			out[cfg.Path] = c
		}
	})

	if err != nil {
		return nil, err
	}
	return out, nil
}

func (fs *filesystemImpl) ListFiles(ctx context.Context, cfgSet config.Set) ([]string, error) {
	cs := configSet{luciPath(cfgSet)}
	if err := cs.validate(); err != nil {
		return nil, err
	}

	var files []string
	err := fs.iterContentRevPath(func(lk lookupKey, cfg *config.Config) {
		if lk.configSet == cs {
			files = append(files, cfg.Path)
		}
	})
	sort.Strings(files)
	return files, err
}

func (fs *filesystemImpl) iterContentRevPath(fn func(lk lookupKey, cfg *config.Config)) error {
	revision, err := fs.scanHeadRevision()
	if err != nil {
		return err
	}

	fs.RLock()
	defer fs.RUnlock()
	for lk, cfg := range fs.contentRevPathMap {
		if lk.revision == revision {
			fn(lk, cfg)
		}
	}
	return nil
}

func (fs *filesystemImpl) GetProjectConfigs(ctx context.Context, cfgPath string, metaOnly bool) ([]config.Config, error) {
	path := luciPath(cfgPath)

	ret := make(configList, 0, 10)
	err := fs.iterContentRevPath(func(lk lookupKey, cfg *config.Config) {
		if lk.path != path {
			return
		}
		if lk.configSet.isProject() {
			c := *cfg
			if metaOnly {
				c.Content = ""
			}
			ret = append(ret, c)
		}
	})
	sort.Sort(ret)
	return ret, err
}

func (fs *filesystemImpl) GetProjects(ctx context.Context) ([]config.Project, error) {
	revision, err := fs.scanHeadRevision()
	if err != nil {
		return nil, err
	}

	fs.RLock()
	ret := make(projList, 0, len(fs.contentRevProject))
	for lk, proj := range fs.contentRevProject {
		if lk.revision == revision {
			ret = append(ret, *proj)
		}
	}
	fs.RUnlock()
	sort.Sort(ret)
	return ret, nil
}

func (fs *filesystemImpl) Close() error {
	return nil
}
