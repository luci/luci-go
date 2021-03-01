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

package common

import (
	"context"
)

var devContextKey = "cv/internal/common:dev"

// IsDev returns true if this CV is -dev.
//
// Defaults to false.
func IsDev(ctx context.Context) bool {
	v, exists := ctx.Value(&devContextKey).(bool)
	if !exists {
		return false
	}
	return v
}

// SetDev returns context marking this CV as -dev.
func SetDev(ctx context.Context) context.Context {
	return context.WithValue(ctx, &devContextKey, true)
}
