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

package value

import (
	"testing"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestURL(t *testing.T) {
	t.Parallel()

	assert.That(t, URL[*emptypb.Empty](), should.Equal(TypePrefix+"google.protobuf.Empty"))
	assert.That(t, URLMsg((*emptypb.Empty)(nil)), should.Equal(TypePrefix+"google.protobuf.Empty"))
}

func TestURLPatternPackageOf(t *testing.T) {
	t.Parallel()

	assert.That(t, URLPatternPackageOf[*emptypb.Empty](), should.Equal(TypePrefix+"google.protobuf.*"))
	assert.That(t, URLPatternPackageOfMsg((*emptypb.Empty)(nil)), should.Equal(TypePrefix+"google.protobuf.*"))
}
