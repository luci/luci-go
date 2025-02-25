// Copyright 2015 The LUCI Authors.
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

package tokens

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
)

func Example() {
	kind := TokenKind{
		Algo:       TokenAlgoHmacSHA256,
		Expiration: 30 * time.Minute,
		SecretKey:  "secret_key_name",
		Version:    1,
	}

	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, time.Unix(1444945245, 0))
	ctx = secrets.Use(ctx, &testsecrets.Store{})

	// Make a token.
	token, err := kind.Generate(ctx, []byte("state"), map[string]string{"k": "v"}, 0)
	if err != nil {
		fmt.Printf("error - %s\n", err)
		return
	}
	fmt.Printf("token - %s\n", token)

	// Validate it, extract embedded data.
	embedded, err := kind.Validate(ctx, token, []byte("state"))
	if err != nil {
		fmt.Printf("error - %s\n", err)
		return
	}
	fmt.Printf("embedded - %s\n", embedded)

	// Output:
	// token - AXsiX2kiOiIxNDQ0OTQ1MjQ1MDAwIiwiayI6InYifQJ85lxSuuoYaZ2q0ecPB5-E8Wv9J2Llh0D4Y4wRWCbx
	// embedded - map[k:v]
}

func TestGenerate(t *testing.T) {
	kind := TokenKind{
		Algo:       TokenAlgoHmacSHA256,
		Expiration: 30 * time.Minute,
		SecretKey:  "secret_key_name",
		Version:    1,
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := testContext()
		token, err := kind.Generate(ctx, nil, nil, 0)
		assert.Loosely(t, token, should.NotEqual(""))
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Empty key", t, func(t *ftt.Test) {
		ctx := testContext()
		token, err := kind.Generate(ctx, nil, map[string]string{"": "v"}, 0)
		assert.Loosely(t, token, should.BeEmpty)
		assert.Loosely(t, err, should.ErrLike("empty key"))
	})

	ftt.Run("Forbidden key", t, func(t *ftt.Test) {
		ctx := testContext()
		token, err := kind.Generate(ctx, nil, map[string]string{"_x": "v"}, 0)
		assert.Loosely(t, token, should.BeEmpty)
		assert.Loosely(t, err, should.ErrLike("bad key"))
	})

	ftt.Run("Negative exp", t, func(t *ftt.Test) {
		ctx := testContext()
		token, err := kind.Generate(ctx, nil, nil, -time.Minute)
		assert.Loosely(t, token, should.BeEmpty)
		assert.Loosely(t, err, should.ErrLike("expiration can't be negative"))
	})

	ftt.Run("Unknown algo", t, func(t *ftt.Test) {
		ctx := testContext()
		k2 := kind
		k2.Algo = "unknown"
		token, err := k2.Generate(ctx, nil, nil, 0)
		assert.Loosely(t, token, should.BeEmpty)
		assert.Loosely(t, err, should.ErrLike("unknown algo"))
	})
}

func TestValidate(t *testing.T) {
	kind := TokenKind{
		Algo:       TokenAlgoHmacSHA256,
		Expiration: 30 * time.Minute,
		SecretKey:  "secret_key_name",
		Version:    1,
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := testContext()
		token, err := kind.Generate(ctx, []byte("state"), map[string]string{
			"key1": "value1",
			"key2": "value2",
		}, 0)
		assert.Loosely(t, err, should.BeNil)

		// Good state.
		embedded, err := kind.Validate(ctx, token, []byte("state"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, embedded, should.Match(map[string]string{
			"key1": "value1",
			"key2": "value2",
		}))

		// Bad state.
		embedded, err = kind.Validate(ctx, token, []byte("???"))
		assert.Loosely(t, err, should.ErrLike("bad token MAC"))

		// Not base64.
		embedded, err = kind.Validate(ctx, "?"+token[1:], []byte("state"))
		assert.Loosely(t, err, should.ErrLike("illegal base64 data"))

		// Corrupted.
		embedded, err = kind.Validate(ctx, "X"+token[1:], []byte("state"))
		assert.Loosely(t, err, should.ErrLike("bad token MAC"))

		// Too short.
		embedded, err = kind.Validate(ctx, token[:10], []byte("state"))
		assert.Loosely(t, err, should.ErrLike("too small"))

		// Make it expired by rolling time forward.
		tc := clock.Get(ctx).(testclock.TestClock)
		tc.Add(31 * time.Minute)
		embedded, err = kind.Validate(ctx, token, []byte("state"))
		assert.Loosely(t, err, should.ErrLike("token expired"))
	})

	ftt.Run("Custom expiration time", t, func(t *ftt.Test) {
		ctx := testContext()
		token, err := kind.Generate(ctx, nil, nil, time.Minute)
		assert.Loosely(t, err, should.BeNil)

		// Valid.
		_, err = kind.Validate(ctx, token, nil)
		assert.Loosely(t, err, should.BeNil)

		// No longer valid.
		tc := clock.Get(ctx).(testclock.TestClock)
		tc.Add(2 * time.Minute)
		_, err = kind.Validate(ctx, token, nil)
		assert.Loosely(t, err, should.ErrLike("token expired"))
	})

	ftt.Run("Unknown algo", t, func(t *ftt.Test) {
		ctx := testContext()
		k2 := kind
		k2.Algo = "unknown"
		_, err := k2.Validate(ctx, "token", nil)
		assert.Loosely(t, err, should.ErrLike("unknown algo"))
	})

	ftt.Run("Padding", t, func(t *ftt.Test) {
		// Produce tokens of various length to ensure base64 padding stripping
		// works.
		ctx := testContext()
		for i := 0; i < 10; i++ {
			data := map[string]string{
				"k": strings.Repeat("a", i),
			}
			token, err := kind.Generate(ctx, nil, data, 0)
			assert.Loosely(t, err, should.BeNil)
			extracted, err := kind.Validate(ctx, token, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, extracted, should.Match(data))
		}
	})
}

func testContext() context.Context {
	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, time.Unix(1444945245, 0))
	ctx = secrets.Use(ctx, &testsecrets.Store{})
	return ctx
}
