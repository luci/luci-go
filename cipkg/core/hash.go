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

package core

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"hash"
	"strings"

	"go.chromium.org/luci/common/proto"
)

// GetDerivationID calculates a unique ID from derivation's content and can
// represent its corresponding output content. The DerivationID is safe for
// filesystem path and other common applications.
func GetDerivationID(drv *Derivation) (string, error) {
	h := sha256.New()
	if err := hashDerivation(h, drv); err != nil {
		return "", err
	}

	// - "name-hash" for usual derivations.
	// - "name+hash" for derivations with fixed output.
	infix := "-"
	if drv.FixedOutput != "" {
		infix = "+"
	}

	// We want to keep the hash as short as possible to avoid reaching the path
	// length limit on windows.
	// Using base32 instead of base64 because filesystem is not promised to be
	// case-sensitive.
	enc := base32.HexEncoding.WithPadding(base32.NoPadding)
	return drv.Name + infix + strings.ToLower(enc.EncodeToString(h.Sum(nil)[:16])), nil
}

func hashDerivation(h hash.Hash, drv *Derivation) error {
	if len(drv.FixedOutput) != 0 {
		_, err := h.Write([]byte(drv.FixedOutput))
		return err
	}
	return proto.StableHash(h, drv)
}

// GetActionID calculates a unique ID from action's content and can represent
// package abstraction. Different actions may generate same derivation and
// share underlying content. ActionID isn't safe to be used as filesystem path.
func GetActionID(a *Action) (string, error) {
	h := sha256.New()
	if err := proto.StableHash(h, a); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%x", a.Name, h.Sum(nil)), nil
}
