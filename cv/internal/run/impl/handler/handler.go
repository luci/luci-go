// Copyright 2021 The LUCI Authors.
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

package handler

import (
	"context"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// Handler is an interface that handles events that RunManager receives.
type Handler interface {
	// Start starts a Run.
	Start(context.Context, *state.RunState) (eventbox.SideEffectFn, *state.RunState, error)

	// Cancel cancels a Run.
	Cancel(context.Context, *state.RunState) (eventbox.SideEffectFn, *state.RunState, error)

	// OnCLUpdated decides whether to cancel a Run based on changes to the CLs.
	OnCLUpdated(context.Context, *state.RunState, common.CLIDs) (eventbox.SideEffectFn, *state.RunState, error)

	// OnCQDVerificationCompleted finalizes the Run according to the verified
	// Run reported by CQDaemon.
	OnCQDVerificationCompleted(context.Context, *state.RunState) (eventbox.SideEffectFn, *state.RunState, error)
}

// Impl is a prod implementation of Handler interface.
type Impl struct{}

var _ Handler = &Impl{}
