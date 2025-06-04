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

package filesystem

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
)

type luciPath string

func newLUCIPath(toks ...string) luciPath {
	return luciPath(path.Join(toks...))
}

func (l luciPath) explode() []string {
	return strings.Split(l.s(), "/")
}

func (l luciPath) s() string {
	return string(l)
}

// TODO(vadimsh): Consolidate this with config.Set.
type configSet struct{ luciPath }

func newConfigSet(toks ...string) configSet {
	return configSet{newLUCIPath(toks...)}
}

func (c configSet) isProject() bool {
	return strings.Count(c.s(), "/") == 1 && c.hasPrefix("projects/")
}

func (c configSet) hasPrefix(prefix luciPath) bool {
	return strings.HasPrefix(c.s(), prefix.s())
}

func (c configSet) id() string {
	return strings.Split(c.s(), "/")[1]
}

func (c configSet) validate() error {
	if !c.hasPrefix("projects/") && !c.hasPrefix("services/") {
		return errors.Fmt("configSet.validate: bad prefix %q", c.s())
	}
	return nil
}

type nativePath string

func (n nativePath) explode() []string {
	return strings.Split(n.s(), string(filepath.Separator))
}

func (n nativePath) readlink() (nativePath, error) {
	ret, err := os.Readlink(n.s())
	if filepath.IsAbs(ret) {
		return nativePath(ret), err
	}
	return nativePath(filepath.Join(filepath.Dir(n.s()), ret)), err
}

func (n nativePath) rel(other nativePath) (nativePath, error) {
	ret, err := filepath.Rel(n.s(), other.s())
	return nativePath(ret), err
}

func (n nativePath) read() ([]byte, error) {
	return os.ReadFile(n.s())
}

func (n nativePath) toLUCI() luciPath {
	return luciPath(filepath.ToSlash(n.s()))
}

func (n nativePath) s() string {
	return string(n)
}
