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

package exe

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/client/butlerlib/bootstrap"
)

var (
	buildKey        = "holds a *build"
	namespaceKey    = "holds the current step namespace as a string"
	logdogClientKey = "holds a *bootstrap.Bootstrap"
	propertyNSKey   = "holds a []string"
)

func getNS(ctx context.Context) string {
	ret, _ := ctx.Value(&namespaceKey).(string)
	return ret
}

func withNS(ctx context.Context, ns string) context.Context {
	return context.WithValue(ctx, &namespaceKey, ns)
}

func getBuild(ctx context.Context) *Build {
	ret, ok := ctx.Value(&buildKey).(*Build)
	if !ok {
		panic(errors.New("no *Build in context"))
	}
	return ret
}

func setBuild(ctx context.Context, b *Build) context.Context {
	return context.WithValue(ctx, &buildKey, b)
}

func getLogdogClient(ctx context.Context) *bootstrap.Bootstrap {
	ret, _ := ctx.Value(&logdogClientKey).(*bootstrap.Bootstrap)
	return ret
}

func setLogdogClient(ctx context.Context, ldc *bootstrap.Bootstrap) context.Context {
	return context.WithValue(ctx, &logdogClientKey, ldc)
}

func getPropertyNS(ctx context.Context) []string {
	curNS, ok := ctx.Value(&propertyNSKey).([]string)
	if !ok {
		return nil
	}
	return append([]string(nil), curNS...)
}

func addPropertyNS(ctx context.Context, toks []string) context.Context {
	if len(toks) == 0 {
		return ctx
	}

	return context.WithValue(ctx, &propertyNSKey, append(getPropertyNS(ctx), toks...))
}
