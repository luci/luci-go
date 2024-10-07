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

package base

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/bazelbuild/buildtools/build"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg"
	"go.chromium.org/luci/lucicfg/buildifier"
)

// ValidateParams contains parameters for Validate call.
type ValidateParams struct {
	Loader interpreter.Loader // represents the main package
	Source []string           // paths to lint, relative to the main package
	Output lucicfg.Output     // generated output files to validate
	Meta   lucicfg.Meta       // validation options (settable through Starlark)

	// LegacyConfigServiceClientFactory returns a HTTP client that is used to end
	// request to LUCI Config service.
	//
	// This is usually just subcommand.LegacyConfigServiceClient.
	LegacyConfigServiceClient LegacyConfigServiceClientFactory

	// ConfigServiceConn returns a gRPC connection that can be used to send
	// request to LUCI Config service.
	//
	// This is usually just subcommand.MakeConfigServiceConn.
	ConfigServiceConn ConfigServiceConnFactory
}

// LegacyConfigServiceClientFactory returns a HTTP client that is used to end
// request to LUCI Config service.
type LegacyConfigServiceClientFactory func(ctx context.Context) (*http.Client, error)

// ConfigServiceConnFactory returns a gRPC connection that can be used to send
// request to LUCI Config service.
type ConfigServiceConnFactory func(ctx context.Context, host string) (*grpc.ClientConn, error)

// Validate validates both input source code and generated config files.
//
// It is a common part of subcommands that validate configs.
//
// Source code is checked using buildifier linters and formatters, if enabled.
// This is controlled by LintChecks meta args.
//
// Generated config files are split into 0 or more config sets and sent to
// the LUCI Config remote service for validation, if enabled. This is controlled
// by ConfigServiceHost meta arg.
//
// Dumps all validation errors to the stderr. In addition to detailed validation
// results, also returns a multi-error with all blocking errors.
func Validate(ctx context.Context, params ValidateParams, getRewriterForPath func(path string) (*build.Rewriter, error)) ([]*buildifier.Finding, []*lucicfg.ValidationResult, error) {
	wg := sync.WaitGroup{}
	wg.Add(2)
	var localRes []*buildifier.Finding
	var localErr error

	go func() {
		defer wg.Done()
		localRes, localErr = buildifier.Lint(
			params.Loader,
			params.Source,
			params.Meta.LintChecks,
			getRewriterForPath,
		)
	}()

	var remoteRes []*lucicfg.ValidationResult
	var remoteErr error
	go func() {
		defer wg.Done()
		remoteRes, remoteErr = validateOutput(ctx,
			params.Output,
			params.LegacyConfigServiceClient,
			params.ConfigServiceConn,
			params.Meta.ConfigServiceHost,
			params.Meta.FailOnWarnings,
		)
	}()

	wg.Wait()

	first := true
	for _, r := range localRes {
		if text := r.Format(); text != "" {
			if first {
				fmt.Fprintf(os.Stderr, "--------------------------------------------\n")
				fmt.Fprintf(os.Stderr, "Formatting and linting errors\n")
				fmt.Fprintf(os.Stderr, "--------------------------------------------\n")
				first = false
			}
			fmt.Fprintf(os.Stderr, "%s", text)
		}
	}

	first = true
	for _, r := range remoteRes {
		if text := r.Format(); text != "" {
			if first {
				fmt.Fprintf(os.Stderr, "--------------------------------------------\n")
				fmt.Fprintf(os.Stderr, "LUCI Config validation errors\n")
				fmt.Fprintf(os.Stderr, "--------------------------------------------\n")
				first = false
			}
			fmt.Fprintf(os.Stderr, "%s", text)
		}
	}

	var merr errors.MultiError
	merr = mergeMerr(merr, localErr)
	merr = mergeMerr(merr, remoteErr)
	if len(merr) != 0 {
		return localRes, remoteRes, merr
	}
	return localRes, remoteRes, nil
}

// mergeMerr adds errs to merr returning new merr.
func mergeMerr(merr errors.MultiError, err error) errors.MultiError {
	if err == nil {
		return merr
	}
	if many, ok := err.(errors.MultiError); ok {
		return append(merr, many...)
	}
	return append(merr, err)
}

// validateOutput splits the output into 0 or more config sets and sends them
// for validation to LUCI Config.
func validateOutput(ctx context.Context, output lucicfg.Output,
	legacyClientFactory LegacyConfigServiceClientFactory,
	clientConnFactory ConfigServiceConnFactory,
	host string, failOnWarns bool) ([]*lucicfg.ValidationResult, error) {
	configSets, err := output.ConfigSets()
	if len(configSets) == 0 || err != nil {
		return nil, err // nothing to validate or failed to serialize
	}

	// Log the warning only if there were some config sets we needed to validate.
	if host == "" {
		logging.Warningf(ctx, "Config service host is not set, skipping validation against LUCI Config service")
		return nil, nil
	}

	conn, err := clientConnFactory(ctx, host)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logging.Warningf(ctx, "failed to close the connection to config service: %s", err)
		}
	}()
	validator := lucicfg.NewRemoteValidator(conn)

	// Validate all config sets in parallel.
	results := make([]*lucicfg.ValidationResult, len(configSets))
	wg := sync.WaitGroup{}
	wg.Add(len(configSets))
	for i, cs := range configSets {
		i, cs := i, cs
		go func() {
			results[i] = cs.Validate(ctx, validator)
			wg.Done()
		}()
	}
	wg.Wait()

	// Assemble the final verdict. Note that OverallError mutates r.Failed.
	var merr errors.MultiError
	for _, r := range results {
		if err := r.OverallError(failOnWarns); err != nil {
			merr = append(merr, err)
		}
	}

	if len(merr) != 0 {
		return results, merr
	}
	return results, nil
}
