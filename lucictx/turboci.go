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

package lucictx

import (
	"context"
)

// GetTurboCI returns the current TurboCI from LUCI_CONTEXT if it was present.
// nil, otherwise.
func GetTurboCI(ctx context.Context) *TurboCI {
	ret := TurboCI{}
	ok, err := Lookup(ctx, "turboci", &ret)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return &ret
}

// SetTurboCI sets the TurboCI in the LUCI_CONTEXT.
func SetTurboCI(ctx context.Context, tc *TurboCI) context.Context {
	return Set(ctx, "turboci", tc)
}
