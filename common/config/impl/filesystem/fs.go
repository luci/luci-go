// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package filesystem implements a file system backend for the config client.
//
// May be useful during local development.
//
// Layout
//
// A "Config Folder" has the following format:
//   - ./services/<servicename>/...
//   - ./projects/<projectname>.json
//   - ./projects/<projectname>/...
//   - ./projects/<projectname>/refs/<refname>/...
//
// Where `...` indicates any arbitrary path-to-a-file, and <brackets> indicate
// a single non-slash-containing filesystem path token. "services", "projects",
// ".json", and "refs" and slashes are all literal text.
//
// This package allows two modes of operation
//
// Symlink Mode
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
// any other verison), and that will be reflected in the config client's
// Revision field. You would pass the path to `current` as the basePath in the
// constructor of New or Use.
//
// Single Version Mode
//
// This is the same as Symlink mode, except that the folder will be scanned
// exactly once, and the Revision will always be the literal string "current".
// This is good if you just want to start with some fixed configuration, and
// that's it.
//
// Quirks
//
// This implementation is quite dumb, and will scan the entire version on the
// first read access to it, caching the whole thing in memory (content, hashes
// and metadata). This means that if you edit one of the files, the change will
// not be reflected until you either restart the webserver, or you symlink to
// a new version.
package filesystem

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/stringset"
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

	basePath nativePath
	islink   bool

	contentRevPathMap       map[lookupKey]*config.Config
	contentRevProject       map[lookupKey]*config.Project
	contentHashMap          map[string]string
	contentRevisionsScanned stringset.Set
}

// New returns an implementation of the config service which reads configuration
// from the local filesystem. `basePath` may be one of two things:
//   * A folder containing the following:
//     ./services/servicename/...               # service configuations
//     ./projects/projectname.json              # project information configuation
//     ./projects/projectname/...               # project configuations
//     ./projects/projectname/refs/refname/...  # project ref configuations
//   * A symlink to a folder as organized above:
//     -> /path/to/revision/folder
//
// If a symlink is used, all Revision fields will be the 'revision' portion of
// that path. If a non-symlink path is isued, the Revision fields will be
// "current".
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
		contentHashMap:          map[string]string{},
		contentRevPathMap:       map[lookupKey]*config.Config{},
		contentRevProject:       map[lookupKey]*config.Project{},
		contentRevisionsScanned: stringset.New(1),
	}

	if ret.islink {
		inf, err := os.Stat(basePath)
		if err != nil {
			return nil, err
		}
		if !inf.IsDir() {
			return nil, (errors.Reason("filesystem.New(%(basePath)q): does not link to a directory").
				D("basePath", basePath).Err())
		}
		if len(ret.basePath.explode()) < 1 {
			return nil, (errors.Reason("filesystem.New(%(basePath)q): not enough tokens in path").
				D("basePath", basePath).Err())
		}
	} else if !inf.IsDir() {
		return nil, (errors.Reason("filesystem.New(%(basePath)q): not a directory").
			D("basePath", basePath).Err())
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
	return fs.basePath, "current", nil
}

func parsePath(rel nativePath) (configSet configSet, path luciPath, ok bool) {
	toks := rel.explode()

	const jsonExt = ".json"

	if toks[0] == "services" {
		configSet = newConfigSet(toks[:2]...)
		path = newLUCIPath(toks[2:]...)
		ok = true
	} else if toks[0] == "projects" {
		ok = true
		if len(toks) > 2 && toks[2] == "refs" { // projects/p/refs/r/...
			if len(toks) > 4 {
				configSet = newConfigSet(toks[:4]...)
				path = newLUCIPath(toks[4:]...)
			} else {
				// otherwise it's invalid /projects/p/refs or /projects/p/refs/somefile
				ok = false
			}
		} else if len(toks) == 2 && strings.HasSuffix(toks[1], jsonExt) {
			configSet = newConfigSet(toks[0], toks[1][:len(toks[1])-len(jsonExt)])
		} else {
			configSet = newConfigSet(toks[:2]...)
			path = newLUCIPath(toks[2:]...)
		}
	}
	return
}

func (fs *filesystemImpl) scanRevision(realPath nativePath, revision string) error {
	fs.RLock()
	done := fs.contentRevisionsScanned.Has(revision)
	fs.RUnlock()
	if done {
		return nil
	}

	fs.Lock()
	defer fs.Unlock()

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

			configSet, cfgPath, ok := parsePath(rel)
			if !ok {
				return nil
			}
			lk := lookupKey{revision, configSet, cfgPath}

			data, err := path.read()
			if err != nil {
				return err
			}

			if cfgPath == "" { // this is the project configuration file
				proj := &ProjectConfiguration{}
				if err := json.Unmarshal(data, proj); err != nil {
					return err
				}
				toks := configSet.explode()
				parsedURL, err := url.ParseRequestURI(proj.URL)
				if err != nil {
					return err
				}
				fs.contentRevProject[lk] = &config.Project{
					ID:       toks[1],
					Name:     proj.Name,
					RepoType: "FILESYSTEM",
					RepoURL:  parsedURL,
				}
				return nil
			}

			content := string(data)

			hsh := sha1.Sum(data)
			hexHsh := "v1:" + hex.EncodeToString(hsh[:])

			fs.contentHashMap[hexHsh] = content

			fs.contentRevPathMap[lk] = &config.Config{
				ConfigSet: configSet.s(), Path: cfgPath.s(),

				Content:     content,
				ContentHash: hexHsh,

				Revision: revision,
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for lk := range fs.contentRevPathMap {
		cs := lk.configSet
		if cs.isProject() {
			pk := lookupKey{revision, cs, ""}
			if fs.contentRevProject[pk] == nil {
				id := cs.id()
				fs.contentRevProject[pk] = &config.Project{
					ID:       id,
					Name:     id,
					RepoType: "FILESYSTEM",
				}
			}
		}
	}

	fs.contentRevisionsScanned.Add(revision)
	return nil
}

func (fs *filesystemImpl) ServiceURL() url.URL {
	return url.URL{
		Scheme: "file",
		Path:   fs.basePath.s(),
	}
}

func (fs *filesystemImpl) GetConfig(cfgSet, cfgPath string, hashOnly bool) (*config.Config, error) {
	configSet := configSet{luciPath(cfgSet)}
	path := luciPath(cfgPath)

	if err := configSet.validate(); err != nil {
		return nil, err
	}

	realPath, revision, err := fs.resolveBasePath()
	if err != nil {
		return nil, err
	}

	if err = fs.scanRevision(realPath, revision); err != nil {
		return nil, err
	}

	lk := lookupKey{revision, configSet, path}

	fs.RLock()
	ret, ok := fs.contentRevPathMap[lk]
	fs.RUnlock()
	if ok {
		return ret, nil
	}
	return nil, config.ErrNoConfig
}

func (fs *filesystemImpl) GetConfigByHash(contentHash string) (string, error) {
	realPath, revision, err := fs.resolveBasePath()
	if err != nil {
		return "", err
	}

	if err = fs.scanRevision(realPath, revision); err != nil {
		return "", err
	}

	fs.RLock()
	content, ok := fs.contentHashMap[contentHash]
	fs.RUnlock()
	if ok {
		return content, nil
	}
	return "", config.ErrNoConfig
}

func (fs *filesystemImpl) GetConfigSetLocation(cfgSet string) (*url.URL, error) {
	configSet := configSet{luciPath(cfgSet)}

	if err := configSet.validate(); err != nil {
		return nil, err
	}
	realPath, _, err := fs.resolveBasePath()
	if err != nil {
		return nil, err
	}
	return &url.URL{
		Scheme: "file",
		Path:   realPath.toLUCI().s() + "/" + configSet.s(),
	}, nil
}

func (fs *filesystemImpl) iterContentRevPath(fn func(lk lookupKey, cfg *config.Config)) error {
	realPath, revision, err := fs.resolveBasePath()
	if err != nil {
		return err
	}

	if err = fs.scanRevision(realPath, revision); err != nil {
		return err
	}

	fs.RLock()
	defer fs.RUnlock()
	for lk, cfg := range fs.contentRevPathMap {
		fn(lk, cfg)
	}
	return nil
}

func (fs *filesystemImpl) GetProjectConfigs(cfgPath string, hashesOnly bool) ([]config.Config, error) {
	path := luciPath(cfgPath)

	ret := make(configList, 0, 10)
	err := fs.iterContentRevPath(func(lk lookupKey, cfg *config.Config) {
		if lk.path != path {
			return
		}
		if lk.configSet.isProject() {
			c := *cfg
			if hashesOnly {
				c.Content = ""
			}
			ret = append(ret, c)
		}
	})
	sort.Sort(ret)
	return ret, err
}

func (fs *filesystemImpl) GetProjects() ([]config.Project, error) {
	realPath, revision, err := fs.resolveBasePath()
	if err != nil {
		return nil, err
	}

	if err = fs.scanRevision(realPath, revision); err != nil {
		return nil, err
	}

	fs.RLock()
	ret := make(projList, 0, len(fs.contentRevProject))
	for _, proj := range fs.contentRevProject {
		ret = append(ret, *proj)
	}
	fs.RUnlock()
	sort.Sort(ret)
	return ret, nil
}

func (fs *filesystemImpl) GetRefConfigs(cfgPath string, hashesOnly bool) ([]config.Config, error) {
	path := luciPath(cfgPath)

	ret := make(configList, 0, 10)
	err := fs.iterContentRevPath(func(lk lookupKey, cfg *config.Config) {
		if lk.path != path {
			return
		}
		if lk.configSet.isProjectRef() {
			c := *cfg
			if hashesOnly {
				c.Content = ""
			}
			ret = append(ret, c)
		}
	})
	sort.Sort(ret)
	return ret, err
}

func (fs *filesystemImpl) GetRefs(projectID string) ([]string, error) {
	pfx := luciPath("projects/" + projectID + "/refs")
	ret := stringset.New(0)
	err := fs.iterContentRevPath(func(lk lookupKey, cfg *config.Config) {
		if lk.configSet.hasPrefix(pfx) {
			ret.Add(newConfigSet(lk.configSet.explode()[2:]...).s())
		}
	})
	retSlc := ret.ToSlice()
	sort.Strings(retSlc)
	return retSlc, err
}
