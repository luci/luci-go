// Copyright 2025 The LUCI Authors.
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

// Package vsa implement a client to interact with VerifySoftwareArtifact
// endpoint and logged calls to BigQuery table.
package vsa

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/bqlog"

	cipdapi "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/vsa/api"
	"go.chromium.org/luci/cipd/common"
)

func init() {
	bqlog.RegisterSink(bqlog.Sink{
		Prototype: &api.VerifySoftwareArtifactLogEntry{},
		Table:     "vsa_log",
	})
}

type Client interface {
	// Register registers vsa client configs as CLI flags.
	Register(f *flag.FlagSet)

	// Init prepares the vsa client based on the parsed configs. It should only be
	// called exactly once.
	Init(ctx context.Context) error

	// VerifySoftwareArtifact calls vsa VerifySoftwareArtifact API with the bundle
	// provided. It returns the Verification Summary Attestation (VSA) if succeeded.
	// Errors and detailed logs will be recorded in BigQuery Table.
	VerifySoftwareArtifact(ctx context.Context, inst *model.Instance, bundle string) (vsa string)
}

// NewClient creates a vsa.Client for invoking verifier API. It needs to
// call Init() after arguments parsed.
func NewClient() Client {
	return &client{}
}

// Client is a vsa client for invoking verifier API.
type client struct {
	softwareVerifierHost string
	resourcePrefix       string

	softwareVerifierUrl *url.URL

	client *http.Client
	bqlog  func(ctx context.Context, m proto.Message)
}

// Register registers vsa client configs as CLI flags.
func (c *client) Register(f *flag.FlagSet) {
	f.StringVar(
		&c.softwareVerifierHost,
		"software-verifier-host",
		c.softwareVerifierHost,
		"The host of the VerifySoftwareArtifact API. If empty, VerifySoftwareArtifact will not be called.",
	)
	f.StringVar(
		&c.resourcePrefix,
		"slsa-resource-prefix",
		c.resourcePrefix,
		"The prefix for slsa resource uri. e.g. cipd_package://chrome-infra-packages.appspot.com",
	)
}

// Init prepares the vsa client based on the parsed configs. It should only be
// called exactly once.
func (c *client) Init(ctx context.Context) error {
	logging.Debugf(ctx, "Initializing vsa client: %v", c)
	if c.softwareVerifierHost == "" {
		if c.resourcePrefix != "" {
			return fmt.Errorf("-slsa-resource-prefix must be set with -software-verifier-host")
		}
		return nil
	}

	// Ensure the prefix ends with "/".
	if !strings.HasSuffix(c.resourcePrefix, "/") {
		return fmt.Errorf("bad -slsa-resource-prefix: must ends with /")
	}
	// Normalize the host.
	u, err := url.Parse(c.softwareVerifierHost)
	if err != nil {
		return fmt.Errorf("bad -software-verifier-host: %w", err)
	}
	// Parse the api endpoint.
	u, err = url.Parse(fmt.Sprintf("https://%s/v1/software-artifact-verification-requests", u.Host))
	if err != nil {
		return fmt.Errorf("bad -software-verifier-host: %w", err)
	}
	c.softwareVerifierUrl = u

	if c.client == nil {
		tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(
			"https://www.googleapis.com/auth/cloud-platform",
			"https://www.googleapis.com/auth/bcid_verify",
		))
		if err != nil {
			return fmt.Errorf("failed to get authenticating transport: %w", err)
		}
		c.client = &http.Client{Transport: tr}
	}
	if c.bqlog == nil {
		c.bqlog = bqlog.Log
	}
	logging.Debugf(ctx, "Initialized vsa client: %v", c)
	return nil
}

// VerifySoftwareArtifact calls VerifySoftwareArtifact API with the attestation
// bundle provided, see:
// https://github.com/in-toto/attestation/blob/main/spec/v1/bundle.md
// It returns the Verification Summary Attestation (VSA) if succeeded, see:
// https://slsa.dev/spec/v0.1/verification_summary
//
// Errors and detailed logs will be recorded in BigQuery Table.
func (c *client) VerifySoftwareArtifact(ctx context.Context, inst *model.Instance, bundle string) string {
	if c.softwareVerifierHost == "" {
		return ""
	}

	uri := c.resourcePrefix + inst.Package.StringID()
	logEntry := &api.VerifySoftwareArtifactLogEntry{
		Package:     inst.Package.StringID(),
		Instance:    inst.InstanceID,
		ResourceUri: uri,
		Timestamp:   clock.Now(ctx).UnixMicro(),
	}
	defer c.bqlog(ctx, logEntry)

	digests := make(map[string]string)
	switch obj := common.InstanceIDToObjectRef(inst.InstanceID); obj.HashAlgo {
	case cipdapi.HashAlgo_SHA1:
		digests["sha1"] = obj.HexDigest
	case cipdapi.HashAlgo_SHA256:
		digests["sha256"] = obj.HexDigest
	}
	digests["cipd_instance_id"] = inst.InstanceID

	req := &api.VerifySoftwareArtifactRequest{
		ResourceToVerify: uri,
		ArtifactInfo: &api.ArtifactInfo{
			Attestations: []string{bundle},
			Artifacts:    []*api.ArtifactInfo_ArtifactDescriptor{{Digests: digests}},
		},
		Context: &api.VerificationContext{
			EnforcementPointName: "cipd",
			VerificationPurpose:  api.VerificationContext_VERIFY_FOR_LOGGING,
			OccurrenceStage:      api.VerificationContext_OBSERVED,
		},
	}
	logging.Debugf(ctx, "vsa request: %v", req)

	resp, err := c.callVerifySoftwareArtifact(ctx, req)
	if err != nil {
		logEntry.ErrorMessage = err.Error()
		return ""
	}
	logEntry.Allowed = resp.Allowed
	logEntry.RejectionMessage = resp.RejectionMessage

	return resp.VerificationSummary
}

func (c *client) callVerifySoftwareArtifact(ctx context.Context, r *api.VerifySoftwareArtifactRequest) (*api.VerifySoftwareArtifactResponse, error) {
	b, err := protojson.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("VerifySoftwareArtifact: marshaling: failed to marshal request: %w: %s", err, r)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.softwareVerifierUrl.String(), bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("VerifySoftwareArtifact: new request: failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("VerifySoftwareArtifact: sending: failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if b, err = io.ReadAll(resp.Body); err != nil {
		return nil, fmt.Errorf("VerifySoftwareArtifact: reading response: failed to read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VerifySoftwareArtifact: bad response status: api returns error: %s: %s", resp.Status, string(b))
	}

	logging.Debugf(ctx, "vsa resp: %v", resp)
	var ret api.VerifySoftwareArtifactResponse
	if err := protojson.Unmarshal(b, &ret); err != nil {
		return nil, fmt.Errorf("VerifySoftwareArtifact: unmarshalling: failed to unmarshal response: %w: %s", err, string(b))
	}
	return &ret, nil
}
