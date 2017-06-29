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
	"unicode"

	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/iotools"
)

// File is an in-process representation of the 'ensure file' format.
type File struct {
	ServiceURL string

	PackagesBySubdir map[string]PackageSlice
}

// ParseFile parses an ensure file from the given reader. See the package docs
// for the format of this file.
//
// This file will contain unresolved template strings for package names as well
// as unpinned package versions. Use File.Resolve() to obtain resolved+pinned
// versions of these.
func ParseFile(r io.Reader) (*File, error) {
	ret := &File{PackagesBySubdir: map[string]PackageSlice{}}

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

		tok1 := line
		tok2 := ""
		if idx := strings.IndexFunc(line, unicode.IsSpace); idx == -1 {
			// only one token. This implies a second token of ""
		} else {
			tok1, tok2 = line[:idx], strings.TrimSpace(line[idx:])
		}

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

			_, err := pkg.Resolve(func(pkg, vers string) (common.Pin, error) {
				return common.Pin{
					PackageName: pkg,
					InstanceID:  vers,
				}, common.ValidateInstanceVersion(vers)
			}, common.TemplateArgs())
			if err != nil && err != errSkipTemplate {
				return nil, err
			}

			ret.PackagesBySubdir[state.curSubdir] = append(ret.PackagesBySubdir[state.curSubdir], pkg)
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

	PackagesBySubdir common.PinSliceBySubdir
}

// Resolve takes the current unresolved File and expands all package templates
// using common.TemplateArgs(), and also resolves all versions with the provided
// VersionResolver.
func (f *File) Resolve(rslv VersionResolver) (*ResolvedFile, error) {
	return f.ResolveWith(rslv, common.TemplateArgs())
}

// ResolveWith takes the current unresolved File and expands all package
// templates using the provided values of arch and os, and also resolves
// all versions with the provided VersionResolver.
//
// templateArgs is a mapping of expansion parameter to value. Usually you'll
// want to pass common.TemplateArgs().
func (f *File) ResolveWith(rslv VersionResolver, templateArgs map[string]string) (*ResolvedFile, error) {
	ret := &ResolvedFile{}

	if f.ServiceURL != "" {
		// double check the url
		if _, err := url.Parse(f.ServiceURL); err != nil {
			return nil, errors.Annotate(err, "bad ServiceURL").Err()
		}
	}

	ret.ServiceURL = f.ServiceURL
	if len(f.PackagesBySubdir) == 0 {
		return ret, nil
	}

	// subdir -> pkg -> orig_lineno
	resolvedPkgDupList := map[string]map[string]int{}

	ret.PackagesBySubdir = common.PinSliceBySubdir{}
	for subdir, pkgs := range f.PackagesBySubdir {
		// double-check the subdir
		if err := common.ValidateSubdir(subdir); err != nil {
			return nil, errors.Annotate(err, "normalizing %q", subdir).Err()
		}
		for _, pkg := range pkgs {
			pin, err := pkg.Resolve(rslv, templateArgs)
			if err == errSkipTemplate {
				continue
			}
			if err != nil {
				return nil, errors.Annotate(err, "resolving package").Err()
			}

			if origLineNo, ok := resolvedPkgDupList[subdir][pin.PackageName]; ok {
				return nil, errors.
					Reason("duplicate package in subdir %q: %q: defined on line %d and %d",
						subdir, pin.PackageName, origLineNo, pkg.LineNo).Err()
			}
			if resolvedPkgDupList[subdir] == nil {
				resolvedPkgDupList[subdir] = map[string]int{}
			}
			resolvedPkgDupList[subdir][pin.PackageName] = pkg.LineNo

			ret.PackagesBySubdir[subdir] = append(ret.PackagesBySubdir[subdir], pin)
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

		keys := make(sort.StringSlice, 0, len(f.PackagesBySubdir))
		for k := range f.PackagesBySubdir {
			keys = append(keys, k)
		}
		keys.Sort()

		for _, k := range keys {
			maybeAddNL()
			if k != "" {
				fmt.Fprintf(w, "\n@Subdir %s", k)
				needsNLs = 1
			}

			pkgs := f.PackagesBySubdir[k]
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
