// Copyright 2022 The LUCI Authors.
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

// Package util contains utility functions
package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

/*
	 	NormalizeFilePath returns the normalized the file path.
		Strips leading "/" or "\"
		Converts "\\" and "//" to "/"
		Resolves ".." and "." from the file path
		e.g.
		//BUILD.gn  -> BUILD.gn
		../a/b/c.cc -> a/b/c.cc
		a/b/./c.cc  -> a/b/c.cc
*/
func NormalizeFilePath(fp string) string {
	fp = strings.TrimLeft(fp, "\\/")
	fp = strings.ReplaceAll(fp, "\\", "/")
	fp = strings.ReplaceAll(fp, "//", "/")

	// path.Clean cannot handle the case like ../c.cc, so
	// we need to do it manually
	parts := strings.Split(fp, "/")
	filteredParts := []string{}
	for _, part := range parts {
		if part == "." {
			continue
		} else if part == ".." {
			if len(filteredParts) > 0 {
				filteredParts = filteredParts[:len(filteredParts)-1]
			}
		} else {
			filteredParts = append(filteredParts, part)
		}
	}
	return strings.Join(filteredParts, "/")
}

// GetCanonicalFileName return the file name without extension and common suffixes.
func GetCanonicalFileName(fp string) string {
	fp = NormalizeFilePath(fp)
	name := filepath.Base(fp)
	return StripExtensionAndCommonSuffixFromFileName(name)
}

// StripExtensionAndCommonSuffix extension and common suffixes from file name.
// Examples:
// file_impl.cc, file_unittest.cc, file_impl_mac.h -> file
func StripExtensionAndCommonSuffixFromFileName(fileName string) string {
	fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName))
	commonSuffixes := []string{
		"impl",
		"browser_tests", // Those test suffixes are here for completeness, in compile analysis we will not use them
		"browser_test",
		"browsertest",
		"browsertests",
		"unittests",
		"unittest",
		"tests",
		"test",
		"gcc",
		"msvc",
		"arm",
		"arm64",
		"mips",
		"portable",
		"x86",
		"android",
		"ios",
		"linux",
		"mac",
		"ozone",
		"posix",
		"win",
		"aura",
		"x",
		"x11",
	}
	for {
		found := false
		for _, suffix := range commonSuffixes {
			suffix = "_" + suffix
			if strings.HasSuffix(fileName, suffix) {
				found = true
				fileName = strings.TrimSuffix(fileName, suffix)
			}
		}
		if !found {
			break
		}
	}
	return fileName
}

// StripExtensionAndCommonSuffix extension and common suffixes from file path.
// Same as StripExtensionAndCommonSuffixFromFileName, but maintain the path.
// If the path is ".", just return the name.
func StripExtensionAndCommonSuffixFromFilePath(fp string) string {
	fp = NormalizeFilePath(fp)
	name := filepath.Base(fp)
	name = StripExtensionAndCommonSuffixFromFileName(name)
	dir := filepath.Dir(fp)
	if dir == "." {
		return name
	}
	return fmt.Sprintf("%s/%s", dir, name)
}
