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

package lucictx

import (
	"context"
	"fmt"
	"strings"
)

// GetRealm returns the current "realm" section from LUCI_CONTEXT if it was
// present or nil otherwise.
func GetRealm(ctx context.Context) *Realm {
	ret := Realm{}
	ok, err := Lookup(ctx, "realm", &ret)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return &ret
}

// SetRealm sets the Realm in the LUCI_CONTEXT.
func SetRealm(ctx context.Context, r *Realm) context.Context {
	return Set(ctx, "realm", r)
}

// CurrentRealm grab the result of GetRealm and parses it into parts.
//
// Returns empty strings if there's no "realm" section in the LUCI_CONTEXT.
func CurrentRealm(ctx context.Context) (project, realm string) {
	r := GetRealm(ctx)
	if r == nil || r.Name == "" {
		return
	}
	idx := strings.IndexRune(r.Name, ':')
	if idx == -1 {
		panic(fmt.Sprintf("bad realm name %q in LUCI_CONTEXT - should be <project>:<realm>", r.Name))
	}
	return r.Name[:idx], r.Name[idx+1:]
}
