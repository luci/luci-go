// Copyright 2022 The LUCI Authors.
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

// Package info facilitates adding global application info to a context.
package info

import (
	"context"
)

var contextKey = "info.ImageVersion"

func SetImageVersion(ctx context.Context, version string) context.Context {
	return context.WithValue(ctx, &contextKey, version)
}

func ImageVersion(ctx context.Context) string {
	return ctx.Value(&contextKey).(string)
}
