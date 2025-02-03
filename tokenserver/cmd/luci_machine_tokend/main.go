// Copyright 2016 The LUCI Authors.
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

// Command luci_machine_tokend runs on all machines via cron.
//
// It wakes up each ~10 min, checks whether it needs to refresh existing machine
// token, and refreshes it if necessary.
//
// It also dumps information about its run into a status file (as JSON), that
// can be picked up sysmon and transformed into ts_mon metrics (most important
// one being "time since last successful token refresh").
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/logging/teelogger"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/target"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/client"
)

// Version identifies the major revision of the tokend code.
//
// It is put in the status file (and subsequently reported to monitoring).
const Version = "1.3"

// commandLine contains all command line flags.
//
// See registerFlags() for description of each individual flag.
type commandLine struct {
	PrivateKeyPath  string
	CertificatePath string
	Backend         string
	TokenFile       string
	TokenFileCopy   stringlistflag.Flag
	StatusFile      string
	Timeout         time.Duration
	ForceRefresh    bool
}

func defaults() commandLine {
	return commandLine{
		Timeout: 60 * time.Second,
	}
}

func (c *commandLine) registerFlags(f *flag.FlagSet) {
	f.StringVar(&c.PrivateKeyPath, "pkey-pem", c.PrivateKeyPath, "path to a private key file")
	f.StringVar(&c.CertificatePath, "cert-pem", c.CertificatePath, "path to a certificate file")
	f.StringVar(&c.Backend, "backend", c.Backend, "hostname of the backend to use")
	f.StringVar(&c.TokenFile, "token-file", c.TokenFile, "where to put the token file")
	f.Var(&c.TokenFileCopy, "token-file-copy", "additional file path to copy the token file into, can be repeated")
	f.StringVar(&c.StatusFile, "status-file", c.StatusFile, "where to put details about this run (optional)")
	f.DurationVar(&c.Timeout, "timeout", c.Timeout, "how long to retry on errors before giving up")
	f.BoolVar(&c.ForceRefresh, "force-refresh", c.ForceRefresh, "forcefully refresh the token even if it is still valid")
}

func (c *commandLine) check() error {
	if c.PrivateKeyPath == "" {
		return fmt.Errorf("-pkey-pem is required")
	}
	if c.CertificatePath == "" {
		return fmt.Errorf("-cert-pem is required")
	}
	if c.Backend == "" {
		return fmt.Errorf("-backend is required")
	}
	if c.TokenFile == "" {
		return fmt.Errorf("-token-file is required")
	}
	return nil
}

func main() {
	os.Exit(realMain(context.Background(), os.Args[1:]))
}

func realMain(ctx context.Context, args []string) int {
	flags := flag.NewFlagSet("", flag.ExitOnError)

	opts := defaults()
	opts.registerFlags(flags)

	tsmonFlags := tsmon.NewFlags()
	tsmonFlags.Target.TargetType = target.TaskType
	tsmonFlags.Target.TaskServiceName = "luci_machine_tokend"
	tsmonFlags.Target.TaskJobName = "default"
	tsmonFlags.Flush = "manual"
	tsmonFlags.Register(flags)

	// This exits on errors.
	_ = flags.Parse(args)

	if err := opts.check(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		flags.Usage()
		return 2
	}

	clientParams := client.Parameters{
		PrivateKeyPath:  opts.PrivateKeyPath,
		CertificatePath: opts.CertificatePath,
		Backend:         opts.Backend,
		Retry: func() retry.Iterator {
			return &retry.ExponentialBackoff{
				Limited: retry.Limited{
					Delay:   200 * time.Millisecond,
					Retries: 100000, // limit only by time, not number of retries
				},
				MaxDelay:   opts.Timeout,
				Multiplier: 1.5,
			}
		},
	}
	if strings.HasPrefix(clientParams.Backend, "localhost:") || strings.HasPrefix(clientParams.Backend, "127.0.0.1:") {
		clientParams.Insecure = true
	}

	log := &memlogger.MemLogger{}

	// Write Debug log to both memlogger and gologger.
	memLogFactory := func(context.Context, *logging.LogContext) logging.Logger {
		return log
	}
	root := teelogger.Use(ctx, memLogFactory, gologger.StdConfig.NewLogger)
	root = logging.SetLevel(root, logging.Debug)

	// Apply tsmon config. A failure here is non-fatal.
	if err := tsmon.InitializeFromFlags(root, &tsmonFlags); err != nil {
		logging.Errorf(root, "Failed to initialize tsmon - %s", err)
	}

	ctx, cancel := context.WithTimeout(root, opts.Timeout)
	defer cancel()
	signals.HandleInterrupt(cancel)

	statusReport := StatusReport{
		Version: Version,
		Started: clock.Now(ctx),
	}
	defer func() {
		// Dump the status of this run. It's picked up by monitoring. Ignore errors
		// here, they are not important compared to 'run' errors. Use root context
		// to be to flush errors to monitoring even if 'ctx' has expired.
		statusReport.Finished = clock.Now(ctx)
		if err := statusReport.SendMetrics(root); err != nil {
			logging.Errorf(root, "Failed to send tsmon metrics - %s", err)
		}
		if opts.StatusFile != "" {
			if err := statusReport.SaveToFile(root, log, opts.StatusFile); err != nil {
				logging.Errorf(root, "Failed to save the status - %s", err)
			}
		}
	}()
	if err := run(ctx, clientParams, opts, &statusReport); err != nil {
		return 1
	}
	return 0
}

func run(ctx context.Context, clientParams client.Parameters, opts commandLine, status *StatusReport) error {
	// Read existing token file on disk to check whether we really need to update
	// it. We update the token if it is missing, close to expiration, or when
	// parameters change.
	existingToken, existingState := readTokenFile(ctx, opts.TokenFile)

	// Record the info about existing token in status report, it is useful even if
	// we fail to refresh the token.
	status.LastToken = existingToken

	// Initialize the client. It will read private key and certificate file into
	// memory and validate them.
	cl, err := client.New(clientParams)
	if err != nil {
		logging.Errorf(ctx, "Failed to initialize the client - %s", err)
		status.FailureError = err
		status.UpdateOutcome = OutcomeCantReadKey
		// Fill in some update reason to avoid "" as metric value.
		if existingToken.NextUpdate == 0 {
			status.UpdateReason = UpdateReasonNewToken
		} else {
			// We successfully updated the token in the past, but now the keys are
			// suddenly unreadable, they probably changed.
			status.UpdateReason = UpdateReasonParametersChange
		}
		return err
	}

	// Generate a hash of all input parameters. It is used to detect that we
	// need to refresh the token file even if the token is still valid. It
	// happens if we change a key or backend URL.
	signer := cl.Signer.(*client.X509Signer)
	inputsDigest := calcDigest(map[string][]byte{
		"forceBump": {1}, // bump this to forcefully regenerate all tokens
		"pkey":      signer.PrivateKeyPEM,
		"cert":      signer.CertificatePEM,
		"backend":   []byte(clientParams.Backend),
	})

	// Record a reason for token update (if we need to update the token).
	now := clock.Now(ctx)
	switch {
	case existingToken.NextUpdate == 0:
		status.UpdateReason = UpdateReasonNewToken
	case now.After(time.Unix(existingToken.NextUpdate, 0)):
		status.UpdateReason = UpdateReasonExpiration
	case existingState.InputsDigest != inputsDigest:
		status.UpdateReason = UpdateReasonParametersChange
	case opts.ForceRefresh:
		status.UpdateReason = UpdateReasonForceRefresh
	default:
		logging.Infof(ctx, "The token is valid, skipping the update")
		// Make sure all copies are up-to-date as well.
		switch wrote, err := writeTokenFiles(ctx, existingToken, existingState, opts.TokenFileCopy); {
		case err != nil:
			status.UpdateReason = UpdateReasonMissingTokenCopy
			if os.IsPermission(err) {
				status.UpdateOutcome = OutcomePermissionError
			} else {
				status.UpdateOutcome = OutcomeUnknownSaveTokenError
			}
			return err
		case wrote:
			status.UpdateReason = UpdateReasonMissingTokenCopy
		default:
			status.UpdateReason = UpdateReasonTokenIsGood
		}
		status.UpdateOutcome = OutcomeTokenIsGood
		return nil
	}

	// Grab a new token. MintMachineToken does retries internally, until success
	// or context deadline.
	resp, err := cl.MintMachineToken(ctx, &minter.MachineTokenRequest{
		TokenType: tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
	})
	status.MintTokenDuration = clock.Now(ctx).Sub(now)
	if err != nil {
		logging.Errorf(ctx, "Failed to generate a new token - %s", err)
		status.FailureError = err
		status.UpdateOutcome = OutcomeFromRPCError(err)
		if details, ok := err.(client.RPCError); ok {
			status.ServiceVersion = details.ServiceVersion
		}
		return err
	}
	status.ServiceVersion = resp.ServiceVersion

	// Grab machine_token field.
	var tok *minter.LuciMachineToken
	if tt, _ := resp.TokenType.(*minter.MachineTokenResponse_LuciMachineToken); tt != nil {
		tok = tt.LuciMachineToken
	}
	if tok == nil {
		err = fmt.Errorf("bad response, empty luci_machine_token field")
		logging.Errorf(ctx, "%s", err)
		status.FailureError = err
		status.UpdateOutcome = OutcomeMalformedReponse
		return err
	}

	now = clock.Now(ctx)
	expiry := tok.Expiry.AsTime()
	lifetime := expiry.Sub(now)

	// lifetime should usually be 1h, add a safeguard to avoid hammering
	// the backend in case the lifetime is unexpectedly wrong.
	if lifetime < 5*time.Minute {
		logging.Warningf(ctx, "Returned token lifetime is unexpectedly too short (%s)", lifetime)
		lifetime = 5 * time.Minute
	}

	// We start to attempt to refresh the token after half of its lifetime has
	// passed, to be able survive short (~30 min) backend outages in exchange for
	// 2x RPC rate.
	newTokenFile := tokenserver.TokenFile{
		LuciMachineToken: tok.MachineToken,
		Expiry:           expiry.Unix(),
		LastUpdate:       now.Unix(),
		NextUpdate:       now.Add(lifetime / 2).Unix(),
	}
	newState := stateInToken{
		InputsDigest: inputsDigest,
		Version:      Version,
	}

	pathsToWrite := append([]string{opts.TokenFile}, opts.TokenFileCopy...)
	if _, err = writeTokenFiles(ctx, &newTokenFile, &newState, pathsToWrite); err != nil {
		status.FailureError = err
		if os.IsPermission(err) {
			status.UpdateOutcome = OutcomePermissionError
		} else {
			status.UpdateOutcome = OutcomeUnknownSaveTokenError
		}
		return err
	}

	status.LastToken = &newTokenFile
	status.UpdateOutcome = OutcomeUpdateSuccess
	return nil
}

// calcDigest produces a digest of a given map using some stable serialization.
func calcDigest(inputs map[string][]byte) string {
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha1.New()
	for _, k := range keys {
		v := inputs[k]
		fmt.Fprintf(h, "%s\n%d\n", k, len(v))
		h.Write(v)
	}
	blob := h.Sum(nil)
	return hex.EncodeToString(blob[:])
}
