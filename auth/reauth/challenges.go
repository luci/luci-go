// Copyright 2024 The LUCI Authors.
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

package reauth

import (
	"context"

	"go.chromium.org/luci/common/errors"
)

func challengeHandlers(facetID string) map[string]challengeHandler {
	return map[string]challengeHandler{
		"SECURITY_KEY": chainHandler{
			handlers: []challengeHandler{},
		},
	}
}

// A challengeHandler handles a Reauth challenge.
type challengeHandler interface {
	IsAvailable() bool
	Handle(context.Context, challenge) (*proposalReply, error)
}

type chainHandler struct {
	handlers []challengeHandler
}

func (h chainHandler) IsAvailable() bool {
	for _, h := range h.handlers {
		if h.IsAvailable() {
			return true
		}
	}
	return false
}

func (h chainHandler) Handle(ctx context.Context, c challenge) (*proposalReply, error) {
	for _, h := range h.handlers {
		if !h.IsAvailable() {
			continue
		}
		return h.Handle(ctx, c)
	}
	return nil, errors.Reason("chainHandler: no handlers reported available").Err()
}
