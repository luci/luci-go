// Copyright 2020 The LUCI Authors.
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

package secrets

import (
	"context"
	"encoding/json"
	"os"

	"go.chromium.org/luci/common/errors"
)

// Source knows how to fetch (and refetch) a secret from some fixed location.
type Source interface {
	// ReadSecret returns the most recent value of the secret in the source.
	ReadSecret(context.Context) (*Secret, error)
	// Close releases any allocated resources.
	Close() error
}

// StaticSource is a Source that returns the same static secret all the time.
type StaticSource struct {
	Secret *Secret
}

// ReadSecret is part of Source interface.
func (s *StaticSource) ReadSecret(context.Context) (*Secret, error) {
	return s.Secret, nil
}

// Close is part of Source interface.
func (s *StaticSource) Close() error {
	return nil
}

// FileSource reads the secret from a JSON file on disk.
type FileSource struct {
	Path string
}

// ReadSecret is part of Source interface.
func (s *FileSource) ReadSecret(context.Context) (*Secret, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	secret := &Secret{}
	if err := json.NewDecoder(f).Decode(secret); err != nil {
		return nil, errors.Annotate(err, "not a valid JSON").Err()
	}
	if len(secret.Current) == 0 {
		return nil, errors.Reason("`current` field in the root secret is empty, this is not allowed").Err()
	}
	return secret, nil
}

// Close is part of Source interface.
func (s *FileSource) Close() error {
	return nil
}
