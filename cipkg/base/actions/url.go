// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package actions

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"go.chromium.org/luci/cipkg/core"
)

// ActionURLFetchTransformer is the default transformer for
// core.ActionURLFetch.
func ActionURLFetchTransformer(a *core.ActionURLFetch, deps []Package) (*core.Derivation, error) {
	return ReexecDerivation(a, false)
}

// ActionURLFetchExecutor is the default executor for core.ActionURLFetch.
func ActionURLFetchExecutor(ctx context.Context, a *core.ActionURLFetch, out string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Url, nil)
	if err != nil {
		return
	}

	joinErr := func(f func() error) { err = errors.Join(err, f()) }

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	if resp.StatusCode/100 > 3 {
		err = errors.New(resp.Status)
		return
	}
	defer joinErr(resp.Body.Close)

	f, err := os.Create(filepath.Join(out, "file"))
	if err != nil {
		return
	}
	defer joinErr(f.Close)

	h, err := hashFromEnum(a.HashAlgorithm)
	if err != nil {
		return
	}

	if _, err = io.Copy(f, io.TeeReader(resp.Body, h)); err != nil {
		return
	}

	if actual := fmt.Sprintf("%x", h.Sum(nil)); actual != a.HashValue {
		return fmt.Errorf("hash mismatch: expected: %s, actual: %s", a.HashValue, actual)
	}
	return
}

func hashFromEnum(h core.HashAlgorithm) (hash.Hash, error) {
	if h == core.HashAlgorithm_HASH_UNSPECIFIED {
		return &emptyHash{}, nil
	}

	switch h {
	case core.HashAlgorithm_HASH_MD5:
		return crypto.MD5.New(), nil
	case core.HashAlgorithm_HASH_SHA256:
		return crypto.SHA256.New(), nil
	}

	return nil, fmt.Errorf("unknown hash algorithm: %s", h)
}

// If no hash algorithm is provided, we will use an empty hash to skip the
// check and assume the artifact from the url is always same. This is only
// used for legacy content.
type emptyHash struct{}

func (h *emptyHash) Write(p []byte) (int, error) { return len(p), nil }
func (h *emptyHash) Sum(b []byte) []byte         { return []byte{} }
func (h *emptyHash) Reset()                      {}
func (h *emptyHash) Size() int                   { return 0 }
func (h *emptyHash) BlockSize() int              { return 0 }
