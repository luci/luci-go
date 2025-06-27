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

package delegation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/delegation/messages"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/identityset"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/revocation"
)

// tokenIDSequenceKind defines the namespace of int64 IDs for delegation tokens.
//
// Changing it will effectively reset the ID generation.
const tokenIDSequenceKind = "delegationTokenID"

// MintDelegationTokenRPC implements TokenMinter.MintDelegationToken RPC method.
type MintDelegationTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is the default server signer that uses server's service account.
	Signer signing.Signer

	// Rules returns delegation rules to use for the request.
	//
	// In prod it is GlobalRulesCache.Rules.
	Rules func(context.Context) (*Rules, error)

	// LogToken is mocked in tests.
	//
	// In prod it is produced by NewTokenLogger.
	LogToken TokenLogger

	// mintMock call is used in tests.
	//
	// In prod it is 'mint'
	mintMock func(context.Context, *mintParams) (*minter.MintDelegationTokenResponse, error)
}

// MintDelegationToken generates a new bearer delegation token.
func (r *MintDelegationTokenRPC) MintDelegationToken(c context.Context, req *minter.MintDelegationTokenRequest) (*minter.MintDelegationTokenResponse, error) {
	state := auth.GetState(c)

	// Dump the whole request and relevant auth state to the debug log.
	if logging.IsLogging(c, logging.Debug) {
		opts := protojson.MarshalOptions{Indent: "  "}
		logging.Debugf(c, "PeerIdentity: %s", state.PeerIdentity())
		logging.Debugf(c, "MintDelegationTokenRequest:\n%s", opts.Format(req))
	}

	// Validate the request authentication context: not an anonymous call, no
	// delegation is used.
	callerID := state.User().Identity
	if callerID != state.PeerIdentity() {
		logging.Errorf(c, "Trying to use delegation, it's forbidden")
		return nil, status.Errorf(codes.PermissionDenied, "delegation is forbidden for this API call")
	}
	if callerID == identity.AnonymousIdentity {
		logging.Errorf(c, "Unauthenticated request")
		return nil, status.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Grab a string that identifies token server version. This almost always
	// just hits local memory cache.
	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	rules, err := r.Rules(c)
	if err != nil {
		// Don't put error details in the message, it may be returned to
		// unauthorized callers.
		logging.WithError(err).Errorf(c, "Failed to load delegation rules")
		return nil, status.Errorf(codes.Internal, "failed to load delegation rules")
	}

	// Make sure the caller is mentioned in the config before doing anything else.
	// This rejects unauthorized callers early. Passing this check doesn't mean
	// that there's a matching rule though, so the request still can be rejected
	// later.
	switch ok, err := rules.IsAuthorizedRequestor(c, callerID); {
	case err != nil:
		logging.WithError(err).Errorf(c, "IsAuthorizedRequestor failed")
		return nil, status.Errorf(codes.Internal, "failed to check authorization")
	case !ok:
		logging.Errorf(c, "Didn't pass initial authorization")
		return nil, status.Errorf(codes.PermissionDenied, "not authorized")
	}

	// Validate requested token lifetime. It's not part of the rules query.
	if req.ValidityDuration == 0 {
		req.ValidityDuration = 3600
	}
	if req.ValidityDuration < 0 {
		err = fmt.Errorf("invalid 'validity_duration' (%d)", req.ValidityDuration)
		logging.WithError(err).Errorf(c, "Bad request")
		return nil, status.Errorf(codes.InvalidArgument, "bad request - %s", err)
	}

	// Same for tags, they are transferred intact to the final token.
	if err := utils.ValidateTags(req.Tags); err != nil {
		err = fmt.Errorf("invalid 'tags': %s", err)
		logging.WithError(err).Errorf(c, "Bad request")
		return nil, status.Errorf(codes.InvalidArgument, "bad request - %s", err)
	}

	// Validate and normalize the request. This may do relatively expensive calls
	// to resolve "https://<service-url>" entries to "service:<id>" entries.
	query, err := buildRulesQuery(c, req, callerID)
	if err != nil {
		if transient.Tag.In(err) {
			logging.WithError(err).Errorf(c, "buildRulesQuery failed")
			return nil, status.Errorf(codes.Internal, "failure when resolving target service ID - %s", err)
		}
		logging.WithError(err).Errorf(c, "Bad request")
		return nil, status.Errorf(codes.InvalidArgument, "bad request - %s", err)
	}

	// Consult the config to find the rule that allows this operation (if any).
	rule, err := rules.FindMatchingRule(c, query)
	if err != nil {
		if transient.Tag.In(err) {
			logging.WithError(err).Errorf(c, "FindMatchingRule failed")
			return nil, status.Errorf(codes.Internal, "failure when checking rules - %s", err)
		}
		logging.WithError(err).Errorf(c, "Didn't pass rules check")
		return nil, status.Errorf(codes.PermissionDenied, "forbidden - %s", err)
	}
	logging.Infof(c, "Found the matching rule %q in the config rev %s", rule.Name, rules.ConfigRevision())

	// Make sure the requested token lifetime is allowed by the rule.
	if req.ValidityDuration > rule.MaxValidityDuration {
		err = fmt.Errorf(
			"the requested validity duration (%d sec) exceeds the maximum allowed one (%d sec)",
			req.ValidityDuration, rule.MaxValidityDuration)
		logging.WithError(err).Errorf(c, "Validity duration check didn't pass")
		return nil, status.Errorf(codes.PermissionDenied, "forbidden - %s", err)
	}

	var resp *minter.MintDelegationTokenResponse
	p := mintParams{
		request:    req,
		query:      query,
		rule:       rule,
		serviceVer: serviceVer,
	}
	if r.mintMock != nil {
		resp, err = r.mintMock(c, &p)
	} else {
		resp, err = r.mint(c, &p)
	}
	if err != nil {
		return nil, err
	}

	if r.LogToken != nil {
		// Errors during logging are considered not fatal. We have a monitoring
		// counter that tracks number of errors, so they are not totally invisible.
		tokInfo := MintedTokenInfo{
			Request:   req,
			Response:  resp,
			ConfigRev: rules.ConfigRevision(),
			Rule:      rule,
			PeerIP:    state.PeerIP(),
			RequestID: trace.SpanContextFromContext(c).TraceID().String(),
			AuthDBRev: authdb.Revision(state.DB()),
		}
		if logErr := r.LogToken(c, &tokInfo); logErr != nil {
			logging.WithError(logErr).Errorf(c, "Failed to insert the delegation token into the BigQuery log")
		}
	}

	return resp, nil
}

// mintParams are passed to 'mint' function.
type mintParams struct {
	request    *minter.MintDelegationTokenRequest // the original RPC request
	query      *RulesQuery                        // extracted from the request
	rule       *admin.DelegationRule              // looked up in the config based on 'query'
	serviceVer string                             // version string to put in the response
}

// mint is called to make the token after the request has been authorized.
func (r *MintDelegationTokenRPC) mint(c context.Context, p *mintParams) (*minter.MintDelegationTokenResponse, error) {
	id, err := revocation.GenerateTokenID(c, tokenIDSequenceKind)
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when generating token ID.")
		return nil, status.Errorf(codes.Internal, "error when generating token ID - %s", err)
	}

	// All the stuff here has already been validated in 'MintDelegationToken'.
	subtok := &messages.Subtoken{
		Kind:              messages.Subtoken_BEARER_DELEGATION_TOKEN,
		SubtokenId:        id,
		DelegatedIdentity: string(p.query.Delegator),
		RequestorIdentity: string(p.query.Requestor),
		CreationTime:      clock.Now(c).Unix(),
		ValidityDuration:  int32(p.request.ValidityDuration),
		Audience:          p.query.Audience.ToStrings(),
		Services:          p.query.Services.ToStrings(),
		Tags:              p.request.Tags,
	}

	signed, err := SignToken(c, r.Signer, subtok)
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when signing the token.")
		return nil, status.Errorf(codes.Internal, "error when signing the token - %s", err)
	}

	return &minter.MintDelegationTokenResponse{
		Token:              signed,
		DelegationSubtoken: subtok,
		ServiceVersion:     p.serviceVer,
	}, nil
}

// buildRulesQuery validates the request, extracts and normalizes relevant
// fields into RulesQuery object.
//
// May return transient errors.
func buildRulesQuery(c context.Context, req *minter.MintDelegationTokenRequest, requestor identity.Identity) (*RulesQuery, error) {
	// Validate 'delegated_identity'.
	var err error
	var delegator identity.Identity
	if req.DelegatedIdentity == "" {
		return nil, fmt.Errorf("'delegated_identity' is required")
	}
	if req.DelegatedIdentity == Requestor {
		delegator = requestor // the requestor is delegating its own identity
	} else {
		if delegator, err = identity.MakeIdentity(req.DelegatedIdentity); err != nil {
			return nil, fmt.Errorf("bad 'delegated_identity' - %s", err)
		}
	}

	// Validate 'audience', convert it into a set.
	if len(req.Audience) == 0 {
		return nil, fmt.Errorf("'audience' is required")
	}
	audienceSet, err := identityset.FromStrings(req.Audience, skipRequestor)
	if err != nil {
		return nil, fmt.Errorf("bad 'audience' - %s", err)
	}
	if sliceHasString(req.Audience, Requestor) {
		audienceSet.AddIdentity(requestor)
	}

	// Split 'services' into two lists: URLs and everything else (which is
	// "service:..." and "*" presumably, validated below).
	if len(req.Services) == 0 {
		return nil, fmt.Errorf("'services' is required")
	}
	urls := make([]string, 0, len(req.Services))
	rest := make([]string, 0, len(req.Services))
	for _, srv := range req.Services {
		if strings.HasPrefix(srv, "https://") {
			urls = append(urls, srv)
		} else {
			rest = append(rest, srv)
		}
	}

	// Convert the list into a set, verify it contains only services (or "*").
	servicesSet, err := identityset.FromStrings(rest, nil)
	if err != nil {
		return nil, fmt.Errorf("bad 'services' - %s", err)
	}
	if len(servicesSet.Groups) != 0 {
		return nil, fmt.Errorf("bad 'services' - can't specify groups")
	}
	for ident := range servicesSet.IDs {
		if ident.Kind() != identity.Service {
			return nil, fmt.Errorf("bad 'services' - %q is not a service ID", ident)
		}
	}

	// Resolve URLs into app IDs. This may involve URL fetch calls (if the cache
	// is cold), so skip this expensive call if already specifying the universal
	// set of all services.
	if !servicesSet.All && len(urls) != 0 {
		if err = resolveServiceIDs(c, urls, servicesSet); err != nil {
			return nil, err
		}
	}

	// Done!
	return &RulesQuery{
		Requestor: requestor,
		Delegator: delegator,
		Audience:  audienceSet,
		Services:  servicesSet,
	}, nil
}

// fetchLUCIServiceIdentity is replaced in tests.
var fetchLUCIServiceIdentity = signing.FetchLUCIServiceIdentity

// resolveServiceIDs takes a bunch of service URLs and resolves them to
// 'service:<app-id>' identities, putting them in the 'out' set.
//
// May return transient errors.
func resolveServiceIDs(c context.Context, urls []string, out *identityset.Set) error {
	// URL fetch calls below should be extra fast. If they get stuck, something is
	// horribly wrong, better to abort soon.
	c, cancel := clock.WithTimeout(c, 5*time.Second)
	defer cancel()

	type Result struct {
		URL string
		ID  identity.Identity
		Err error
	}

	ch := make(chan Result, len(urls))

	for _, url := range urls {
		go func(url string) {
			id, err := fetchLUCIServiceIdentity(c, url)
			ch <- Result{url, id, err}
		}(url)
	}

	for range urls {
		result := <-ch
		if result.Err != nil {
			if transient.Tag.In(result.Err) {
				return result.Err
			}
			return fmt.Errorf("could not resolve %q to service ID - %s", result.URL, result.Err)
		}
		out.AddIdentity(result.ID)
	}

	return nil
}
