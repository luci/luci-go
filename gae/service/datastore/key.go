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

package datastore

import (
	"go.chromium.org/luci/gae/service/datastore/types"
)

// KeyTok is a single token from a multi-part Key.
type KeyTok = types.KeyTok

// KeyContext is the context in which a key is generated.
type KeyContext = types.KeyContext

// MkKeyContext is a helper function to create a new KeyContext.
//
// It is preferable to field-based struct initialization because, as a function,
// it has the ability to enforce an exact number of parameters.
func MkKeyContext(appID, namespace string) KeyContext {
	return types.MkKeyContext(appID, namespace)
}

// Key is the type used for all datastore operations.
type Key = types.Key

// NewKeyEncoded decodes and returns a *Key
func NewKeyEncoded(encoded string) (ret *Key, err error) {
	types.NewKeyEncoded(encoded)
	return
}
