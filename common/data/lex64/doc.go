// Copyright 2023 The LUCI Authors.
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

// Package lex64 is a base64 variant that preserves lexicographic ordering of
// bytestrings.
// It does this by using the alphanumeric characters, _, . and - as the alphabet.
//   - is used as padding since it is the smallest character.
//
// The codec preserves order and has roundtrip equality.
//
// This package has two versions of the alphabet. V2 is the default and
// is URL-safe. V1 is legacy and is not URL-safe.
//
// Lex64 V2 is a base64 encoding using the following alphabet:
//
// .0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz
//
// The padding character is '-', which precedes the characters above.
// Lex64, by default, omits the padding character from generated strings,
// but can be made to emit them and conform to rfc4648 (with a different alphabet).
//
// Lex64 also supports the following legacy alphabet:
//
// 0123456789=ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz
//
// At time of writing (2023 Q1), the legacy alphabet is used in Karte.
package lex64
