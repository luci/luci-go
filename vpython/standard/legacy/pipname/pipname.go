// Copyright 2026 The LUCI Authors.
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

// Package pipname provides low-level utilities to normalize CIPD package names
// and version strings into PEP-conforming formats for pip requirements.
package pipname

import (
	"strings"
	"unicode"
)

// PipNameFromPackageName extracts and normalizes the standard pip package name from a CIPD path.
func PipNameFromPackageName(name string) string {
	res := name
	const prefix = "infra/python/wheels/"
	if after, ok := strings.CutPrefix(name, prefix); ok {
		res = after
		if idx := strings.IndexByte(res, '/'); idx != -1 {
			res = res[:idx]
		}
	} else {
		curr := name
		for {
			idx := strings.LastIndexByte(curr, '/')
			segment := curr
			if idx != -1 {
				segment = curr[idx+1:]
				curr = curr[:idx]
			}
			if segment != "" && !strings.Contains(segment, "${") {
				res = segment
				break
			}
			if idx == -1 {
				break
			}
		}
	}

	for _, suffix := range [...]string{"-py2_py3", "_py2_py3", "-py3", "_py3", "-py2", "_py2"} {
		if strings.HasSuffix(res, suffix) {
			res = res[:len(res)-len(suffix)]
			break
		}
	}

	return normalizePackageName(res)
}

// normalizePackageName normalizes a raw Python package name to conform to standard pip guidelines.
// It processes the string in a single allocation-free pass: converting all uppercase characters to lowercase,
// and collapsing any contiguous sequences of separator characters ('-', '_', '.') into a single hyphen ('-').
func normalizePackageName(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))

	inSeparator := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c |= ' ' // Fast inlined ASCII-only case folding (equivalent to strings.ToLower but avoiding heap allocations).
		}

		if c == '-' || c == '_' || c == '.' {
			if !inSeparator {
				sb.WriteByte('-')
				inSeparator = true
			}
		} else {
			sb.WriteByte(c)
			inSeparator = false
		}
	}
	return sb.String()
}

// PipVersionFromPackageVersion normalizes a legacy package version string to PEP 440 specification layout.
func PipVersionFromPackageVersion(version string) string {
	v := version
	if strings.HasPrefix(v, "version:2@") {
		v = v[10:]
	} else if strings.HasPrefix(v, "version:") {
		v = v[8:]
	}

	if len(v) == 0 {
		return v
	}
	if !unicode.IsDigit(rune(v[0])) && !(v[0] == 'v' && len(v) > 1 && unicode.IsDigit(rune(v[1]))) {
		return v
	}

	segments := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '+'
	})

	offset := 0
	for _, seg := range segments {
		if len(seg) == 0 {
			continue
		}
		idx := strings.Index(v[offset:], seg)
		if idx == -1 {
			continue
		}
		actualIdx := offset + idx
		offset = actualIdx + len(seg)

		allDigits := true
		for _, r := range seg {
			if !unicode.IsDigit(r) {
				allDigits = false
				break
			}
		}
		if !allDigits {
			isPre := false
			for _, pre := range [...]string{"a", "b", "rc", "alpha", "beta", "pre", "preview", "c", "post", "rev", "r", "dev"} {
				if strings.HasPrefix(strings.ToLower(seg), pre) {
					rest := seg[len(pre):]
					isNumeric := true
					for _, r := range rest {
						if !unicode.IsDigit(r) {
							isNumeric = false
							break
						}
					}
					if isNumeric {
						isPre = true
						break
					}
				}
			}
			if !isPre {
				if actualIdx > 0 {
					sep := v[actualIdx-1]
					if sep == '.' || sep == '-' {
						localSuffix := strings.ReplaceAll(v[actualIdx:], "-", ".")
						return v[:actualIdx-1] + "+" + localSuffix
					}
				}
			}
		}
	}

	return v
}
