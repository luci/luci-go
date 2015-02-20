// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

const ISOLATED_GEN_JSON_VERSION = 1

// Tree to be isolated.
type Tree struct {
	Cwd  string
	Opts ArchiveOptions
}

// Options for achiving trees.
type ArchiveOptions struct {
	Isolate           string     `json:"isolate"`
	Isolated          string     `json:"isolated"`
	Subdir            string     `json:"subdir"`
	IgnoreBrokenItems bool       `json:"ignore_broken_items"`
	Blacklist         []string   `json:"blacklist"`
	PathVariables     [][]string `json:"path_variables"`
	ExtraVariables    [][]string `json:"extra_variables"`
	ConfigVariables   [][]string `json:"config_variables"`
}

// initializes with non-nil values
func NewArchiveOptions() ArchiveOptions {
	return ArchiveOptions{
		Blacklist:       make([]string, 0),
		PathVariables:   make([][]string, 0),
		ExtraVariables:  make([][]string, 0),
		ConfigVariables: make([][]string, 0),
	}
}
