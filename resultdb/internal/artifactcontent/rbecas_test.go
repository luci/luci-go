// Copyright 2021 The LUCI Authors.
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

package artifactcontent

import (
	"bufio"
	"context"
	"io"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestDownloadRBECASContent(t *testing.T) {
	ftt.Run(`TestDownloadRBECASContent`, t, func(t *ftt.Test) {
		ctx := testutil.TestingContext()

		ac := &Reader{
			RBEInstance: "projects/p/instances/a",
			Hash:        "deadbeef",
			Size:        int64(10),
		}

		var str strings.Builder
		err := ac.DownloadRBECASContent(ctx, &artifactcontenttest.FakeByteStreamClient{[]byte("contentspart2\n")}, func(_ context.Context, pr io.Reader) error {
			sc := bufio.NewScanner(pr)
			for sc.Scan() {
				str.Write(sc.Bytes())
				str.Write([]byte("\n"))
			}
			return nil
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, str.String(), should.Equal("contentspart1\ncontentspart2\n"))
	})
}
