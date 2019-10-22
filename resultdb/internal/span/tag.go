// Copyright 2019 The LUCI Authors.
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

package span

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TagID treats the given StringPair as a tag and gets its ID.
// The ID format is "${sha256_hex(tag)}_${key}:${value}".
func TagID(tag *pb.StringPair) string {
	tagStr := pbutil.StringPairToString(tag)
	h := sha256.New()
	io.WriteString(h, tagStr)
	return fmt.Sprintf("%s_%s", hex.EncodeToString(h.Sum(nil)), tagStr)
}
