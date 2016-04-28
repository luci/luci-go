// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filesystem

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type luciPath string

func newLUCIPath(toks ...string) luciPath {
	return luciPath(strings.Join(toks, "/"))
}

func (l luciPath) explode() []string {
	return strings.Split(l.s(), "/")
}

func (l luciPath) toNative() nativePath {
	return nativePath(filepath.FromSlash(l.s()))
}

func (l luciPath) s() string {
	return string(l)
}

type configSet struct{ luciPath }

func newConfigSet(toks ...string) configSet {
	return configSet{newLUCIPath(toks...)}
}

func (c configSet) isProject() bool {
	return strings.Count(c.s(), "/") == 1 && c.hasPrefix("projects/")
}

func (c configSet) isProjectRef() bool {
	toks := c.explode()
	return len(toks) > 3 && toks[0] == "projects" && toks[2] == "refs"
}

func (c configSet) hasPrefix(prefix luciPath) bool {
	return strings.HasPrefix(c.s(), prefix.s())
}

func (c configSet) id() string {
	return strings.Split(c.s(), "/")[1]
}

func (c configSet) validate() error {
	if !c.hasPrefix("projects/") && !c.hasPrefix("services/") {
		return mark(fmt.Errorf("invalid c: %q", c))
	}
	return nil
}

type nativePath string

func (n nativePath) explode() []string {
	return strings.Split(string(n), string(filepath.Separator))
}

func (n nativePath) readlink() (nativePath, error) {
	ret, err := os.Readlink(string(n))
	return nativePath(ret), err
}

func (n nativePath) rel(other nativePath) (nativePath, error) {
	ret, err := filepath.Rel(string(n), string(other))
	return nativePath(ret), err
}

func (n nativePath) read() ([]byte, error) {
	return ioutil.ReadFile(string(n))
}

func (n nativePath) toLUCI() luciPath {
	return luciPath(filepath.ToSlash(n.s()))
}

func (n nativePath) s() string {
	return string(n)
}
