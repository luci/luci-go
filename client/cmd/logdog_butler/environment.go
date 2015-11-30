// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func environSplit(v string) (string, string) {
	parts := strings.SplitN(v, "=", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func environJoin(k, v string) string {
	return fmt.Sprintf("%s=%s", k, v)
}

// environ is a mutable map of environment variables. It preserves each
// environment variable verbatim (even if invalid).
type environ map[string]string

func newEnviron(s []string) environ {
	if s == nil {
		s = os.Environ()
	}

	e := make(environ, len(s))
	for _, v := range s {
		k, _ := environSplit(v)
		e[k] = v
	}
	return e
}

func (e environ) get(k string) string {
	if v, ok := e[k]; ok {
		_, value := environSplit(v)
		return value
	}
	return ""
}

func (e environ) set(k, v string) {
	e[k] = environJoin(k, v)
}

func (e environ) sorted() []string {
	r := make([]string, 0, len(e))
	for _, v := range e {
		r = append(r, v)
	}
	sort.Strings(r)
	return r
}
