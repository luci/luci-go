// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// StreamNameSep is the separator rune for stream name tokens.
	StreamNameSep = '/'
	nameSepStr    = "/"

	// StreamPathSep is the separator rune between a stream prefix and its
	// name.
	StreamPathSep = '+'
	pathSepStr    = "+"

	// MaxStreamNameLength is the maximum size, in bytes, of a StreamName. Since
	// stream names must be valid ASCII, this is also the maximum string length.
	MaxStreamNameLength = 4096
)

// StreamName is a structured stream name.
//
// A valid stream name is composed of segments internally separated by a
// StreamNameSep (/).
//
// Each segment:
// - Consists of the following character types:
//   - Alphanumeric characters [a-zA-Z0-9]
//   - Colon (:)
//   - Underscore (_)
//   - Hyphen (-)
//   - Period (.)
// - Must begin with an alphanumeric character.
type StreamName string

func isAlnum(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// MakeStreamName constructs a new stream name from its segments.
//
// This method is guaranteed to return a valid stream name. In order to ensure
// that the arbitrary input can meet this standard, the following
// transformations will be applied as needed:
// - If the segment doesn't begin with an alphanumeric character, the fill
//   string will be prepended.
// - Any character disallowed in the segment will be replaced with an
//   underscore. This includes segment separators within a segment string.
func MakeStreamName(fill string, s ...string) (StreamName, error) {
	if len(s) == 0 {
		return "", errors.New("at least one segment must be provided")
	}
	if err := StreamName(fill).Validate(); err != nil {
		return "", fmt.Errorf("fill string must be a valid stream name: %s", err)
	}

	for idx, v := range s {
		v = strings.Map(func(r rune) rune {
			switch {
			case r >= 'A' && r <= 'Z':
				fallthrough
			case r >= 'a' && r <= 'z':
				fallthrough
			case r >= '0' && r <= '9':
				fallthrough
			case r == '.':
				fallthrough
			case r == '_':
				fallthrough
			case r == '-':
				fallthrough
			case r == ':':
				return r

			default:
				return '_'
			}
		}, v)
		if len(v) == 0 {
			v = fill
		} else {
			r, _ := utf8.DecodeRuneInString(v)
			if !isAlnum(r) {
				v = fill + v
			}
		}
		s[idx] = v
	}
	result := makeStreamName(s...)
	if err := result.Validate(); err != nil {
		return "", err
	}
	return result, nil
}

// makeStreamName constructs a StreamName by joining segments with the stream
// name separator.
//
// Note that makeStreamName does not validate the resulting stream name.
func makeStreamName(s ...string) StreamName {
	return StreamName(strings.Join(s, nameSepStr))
}

// String implements flag.String.
func (s *StreamName) String() string {
	return string(*s)
}

// Set implements flag.Value.
func (s *StreamName) Set(value string) error {
	v := StreamName(value)
	if err := v.Validate(); err != nil {
		return err
	}
	*s = v
	return nil
}

// Trim trims separator characters from the beginning and end of a StreamName.
//
// While such a StreamName is not Valid, this method helps correct small user
// input errors.
func (s StreamName) Trim() StreamName {
	for {
		r, l := utf8.DecodeRuneInString(string(s))
		if r != StreamNameSep {
			break
		}
		s = s[l:]
	}

	for {
		r, l := utf8.DecodeLastRuneInString(string(s))
		if r != StreamNameSep {
			break
		}
		s = s[:len(s)-l]
	}

	return s
}

// Join concatenates a stream name onto the end of the current name, separating
// it with a separator character.
func (s StreamName) Join(o StreamName) StreamPath {
	return StreamPath(fmt.Sprintf("%s%c%c%c%s",
		s.Trim(), StreamNameSep, StreamPathSep, StreamNameSep, o.Trim()))
}

// Validate tests whether the stream name is valid.
func (s StreamName) Validate() error {
	if len(s) == 0 {
		return errors.New("must contain at least one character")
	}
	if len(s) > MaxStreamNameLength {
		return fmt.Errorf("stream name is too long (%d > %d)", len(s), MaxStreamNameLength)
	}

	var lastRune rune
	var segmentIdx int
	for idx, r := range s {
		// Alphanumeric.
		if !isAlnum(r) {
			// The stream name must begin with an alphanumeric character.
			if idx == segmentIdx {
				return fmt.Errorf("Segment (at %d) must begin with alphanumeric character.", segmentIdx)
			}

			// Test forward slash, and ensure no adjacent forward slashes.
			if r == StreamNameSep {
				segmentIdx = idx + utf8.RuneLen(r)
			} else if !(r == '.' || r == '_' || r == '-' || r == ':') {
				// Test remaining allowed characters.
				return fmt.Errorf("Illegal charater (%c) at index %d.", r, idx)
			}
		}
		lastRune = r
	}

	// The last rune may not be a separator.
	if lastRune == StreamNameSep {
		return errors.New("Name may not end with a separator.")
	}
	return nil
}

// Segments returns the individual StreamName segments by splitting splitting
// the StreamName with StreamNameSep.
func (s StreamName) Segments() []string {
	if len(s) == 0 {
		return nil
	}
	return strings.Split(string(s), string(StreamNameSep))
}

// SegmentCount returns the total number of segments in the StreamName.
func (s StreamName) SegmentCount() int {
	if len(s) == 0 {
		return 0
	}
	return strings.Count(string(s), string(StreamNameSep)) + 1
}

// A StreamPath consists of two StreamName, joined via a StreamPathSep (+)
// separator.
type StreamPath string

// Split splits a StreamPath into its prefix and name components.
//
// If there is no divider present (e.g., foo/bar/baz), the result will parse
// as the stream prefix with an empty name component.
func (p StreamPath) Split() (StreamName, StreamName) {
	segments := StreamName(p).Segments()
	for idx, s := range segments {
		if s == pathSepStr {
			return makeStreamName(segments[:idx]...), makeStreamName(segments[idx+1:]...)
		}
	}
	return StreamName(p), ""
}

// Validate checks whether a StreamPath is valid. A valid stream path must have
// a valid prefix and tail components.
func (p StreamPath) Validate() error {
	prefix, tail := p.Split()
	if err := prefix.Validate(); err != nil {
		return err
	}
	if err := tail.Validate(); err != nil {
		return err
	}
	return nil
}
