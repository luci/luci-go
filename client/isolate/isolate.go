// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package isolate implements the code to process '.isolate' files.
package isolate

import (
	"regexp"

	"github.com/luci/luci-go/client/internal/common"
)

const ISOLATED_GEN_JSON_VERSION = 1
const VALID_VARIABLE = "[A-Za-z_][A-Za-z_0-9]*"

var VALID_VARIABLE_MATCHER = regexp.MustCompile(VALID_VARIABLE)

func IsValidVariable(variable string) bool {
	return VALID_VARIABLE_MATCHER.MatchString(variable)
}

// Tree to be isolated.
type Tree struct {
	Cwd  string
	Opts ArchiveOptions
}

// ArchiveOptions for achiving trees.
type ArchiveOptions struct {
	Isolate         string            `json:"isolate"`
	Isolated        string            `json:"isolated"`
	Blacklist       []string          `json:"blacklist"`
	PathVariables   common.KeyValVars `json:"path_variables"`
	ExtraVariables  common.KeyValVars `json:"extra_variables"`
	ConfigVariables common.KeyValVars `json:"config_variables"`
}

// NewArchiveOptions initializes with non-nil values
func (a *ArchiveOptions) Init() {
	a.Blacklist = []string{}
	a.PathVariables = common.KeyValVars{}
	a.ExtraVariables = common.KeyValVars{}
	a.ConfigVariables = common.KeyValVars{}
}

func IsolateAndArchive(trees []Tree, namespace string, server string) (
	map[string]string, error) {
	return nil, nil
}
