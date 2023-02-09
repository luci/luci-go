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

package lex64

import (
	"encoding/base64"
	"fmt"
)

// Scheme identifies the alphabet being used and whether or not it has padding.
type Scheme string

const (
	V1Padding     Scheme = "v1+pad"
	V1            Scheme = "v1"
	V2Padding     Scheme = "v2+pad"
	V2            Scheme = "v2"
	DefaultScheme Scheme = V2
)

// The alphabetV1 consists of the alphanumeric characters and = and _.
//
// This is the legacy encoding used for Karte events with the "zzzz" header.
const alphabetV1 = "0123456789=ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"

// The alphabetV2 consists of the alphanumeric characters and . and _.
const alphabetV2 = ".0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"

var encodings = map[Scheme]*base64.Encoding{
	V1Padding: base64.NewEncoding(alphabetV1).WithPadding('-'),
	V1:        base64.NewEncoding(alphabetV1).WithPadding(base64.NoPadding),
	V2Padding: base64.NewEncoding(alphabetV2).WithPadding('-'),
	V2:        base64.NewEncoding(alphabetV2).WithPadding(base64.NoPadding),
}

// GetEncoding gets the encoding, parameterized by whether
// we want padding or not.
func GetEncoding(s Scheme) (*base64.Encoding, error) {
	enc := encodings[s]
	if enc == nil {
		return nil, fmt.Errorf("invalid lex64 version %q", s)
	}
	return enc, nil
}

// Encode takes an array of bytes and converts it to a UTF-8 string
// encoded in lexicographic base64.
func Encode(encoding *base64.Encoding, src []byte) (string, error) {
	bytes, err := doEncode(encoding, src)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Decode takes a lexicographic base64 string and converts it to an array of
// bytes.
func Decode(encoding *base64.Encoding, src string) ([]byte, error) {
	bytes, err := doDecode(encoding, []byte(src))
	if err != nil {
		return nil, err
	}
	return bytes, err
}
