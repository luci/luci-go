// Copyright 2024 The LUCI Authors.
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

package gs

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestObjectStream(t *testing.T) {
	const (
		path  = "gs://bucket/file"
		line1 = "line-1"
		line2 = "line-2"
	)
	trc := testReaderClient{
		data: map[Path][]byte{
			path: []byte(fmt.Sprintf("%s\n%s", line1, line2)),
		},
		readers: map[*testReader]struct{}{},
	}

	lines := make(chan string)
	os := NewObjectStream(
		context.TODO(),
		&ObjectStreamOptions{
			Client:        &trc,
			Path:          path,
			PollFrequency: 0,
			LinesC:        lines,
		},
	)

	defer os.Close()

	line := <-lines
	assert.Loosely(t, line, should.Equal(line1))

	line = <-lines
	assert.Loosely(t, line, should.Equal(line2))
}
