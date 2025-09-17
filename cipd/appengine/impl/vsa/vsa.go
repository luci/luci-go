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
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/bqlog"
	"go.chromium.org/luci/server/caching"

	cipdapi "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/repo/tasks"
	"go.chromium.org/luci/cipd/appengine/impl/vsa/api"
	"go.chromium.org/luci/cipd/common"
)

func init() {
	bqlog.RegisterSink(bqlog.Sink{
		Prototype: &api.VerifySoftwareArtifactLogEntry{},
		Table:     "vsa_log",
	})
}

type CacheStatus string

const (
	CacheStatusUnknown   CacheStatus = "Unknown"
	CacheStatusPending   CacheStatus = "Pending"
	CacheStatusCompleted CacheStatus = "Completed"
)

type Client interface {
	// Register registers vsa client configs as CLI flags.
	Register(f *flag.FlagSet)

	// Init prepares the vsa client based on the parsed configs. It should only be
	// called exactly once.
	Init(ctx context.Context) error

	// VerifySoftwareArtifact calls vsa VerifySoftwareArtifact API with the bundle
	// provided. It returns the Verification Summary Attestation (VSA) if succeeded.
	// Errors and detailed logs will be recorded in BigQuery Table.
	// This is equivalent to use NewVerifySoftwareArtifactTask create a task then
	// invoke CallVerifySoftwareArtifact immediately.
	VerifySoftwareArtifact(ctx context.Context, inst *model.Instance, bundle string) string

	// NewVerifySoftwareArtifactTask creates a task for VerifySoftwareArtifact
	// which could be processed by CallVerifySoftwareArtifact.
	NewVerifySoftwareArtifactTask(ctx context.Context, inst *model.Instance, bundle string) *tasks.CallVerifySoftwareArtifact

	// CallVerifySoftwareArtifact calls vsa VerifySoftwareArtifact API with the
	// task created by NewVerifySoftwareArtifactTask. Errors and detailed logs
	// will be recorded in BigQuery Table.
	CallVerifySoftwareArtifact(ctx context.Context, t *tasks.CallVerifySoftwareArtifact) string

	// Get the vsa status (Unknown, Pending, Completed) for the instance.
	GetStatus(ctx context.Context, inst *model.Instance) (CacheStatus, error)

	// Set the vsa status (Unknown, Pending, Completed) for the instance.
	// Depending on the status they will be cached for a period of time.
	SetStatus(ctx context.Context, inst *model.Instance, status CacheStatus) error
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
	cache  caching.BlobCache
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

	if c.cache == nil {
		c.cache = caching.GlobalCache(ctx, "vsa")
	}

	if c.bqlog == nil {
		c.bqlog = bqlog.Log
	}
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
	t := c.NewVerifySoftwareArtifactTask(ctx, inst, bundle)
	if t == nil {
		return ""
	}
	return c.CallVerifySoftwareArtifact(ctx, t)
}

// NewVerifySoftwareArtifactTask creates a task for VerifySoftwareArtifact
// with the bundle provided, see:
// https://github.com/in-toto/attestation/blob/main/spec/v1/bundle.md
func (c *client) NewVerifySoftwareArtifactTask(ctx context.Context, inst *model.Instance, bundle string) *tasks.CallVerifySoftwareArtifact {
	if c.softwareVerifierHost == "" {
		return nil
	}

	uri := c.resourcePrefix + inst.Package.StringID()
	logEntry := &api.VerifySoftwareArtifactLogEntry{
		Package:     inst.Package.StringID(),
		Instance:    inst.InstanceID,
		ResourceUri: uri,
		Timestamp:   clock.Now(ctx).UnixMicro(),
	}

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
			OccurrenceStage:      api.VerificationContext_AS_VERIFIED,
		},
	}

	return &tasks.CallVerifySoftwareArtifact{
		Instance: inst.Proto(),
		Request:  req,
		Log:      logEntry,
	}
}

// CallVerifySoftwareArtifact calls vsa VerifySoftwareArtifact API with the
// task created by NewVerifySoftwareArtifactTask.
// It returns the Verification Summary Attestation (VSA) if succeeded, see:
// https://slsa.dev/spec/v0.1/verification_summary
//
// Errors and detailed logs will be recorded in BigQuery Table.
func (c *client) CallVerifySoftwareArtifact(ctx context.Context, t *tasks.CallVerifySoftwareArtifact) string {
	defer c.bqlog(ctx, t.Log)
	resp, err := c.callVerifySoftwareArtifact(ctx, t.Request)
	if err != nil {
		t.Log.ErrorMessage = err.Error()
		return ""
	}
	t.Log.Allowed = resp.Allowed
	t.Log.RejectionMessage = resp.RejectionMessage

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

	var ret api.VerifySoftwareArtifactResponse
	if err := protojson.Unmarshal(b, &ret); err != nil {
		return nil, fmt.Errorf("VerifySoftwareArtifact: unmarshalling: failed to unmarshal response: %w: %s", err, string(b))
	}
	return &ret, nil
}

func cacheKey(inst *model.Instance) string { return inst.InstanceID + ":vsa_status" }

func (c *client) GetStatus(ctx context.Context, inst *model.Instance) (CacheStatus, error) {
	if c.cache == nil {
		return CacheStatusUnknown, nil
	}

	b, err := c.cache.Get(ctx, cacheKey(inst))
	if err != nil {
		if errors.Is(err, caching.ErrCacheMiss) {
			return CacheStatusUnknown, nil
		}
		return CacheStatusUnknown, err
	}

	status := CacheStatus(b)
	switch status {
	case CacheStatusUnknown:
	case CacheStatusPending:
	case CacheStatusCompleted:
	default:
		return CacheStatusUnknown, fmt.Errorf("VerifySoftwareArtifact: get cache: unknown cache status: %s", status)
	}
	return status, nil
}

func (c *client) SetStatus(ctx context.Context, inst *model.Instance, status CacheStatus) error {
	if c.cache == nil {
		return nil
	}

	var ttl time.Duration
	switch status {
	case CacheStatusUnknown:
		ttl = time.Second // Clear existing status
	case CacheStatusPending:
		ttl = time.Second * 30
	case CacheStatusCompleted:
		ttl = time.Minute * 5
	default:
		return fmt.Errorf("VerifySoftwareArtifact: set cache: unknown cache status: %s", status)
	}
	if err := c.cache.Set(ctx, cacheKey(inst), []byte(status), ttl); err != nil {
		return fmt.Errorf("VerifySoftwareArtifact: set cache: failed to set: %w", err)
	}
	return nil
}
