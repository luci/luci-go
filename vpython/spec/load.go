// Copyright 2017 The LUCI Authors.
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

package spec

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	cproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/system/filesystem"

	"go.chromium.org/luci/vpython/api/vpython"
)

// DefaultPartnerSuffix is the default filesystem suffix for a script's partner
// specification file.
//
// See LoadForScript for more information.
const DefaultPartnerSuffix = ".vpython"

// DefaultCommonSpecNames is the name of the "common" specification file.
//
// If a script doesn't explicitly specific a specification file, "vpython" will
// automatically walk up from the script's directory towards filesystem root
// and will use the first file named CommonName that it finds. This enables
// repository-wide and shared environment specifications.
var DefaultCommonSpecNames = []string{
	"common.vpython",
}

const (
	// DefaultInlineBeginGuard is the default loader InlineBeginGuard value.
	DefaultInlineBeginGuard = "[VPYTHON:BEGIN]"
	// DefaultInlineEndGuard is the default loader InlineEndGuard value.
	DefaultInlineEndGuard = "[VPYTHON:END]"
)

// Load loads an specification file text protobuf from the supplied path.
func Load(path string, spec *vpython.Spec) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return errors.Fmt("failed to load file from: %s: %w", path, err)
	}

	return Parse(string(content), spec)
}

// Parse loads a specification message from a content string.
func Parse(content string, spec *vpython.Spec) error {
	if err := cproto.UnmarshalTextML(content, spec); err != nil {
		return errors.Fmt("failed to unmarshal vpython.Spec: %w", err)
	}
	return nil
}

// Loader implements the generic ability to load a "vpython" spec file.
type Loader struct {
	// InlineBeginGuard is a string that signifies the beginning of an inline
	// specification. If empty, DefaultInlineBeginGuard will be used.
	InlineBeginGuard string
	// InlineEndGuard is a string that signifies the end of an inline
	// specification. If empty, DefaultInlineEndGuard will be used.
	InlineEndGuard string

	// CommonFilesystemBarriers is a list of filenames. During common spec, Loader
	// walks directories towards root looking for a file named CommonName. If a
	// directory is observed to contain a file in CommonFilesystemBarriers, the
	// walk will terminate after processing that directory.
	CommonFilesystemBarriers []string

	// CommonSpecNames, if not empty, is the list of common "vpython" spec files
	// to use. If empty, DefaultCommonSpecNames will be used.
	//
	// Names will be considered in the order that they appear.
	CommonSpecNames []string

	// PartnerSuffix is the filesystem suffix for a script's partner spec file. If
	// empty, DefaultPartnerSuffix will be used.
	PartnerSuffix string
}

// LoadForScript attempts to load a spec file for the specified script. If
// nothing went wrong, a nil error will be returned. If a spec file was
// identified, it will also be returned along with the path to the spec file
// itself. Otherwise, a nil spec will be returned.
//
// Spec files can be specified in a variety of ways. This function will look for
// them in the following order, and return the first one that was identified:
//
//   - Partner File
//   - Inline
//
// Partner File
// ============
//
// LoadForScript traverses the filesystem to find the specification file that is
// naturally associated with the specified
// path.
//
// If the path is a Python script (e.g, "/path/to/test.py"), isModule will be
// false, and the file will be found at "/path/to/test.py.vpython".
//
// If the path is a Python module (isModule is true), findForScript walks
// upwards in the directory structure, looking for a file that shares a module
// directory name and ends with ".vpython". For example, for module:
//
//	/path/to/foo/bar/baz/__init__.py
//	/path/to/foo/bar/__init__.py
//	/path/to/foo/__init__.py
//	/path/to/foo.vpython
//
// LoadForScript will first look at "/path/to/foo/bar/baz", then walk upwards
// until it either hits a directory that doesn't contain an "__init__.py" file,
// or finds the ES path. In this case, for module "foo.bar.baz", it will
// identify "/path/to/foo.vpython" as the ES file for that module.
//
// Inline
// ======
//
// LoadForScript scans through the contents of the file at path and attempts to
// load specification boundaries.
//
// If the file at path does not exist, or if the file does not contain spec
// guards, a nil spec will be returned.
//
// The embedded specification is a text protobuf embedded within the file. To
// parse it, the file is scanned line-by-line for a beginning and ending guard.
// The content between those guards is minimally processed, then interpreted as
// a text protobuf.
//
//	[VPYTHON:BEGIN]
//	wheel {
//	  path: ...
//	  version: ...
//	}
//	[VPYTHON:END]
//
// To allow VPYTHON directives to be embedded in a language-compatible manner
// (with indentation, comments, etc.), the processor will identify any common
// characters preceding the BEGIN and END clauses. If they match, those
// characters will be automatically stripped out of the intermediate lines. This
// can be used to embed the directives in comments:
//
//	// [VPYTHON:BEGIN]
//	// wheel {
//	//   path: ...
//	//   version: ...
//	// }
//	// [VPYTHON:END]
//
// In this case, the "// " characters will be removed.
//
// Common
// ======
//
// LoadForScript will examine successive parent directories starting from the
// script's location, looking for a file named in CommonSpecNames. If it finds
// one, it will use that as the specification file. This enables scripts to
// implicitly share an specification.
func (l *Loader) LoadForScript(c context.Context, path string, isModule bool) (*vpython.Spec, string, error) {
	// Spec search order:
	// 1. Partner File of the symbolic link (if exist)
	// 2. Partner File of the real file
	// 3. Inline specification in the script
	// 4. Common specification file from the real file

	// Partner File: Try loading the spec from an adjacent file.
	specPath, err := l.findForScript(path, isModule)
	if err != nil {
		return nil, "", errors.Fmt("failed to scan for filesystem spec: %w", err)
	}

	// Partner File: Try loading the spec from an adjacent file to the evaluated path.
	if specPath == "" && runtime.GOOS != "windows" {
		// Skip EvalSymlinks for windows because it is broken:
		// https://github.com/golang/go/issues/40180
		if path, err = filepath.EvalSymlinks(path); err != nil {
			return nil, "", errors.Fmt("failed to get real path for script: %s: %w", path, err)
		}
		specPath, err = l.findForScript(path, isModule)
		if err != nil {
			return nil, "", errors.Fmt("failed to scan for filesystem spec: %w", err)
		}
	}

	if specPath != "" {
		var spec vpython.Spec
		if err := Load(specPath, &spec); err != nil {
			return nil, "", err
		}

		logging.Infof(c, "Loaded specification from: %s", specPath)
		return &spec, specPath, nil
	}

	// Inline: Try and parse the main script for the spec file.
	mainScript := path
	if isModule {
		// Module.
		mainScript = filepath.Join(mainScript, "__main__.py")
	}

	// Assume the path is a directory until we're sure it's not, then get its directory component
	currPath := mainScript
	info, err := os.Stat(currPath)
	if err != nil {
		return nil, "", errors.Fmt("error stat-ing file: %s: %w", currPath, err)
	}

	if !info.IsDir() {
		switch spec, err := l.parseFrom(currPath); {
		case err != nil:
			return nil, "", errors.Fmt("failed to parse inline spec from: %s: %w", currPath, err)

		case spec != nil:
			logging.Infof(c, "Loaded inline spec from: %s", currPath)
			return spec, currPath, nil
		}

		// Scan starting from directory containing the main script
		currPath = filepath.Dir(currPath)
	}

	// Common: Try and identify a common specification file.
	switch path, err := l.findCommonWalkingFrom(currPath); {
	case err != nil:
		return nil, "", err

	case path != "":
		var spec vpython.Spec
		if err := Load(path, &spec); err != nil {
			return nil, "", err
		}

		logging.Infof(c, "Loaded common spec from: %s", path)
		return &spec, path, nil
	}

	// Couldn't identify a specification file.
	return nil, "", nil
}

func (l *Loader) findForScript(path string, isModule bool) (string, error) {
	if l.PartnerSuffix == "" {
		l.PartnerSuffix = DefaultPartnerSuffix
	}

	if !isModule {
		path += l.PartnerSuffix
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			// File does not exist at this path.
			return "", nil
		}
		return path, nil
	}

	// If it's a directory, scan for a ".vpython" file until we don't have a
	// __init__.py.
	for {
		prev := path

		// Directory must be a Python module.
		initPath := filepath.Join(path, "__init__.py")
		if _, err := os.Stat(initPath); err != nil {
			if os.IsNotExist(err) {
				// Not a Python module, so we're done our search.
				return "", nil
			}
			return "", errors.Fmt("failed to stat for: %s: %w", path, err)
		}

		// Does a spec file exist for this path?
		specPath := path + l.PartnerSuffix
		switch st, err := os.Stat(specPath); {
		case err == nil && !st.IsDir():
			// Found the file.
			return specPath, nil

		case os.IsNotExist(err):
			// Recurse to parent.
			path = filepath.Dir(path)
			if path == prev {
				// Finished recursing, no ES file.
				return "", nil
			}

		default:
			return "", errors.Fmt("failed to check for spec file at: %s: %w", specPath, err)
		}
	}
}

func (l *Loader) parseFrom(path string) (*vpython.Spec, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, errors.Fmt("failed to open file: %w", err)
	}
	defer fd.Close()

	// Determine our guards.
	beginGuard := l.InlineBeginGuard
	if beginGuard == "" {
		beginGuard = DefaultInlineBeginGuard
	}

	endGuard := l.InlineEndGuard
	if endGuard == "" {
		endGuard = DefaultInlineEndGuard
	}

	s := bufio.NewScanner(fd)
	var (
		content   []string
		beginLine string
		endLine   string
		inRegion  = false
	)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !inRegion {
			inRegion = strings.HasSuffix(line, beginGuard)
			beginLine = line
		} else {
			if strings.HasSuffix(line, endGuard) {
				// Finished processing.
				endLine = line
				break
			}
			content = append(content, line)
		}
	}
	if err := s.Err(); err != nil {
		return nil, errors.Fmt("error scanning file: %w", err)
	}
	if len(content) == 0 {
		return nil, nil
	}
	if endLine == "" {
		return nil, errors.New("unterminated inline spec file")
	}

	// If we have a common begin/end prefix, trim it from each content line that
	// also has it.
	prefix := beginLine[:len(beginLine)-len(beginGuard)]
	if endLine[:len(endLine)-len(endGuard)] != prefix {
		prefix = ""
	}
	if prefix != "" {
		for i, line := range content {
			if len(line) < len(prefix) {
				// This line is shorter than the prefix. Does the part of that line that
				// exists match the prefix up until that point?
				if line == prefix[:len(line)] {
					// Yes, so empty line.
					line = ""
				}
			} else {
				line = strings.TrimPrefix(line, prefix)
			}
			content[i] = line
		}
	}

	// Process the resulting file.
	var spec vpython.Spec
	if err := Parse(strings.Join(content, "\n"), &spec); err != nil {
		return nil, errors.Fmt("failed to parse spec file from: %s: %w", path, err)
	}
	return &spec, nil
}

func (l *Loader) findCommonWalkingFrom(startDir string) (string, error) {
	names := l.CommonSpecNames
	if len(names) == 0 {
		names = DefaultCommonSpecNames
	}

	// Walk until we hit root.
	prevDir := ""
	for prevDir != startDir {
		// Check the current directory before checking barrier files.
		for _, name := range names {
			checkPath := filepath.Join(startDir, name)
			switch st, err := os.Stat(checkPath); {
			case err == nil && !st.IsDir():
				return checkPath, nil

			case filesystem.IsNotExist(err):
				// Not in this directory.

			default:
				// Failed to load specification from this file.
				return "", errors.Fmt("failed to stat common spec file at: %s: %w", checkPath, err)
			}
		}

		// If we have any barrier files, check to see if they are present in this
		// directory.
		for _, name := range l.CommonFilesystemBarriers {
			barrierName := filepath.Join(startDir, name)
			if _, err := os.Stat(barrierName); err == nil {
				// Identified a barrier file in this directory.
				return "", nil
			}
		}

		// Walk up a directory.
		startDir, prevDir = filepath.Dir(startDir), startDir
	}

	// Couldn't find the file.
	return "", nil
}
