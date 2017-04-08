// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package spec

import (
	"bufio"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/luci/luci-go/vpython/api/vpython"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	cproto "github.com/luci/luci-go/common/proto"
	"github.com/luci/luci-go/common/system/filesystem"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// Suffix is the filesystem suffix for a script's partner specification file.
//
// See LoadForScript for more information.
const Suffix = ".vpython"

// CommonName is the name of the "common" specification file.
//
// If a script doesn't explicitly specific a specification file, "vpython" will
// automatically walk up from the script's directory towards filesystem root
// and will use the first file named CommonName that it finds. This enables
// repository-wide and shared environment specifications.
const CommonName = "common" + Suffix

const (
	// DefaultInlineBeginGuard is the default loader InlineBeginGuard value.
	DefaultInlineBeginGuard = "[VPYTHON:BEGIN]"
	// DefaultInlineEndGuard is the default loader InlineEndGuard value.
	DefaultInlineEndGuard = "[VPYTHON:END]"
)

// Load loads an specification file text protobuf from the supplied path.
func Load(path string) (*vpython.Spec, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to load file from: %(path)s").
			D("path", path).
			Err()
	}

	spec, err := Parse(string(content))
	if err != nil {
		return nil, errors.Annotate(err).Err()
	}
	return spec, nil
}

// Parse loads a specification message from a content string.
func Parse(content string) (*vpython.Spec, error) {
	var spec vpython.Spec
	if err := cproto.UnmarshalTextML(content, &spec); err != nil {
		return nil, errors.Annotate(err).Reason("failed to unmarshal vpython.Spec").Err()
	}
	return &spec, nil
}

// Write writes a text protobuf form of spec to path.
func Write(spec *vpython.Spec, path string) error {
	fd, err := os.Create(path)
	if err != nil {
		return errors.Annotate(err).Reason("failed to create output file").Err()
	}

	if err := proto.MarshalText(fd, spec); err != nil {
		_ = fd.Close()
		return errors.Annotate(err).Reason("failed to output text protobuf").Err()
	}

	if err := fd.Close(); err != nil {
		return errors.Annotate(err).Reason("failed to Close file").Err()
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
}

// LoadForScript attempts to load a spec file for the specified script. If
// nothing went wrong, a nil error will be returned. If a spec file was
// identified, it will also be returned. Otherwise, a nil spec will be returned.
//
// Spec files can be specified in a variety of ways. This function will look for
// them in the following order, and return the first one that was identified:
//
//	- Partner File
//	- Inline
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
// script's location, looking for a file named CommonName. If it finds one, it
// will use that as the specification file. This enables scripts to implicitly
// share an specification.
func (l *Loader) LoadForScript(c context.Context, path string, isModule bool) (*vpython.Spec, error) {
	// Partner File: Try loading the spec from an adjacent file.
	specPath, err := l.findForScript(path, isModule)
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to scan for filesystem spec").Err()
	}
	if specPath != "" {
		switch sp, err := Load(specPath); {
		case err != nil:
			return nil, errors.Annotate(err).Reason("failed to load specification file").
				D("specPath", specPath).
				Err()

		case sp != nil:
			logging.Infof(c, "Loaded specification from: %s", specPath)
			return sp, nil
		}
	}

	// Inline: Try and parse the main script for the spec file.
	mainScript := path
	if isModule {
		// Module.
		mainScript = filepath.Join(mainScript, "__main__.py")
	}
	switch sp, err := l.parseFrom(mainScript); {
	case err != nil:
		return nil, errors.Annotate(err).Reason("failed to parse inline spec from: %(script)s").
			D("script", mainScript).
			Err()

	case sp != nil:
		logging.Infof(c, "Loaded inline spec from: %s", mainScript)
		return sp, nil
	}

	// Common: Try and identify a common specification file.
	switch path, err := l.findCommonWalkingFrom(filepath.Dir(mainScript)); {
	case err != nil:
		return nil, err

	case path != "":
		spec, err := Load(path)
		if err != nil {
			return nil, err
		}

		logging.Infof(c, "Loaded common spec from: %s", path)
		return spec, nil
	}

	// Couldn't identify a specification file.
	return nil, nil
}

func (l *Loader) findForScript(path string, isModule bool) (string, error) {
	if !isModule {
		path += Suffix
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
			return "", errors.Annotate(err).Reason("failed to stat for: %(path)").
				D("path", initPath).
				Err()
		}

		// Does a spec file exist for this path?
		specPath := path + Suffix
		switch _, err := os.Stat(specPath); {
		case err == nil:
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
			return "", errors.Annotate(err).Reason("failed to check for spec file at: %(path)s").
				D("path", specPath).
				Err()
		}
	}
}

func (l *Loader) parseFrom(path string) (*vpython.Spec, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to open file").Err()
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
		return nil, errors.Annotate(err).Reason("error scanning file").Err()
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
	spec, err := Parse(strings.Join(content, "\n"))
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to parse spec file from: %(path)s").
			D("path", path).
			Err()
	}
	return spec, nil
}

func (l *Loader) findCommonWalkingFrom(startDir string) (string, error) {
	// Walk until we hit root.
	prevDir := ""
	for prevDir != startDir {
		checkPath := filepath.Join(startDir, CommonName)

		switch _, err := os.Stat(checkPath); {
		case err == nil:
			return checkPath, nil

		case filesystem.IsNotExist(err):
			// Not in this directory.

		default:
			// Failed to load specification from this file.
			return "", errors.Annotate(err).Reason("failed to stat common spec file at: %(path)s").
				D("path", checkPath).
				Err()
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
