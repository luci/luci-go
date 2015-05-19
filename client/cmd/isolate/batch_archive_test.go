// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"testing"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/maruel/ut"
)

func TestConvertPyToGoArchiveCMDArgs(t *testing.T) {
	data := []struct {
		input    []string
		expected []string
	}{
		// Simple.
		{
			[]string{"--path-variable", "key=value"},
			[]string{"--path-variable", "key=value"},
		},
		{
			[]string{"--path-variable", "key", "value1"},
			[]string{"--path-variable", "key=value1"},
		},
		// That's how python isolate works.
		{
			[]string{"--path-variable", "key", "--even-this-value"},
			[]string{"--path-variable", "key=--even-this-value"}},
		// Other args.
		{
			[]string{"-x", "--var", "--path-variable", "key", "value"},
			[]string{"-x", "--var", "--path-variable", "key=value"}},
		{
			[]string{"--path-variable", "key", "value", "posarg"},
			[]string{"--path-variable", "key=value", "posarg"},
		},
		// Too few args are just ignored.
		{
			[]string{"--path-variable"},
			[]string{"--path-variable"},
		},
		{
			[]string{"--path-variable", "key-and-no-value"},
			[]string{"--path-variable", "key-and-no-value"},
		},
	}
	for i, line := range data {
		ut.AssertEqualIndex(t, i, line.expected, convertPyToGoArchiveCMDArgs(line.input))
	}
}

func TestInvalidArchiveCMD(t *testing.T) {
	_, err := parseArchiveCMD([]string{}, "")
	ut.AssertEqual(t, "-isolated must be specified", err.Error())
}

func TestArchiveCMDParsing(t *testing.T) {
	args := []string{
		"--isolated", ".isolated",
		"--isolate", ".isolate",
		"--path-variable", "DEPTH", "../..",
		"--path-variable", "PRODUCT_DIR", "../../out/Release",
		"--extra-variable", "version_full=42.0.2284.0",
		"--config-variable", "OS=linux",
	}
	opts, err := parseArchiveCMD(args, "")
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, opts.ConfigVariables, common.KeyValVars{"OS": "linux"})
	if common.IsWindows() {
		ut.AssertEqual(t, opts.PathVariables, common.KeyValVars{"PRODUCT_DIR": "../../out/Release", "EXECUTABLE_SUFFIX": ".exe", "DEPTH": "../.."})
	} else {
		ut.AssertEqual(t, opts.PathVariables, common.KeyValVars{"PRODUCT_DIR": "../../out/Release", "EXECUTABLE_SUFFIX": "", "DEPTH": "../.."})
	}
	ut.AssertEqual(t, opts.ExtraVariables, common.KeyValVars{"version_full": "42.0.2284.0"})
}
