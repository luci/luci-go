// Copyright 2016 The LUCI Authors.
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

package cloud

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/info"
)

func TestInfo(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing Info service`, t, func(t *ftt.Test) {
		const maxNamespaceLen = 100

		gi := serviceInstanceGlobalInfo{
			IsDev:              false,
			ProjectID:          "project-id",
			ServiceName:        "service-name",
			VersionName:        "version-name",
			InstanceID:         "instance-id",
			ServiceAccountName: "service-account@example.com",
			RequestID:          "trace",
		}
		c := useInfo(context.Background(), &gi)

		t.Run(`Can set valid namespaces.`, func(t *ftt.Test) {
			for _, v := range []string{
				"",
				"test",
				"0123456789-ABCDEFGHIJKLMNOPQRSTUVWXYZ.abcdefghijklmnopqrstuvwxyz_",
				strings.Repeat("X", maxNamespaceLen),
			} {
				t.Run(fmt.Sprintf(`Rejects %q`, v), func(t *ftt.Test) {
					c, err := info.Namespace(c, v)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, info.GetNamespace(c), should.Equal(v))
				})
			}
		})

		t.Run(`Rejects invalid namespaces on the client.`, func(t *ftt.Test) {
			for _, v := range []string{
				" ",
				strings.Repeat("X", maxNamespaceLen+1),
			} {
				t.Run(fmt.Sprintf(`Rejects %q`, v), func(t *ftt.Test) {
					_, err := info.Namespace(c, v)
					assert.Loosely(t, err, should.ErrLike("does not match"))
				})
			}
		})
	})
}
