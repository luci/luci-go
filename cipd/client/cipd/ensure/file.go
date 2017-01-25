// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ensure

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/iotools"
)

// File is an in-process representation of the 'ensure file' format.
type File struct {
	ServiceURL string

	PackagesByRoot map[string]PackageSlice
}

// ParseFile parses an ensure file from the given reader. See the package docs
// for the format of this file.
//
// This file will contain unresolved template strings for package names as well
// as unpinned package versions. Use File.Resolve() to obtain resolved+pinned
// versions of these.
func ParseFile(r io.Reader) (*File, error) {
	ret := &File{PackagesByRoot: map[string]PackageSlice{}}

	state := itemParserState{}

	// indicates that the parser is able to read $setting lines. This is flipped
	// to false on the first non-$setting statement in the file.
	settingsAllowed := true

	lineNo := 0
	makeError := func(fmtStr string, args ...interface{}) error {
		args = append([]interface{}{lineNo}, args...)
		return fmt.Errorf("failed to parse desired state (line %d): "+fmtStr, args...)
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineNo++

		// Remove all space
		line := strings.TrimSpace(scanner.Text())

		if len(line) == 0 || line[0] == '#' {
			// skip blank lines and comments
			continue
		}

		tokens := strings.SplitN(line, " ", 2)
		if len(tokens) != 2 {
			tokens = append(tokens, "") // no value means a value of ""
		}
		tok1, tok2 := tokens[0], tokens[1]

		switch c := tok1[0]; c {
		case '@', '$':
			if c == '$' {
				if !settingsAllowed {
					return nil, makeError("$setting found after non-$setting statements")
				}
			} else {
				settingsAllowed = false
			}

			if p := itemParsers[strings.ToLower(tok1)]; p != nil {
				if err := p(&state, ret, tok2); err != nil {
					return nil, makeError("%s", err)
				}
			} else {
				tag := map[byte]string{'@': "@directive", '$': "$setting"}[c]
				return nil, makeError("unknown %s: %q", tag, tok1)
			}

		default:
			settingsAllowed = false
			pkg := PackageDef{tok1, tok2, lineNo}

			// pass dummy VersionResolver to validate vers
			_, err := pkg.Resolve("dummy_plat", "dummy_arch", func(pkg, vers string) (common.Pin, error) {
				return common.Pin{
					PackageName: pkg,
					InstanceID:  vers,
				}, common.ValidateInstanceVersion(vers)
			})
			if err != nil && err != errSkipTemplate {
				return nil, err
			}

			ret.PackagesByRoot[state.curRoot] = append(ret.PackagesByRoot[state.curRoot], pkg)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ret, nil
}

// ResolvedFile only contains valid, fully-resolved information and is the
// result of calling File.Resolve.
type ResolvedFile struct {
	ServiceURL string

	PackagesByRoot map[string][]common.Pin
}

// Resolve takes the current unresolved File and expands all package templates
// using cipd/common's values for arch and platform, and also resolves all
// versions with the provided VersionResolver.
func (f *File) Resolve(rslv VersionResolver) (*ResolvedFile, error) {
	return f.ResolveWith(common.CurrentArchitecture(), common.CurrentPlatform(), rslv)
}

// ResolveWith takes the current unresolved File and expands all package
// templates using the provided values of arch and platform, and also resolves
// all versions with the provided VersionResolver.
func (f *File) ResolveWith(arch, plat string, rslv VersionResolver) (*ResolvedFile, error) {
	ret := &ResolvedFile{}

	if f.ServiceURL != "" {
		// double check the url
		if _, err := url.Parse(f.ServiceURL); err != nil {
			return nil, errors.Annotate(err).Reason("bad ServiceURL").Err()
		}
	}

	ret.ServiceURL = f.ServiceURL
	if len(f.PackagesByRoot) == 0 {
		return ret, nil
	}

	// root -> pkg -> orig_lineno
	resolvedPkgDupList := map[string]map[string]int{}

	ret.PackagesByRoot = map[string][]common.Pin{}
	for root, pkgs := range f.PackagesByRoot {
		// double-check the root
		if err := common.ValidateRoot(root); err != nil {
			return nil, errors.Annotate(err).
				Reason("normalizing %(root)q").
				D("root", root).
				Err()
		}
		for _, pkg := range pkgs {
			pin, err := pkg.Resolve(plat, arch, rslv)
			if err == errSkipTemplate {
				continue
			}
			if err != nil {
				return nil, errors.Annotate(err).Reason("resolving package").Err()
			}

			if origLineNo, ok := resolvedPkgDupList[root][pin.PackageName]; ok {
				return nil, errors.
					Reason("duplicate package in root %(root)q: %(pkg)q: defined on line %(orig)d and %(new)d").
					D("root", root).
					D("pkg", pin.PackageName).
					D("orig", origLineNo).
					D("new", pkg.LineNo).
					Err()
			}
			if resolvedPkgDupList[root] == nil {
				resolvedPkgDupList[root] = map[string]int{}
			}
			resolvedPkgDupList[root][pin.PackageName] = pkg.LineNo

			ret.PackagesByRoot[root] = append(ret.PackagesByRoot[root], pin)
		}
	}

	return ret, nil
}

// Serialize writes the File to an io.Writer in canonical order.
func (f *File) Serialize(w io.Writer) (int, error) {
	return iotools.WriteTracker(w, func(w io.Writer) error {
		needsNLs := 0
		maybeAddNL := func() {
			if needsNLs > 0 {
				w.Write(bytes.Repeat([]byte("\n"), needsNLs))
				needsNLs = 0
			}
		}

		if f.ServiceURL != "" {
			maybeAddNL()
			fmt.Fprintf(w, "$ServiceURL %s", f.ServiceURL)
			needsNLs = 2
		}

		keys := make(sort.StringSlice, 0, len(f.PackagesByRoot))
		for k := range f.PackagesByRoot {
			keys = append(keys, k)
		}
		keys.Sort()

		for _, k := range keys {
			maybeAddNL()
			if k != "" {
				fmt.Fprintf(w, "\n@Root %s", k)
				needsNLs = 1
			}

			pkgs := f.PackagesByRoot[k]
			pkgsSort := make(PackageSlice, len(pkgs))
			copy(pkgsSort, pkgs)
			sort.Sort(pkgsSort)

			for _, p := range pkgsSort {
				maybeAddNL()
				fmt.Fprintf(w, "%s", &p)
				needsNLs = 1
			}
		}

		return nil
	})
}
