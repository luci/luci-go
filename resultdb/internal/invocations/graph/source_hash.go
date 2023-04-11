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

// Package graph contains methods to explore reachable invocations.
package graph

import (
	"crypto/sha256"
	"encoding/hex"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// SourceHash stores a 12 bytes hash of sources, as raw bytes.
// To display, use the String() method.
type SourceHash string

const (
	EmptySourceHash SourceHash = ""
)

func (h SourceHash) String() string {
	return hex.EncodeToString([]byte(h))
}

// hashSources returns a hash of the given code sources proto,
// which can be used as a key in maps.
func hashSources(s *pb.Sources) SourceHash {
	h := sha256.New()
	b := pbutil.MustMarshal(s)
	_, err := h.Write(b)
	if err != nil {
		panic(err)
	}
	return SourceHash(string(h.Sum(nil)[:12]))
}
