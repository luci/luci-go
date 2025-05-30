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

// Package internalcontext contains a function to store custom metrics in context.
package internalcontext

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// WithCustomMetrics creates a CustomMetrics with dedicated tsmon.State to
// register custom metrics from service config, and store it in context.
//
// Placing this function in metrics package would cause an import loop.
//
// This function should only be called at server start up.
func WithCustomMetrics(ctx context.Context) (context.Context, error) {
	var globalCfg *pb.SettingsCfg
	err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		var err error
		globalCfg, err = config.GetSettingsCfg(ctx)
		return err
	}, nil)
	if err != nil {
		return ctx, errors.Fmt("failed to get service config when register custom metrics: %w", err)
	}
	return metrics.WithCustomMetrics(ctx, globalCfg)
}
