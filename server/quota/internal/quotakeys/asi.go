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

package quotakeys

import (
	"encoding/ascii85"
	"strings"

	"go.chromium.org/luci/common/errors"
)

const (
	// ASIFieldDelim is used to delimit sections within Application Specific
	// Identifiers (ASIs).
	//
	// NOTE: this is ascii85-safe.
	ASIFieldDelim = "|"

	// EncodedSectionPrefix is the prefix used for ASI sections which are nominally
	// encoded with ascii85.
	//
	// NOTE: this is ascii85-safe.
	EncodedSectionPrefix     = "{"
	encodedSectionPrefixRune = '{'

	// EscapedCharacters is the set of characters which are reserved within
	// Application-specific-identifiers (ASIs) and will cause the ASI section to be
	// escaped with ascii85.
	//
	// We also encode ASI sections which start with `EncodedSectionPrefix`.
	EscapedCharacters = QuotaFieldDelim + ASIFieldDelim
)

func extendBuffer(buf []byte, n int) []byte {
	if cap(buf)-len(buf) < n {
		buf = append(make([]byte, 0, len(buf)+n), buf...)
	}
	return buf[:n]
}

// AssembleASI will return an ASI with the given sections.
//
// Sections are assembled with a "|" separator verbatim, unless the section
// contains a "|", "~" or begins with "{". In this case the section will be
// encoded with ascii85 and inserted to the final string with a "{" prefix
// character.
func AssembleASI(sections ...string) string {
	encodedSections := make([]string, len(sections))
	var buf []byte

	for i, section := range sections {
		if strings.HasPrefix(section, EncodedSectionPrefix) || strings.ContainsAny(section, EscapedCharacters) {
			buf = extendBuffer(buf, ascii85.MaxEncodedLen(len(section))+1)
			buf[0] = encodedSectionPrefixRune
			smallBuf := buf[1:]
			encodedSections[i] = string(buf[:ascii85.Encode(smallBuf, []byte(section))+1])
		} else {
			encodedSections[i] = section
		}
	}
	return strings.Join(encodedSections, ASIFieldDelim)
}

// DecodeASI will return the sections within an ASI, decoding any which appear
// to be ascii85-encoded.
//
// If a section has the ascii85 prefix, but doesn't correctly decode, this
// returns an error.
func DecodeASI(asi string) ([]string, error) {
	if asi == "" {
		return nil, nil
	}
	var buf []byte
	sections := strings.Split(asi, ASIFieldDelim)
	for i, section := range sections {
		if strings.HasPrefix(section, EncodedSectionPrefix) {
			buf = extendBuffer(buf, len(section)-1)
			ndst, _, err := ascii85.Decode(buf, []byte(section[1:]), true)
			if err != nil {
				return nil, errors.Fmt("DecodeASI: section[%d]: %w", i, err)
			}
			sections[i] = string(buf[:ndst])
		}
	}
	return sections, nil
}
