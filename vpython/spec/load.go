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

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// Suffix is the filesystem suffix for a script's partner specification file.
//
// See LoadForScript for more information.
const Suffix = ".vpython"

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
func LoadForScript(c context.Context, path string, isModule bool) (*vpython.Spec, error) {
	// Partner File: Try loading the spec from an adjacent file.
	specPath, err := findForScript(path, isModule)
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
	switch sp, err := parseFrom(mainScript); {
	case err != nil:
		return nil, errors.Annotate(err).Reason("failed to parse inline spec from: %(script)s").
			D("script", mainScript).
			Err()

	case sp != nil:
		logging.Infof(c, "Loaded inline spec from: %s", mainScript)
		return sp, nil
	}

	// No spec file found.
	return nil, nil
}

func findForScript(path string, isModule bool) (string, error) {
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

func parseFrom(path string) (*vpython.Spec, error) {
	const (
		beginGuard = "[VPYTHON:BEGIN]"
		endGuard   = "[VPYTHON:END]"
	)

	fd, err := os.Open(path)
	if err != nil {
		return nil, errors.Annotate(err).Reason("failed to open file").Err()
	}
	defer fd.Close()

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
