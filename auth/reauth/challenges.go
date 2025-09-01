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
	"fmt"

	"go.chromium.org/luci/common/errors"
)

func challengeHandlers(facetID string) map[string]challengeHandler {
	return map[string]challengeHandler{
		"SECURITY_KEY": chainHandler{
			handlers: []challengeHandler{
				newPluginHandler(facetID),
			},
		},
	}
}

// A challengeHandler handles a Reauth challenge.
type challengeHandler interface {
	// Checks if the handler is availablein the current execution environment
	// (e.g. are the necessary environment variables set).
	//
	// Returns `nil` if the handler is available.
	//
	// Otherwise, return an error explaining why the handler isn't available
	// (to help troubleshooting).
	CheckAvailable(context.Context) error

	Handle(context.Context, challenge) (*proposalReply, error)
}

type chainHandler struct {
	handlers []challengeHandler
}

func (h chainHandler) CheckAvailable(ctx context.Context) error {
	var handlerErrs []error
	for _, h := range h.handlers {
		if err := h.CheckAvailable(ctx); err != nil {
			handlerErrs = append(handlerErrs, fmt.Errorf("%T: %v", h, err))
			continue
		}
		return nil
	}
	return errors.Fmt("no available handler: %v", errors.Join(handlerErrs...))
}

func (h chainHandler) Handle(ctx context.Context, c challenge) (*proposalReply, error) {
	for _, h := range h.handlers {
		if err := h.CheckAvailable(ctx); err != nil {
			continue
		}
		return h.Handle(ctx, c)
	}
	return nil, errors.New("chainHandler: no handlers reported available")
}
