// Copyright 2019 The LUCI Authors.
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

package spanutil

import (
	"context"

	"cloud.google.com/go/spanner"
)

var clientCtxKey = "context key for a *spanner.Client"

// WithClient returns a context with the client embedded.
func WithClient(ctx context.Context, client *spanner.Client) context.Context {
	return context.WithValue(ctx, &clientCtxKey, client)
}

// Client retrieves the current spanner client from the context.
func Client(ctx context.Context) *spanner.Client {
	client, ok := ctx.Value(&clientCtxKey).(*spanner.Client)
	if !ok {
		panic("no Spanner client in context")
	}
	return client
}
