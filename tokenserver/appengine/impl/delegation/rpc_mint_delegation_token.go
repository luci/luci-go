// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/delegation/messages"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/signing"

	admin "github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/identityset"
)

// MintDelegationTokenRPC implements TokenMinter.MintDelegationToken RPC method.
type MintDelegationTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer

	// ConfigLoader loads delegation config on demand.
	//
	// In prod it is DelegationConfigLoader.
	ConfigLoader func(context.Context) (*DelegationConfig, error)

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
		m := jsonpb.Marshaler{Indent: "  "}
		dump, _ := m.MarshalToString(req)
		logging.Debugf(c, "PeerIdentity: %s", state.PeerIdentity())
		logging.Debugf(c, "MintDelegationTokenRequest:\n%s", dump)
	}

	// Validate the request authentication context: not an anonymous call, no
	// delegation is used.
	callerID := state.User().Identity
	if callerID != state.PeerIdentity() {
		logging.Errorf(c, "Trying to use delegation, it's forbidden")
		return nil, grpc.Errorf(codes.PermissionDenied, "delegation is forbidden for this API call")
	}
	if callerID == identity.AnonymousIdentity {
		logging.Errorf(c, "Unauthenticated request")
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication required")
	}

	// Grab a string that identifies token server version. This almost always
	// just hits local memory cache.
	serviceVer, err := utils.ServiceVersion(c, r.Signer)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "can't grab service version - %s", err)
	}

	cfg, err := r.ConfigLoader(c)
	if err != nil {
		// Don't put error details in the message, it may be returned to
		// unauthorized callers. ConfigLoader logs the error already.
		return nil, grpc.Errorf(codes.Internal, "failed to load delegation config")
	}

	// Make sure the caller is mentioned in the config before doing anything else.
	// This rejects unauthorized callers early. Passing this check doesn't mean
	// that there's a matching rule though, so the request still can be rejected
	// later.
	switch ok, err := cfg.IsAuthorizedRequestor(c, callerID); {
	case err != nil:
		logging.WithError(err).Errorf(c, "IsAuthorizedRequestor failed")
		return nil, grpc.Errorf(codes.Internal, "failed to check authorization")
	case !ok:
		logging.Errorf(c, "Didn't pass initial authorization")
		return nil, grpc.Errorf(codes.PermissionDenied, "not authorized")
	}

	// Validate requested token lifetime. It's not part of the rules query.
	if req.ValidityDuration == 0 {
		req.ValidityDuration = 3600
	}
	if req.ValidityDuration < 0 {
		err = fmt.Errorf("invalid 'validity_duration' (%d)", req.ValidityDuration)
		logging.WithError(err).Errorf(c, "Bad request")
		return nil, grpc.Errorf(codes.InvalidArgument, "bad request - %s", err)
	}

	// Validate and normalize the request. This may do relatively expensive calls
	// to resolve "https://<service-url>" entries to "service:<id>" entries.
	query, err := buildRulesQuery(c, req, callerID)
	if err != nil {
		if errors.IsTransient(err) {
			logging.WithError(err).Errorf(c, "buildRulesQuery failed")
			return nil, grpc.Errorf(codes.Internal, "failure when resolving target service ID - %s", err)
		}
		logging.WithError(err).Errorf(c, "Bad request")
		return nil, grpc.Errorf(codes.InvalidArgument, "bad request - %s", err)
	}

	// Consult the config to find the rule that allows this operation (if any).
	rule, err := cfg.FindMatchingRule(c, query)
	if err != nil {
		if errors.IsTransient(err) {
			logging.WithError(err).Errorf(c, "FindMatchingRule failed")
			return nil, grpc.Errorf(codes.Internal, "failure when checking rules - %s", err)
		}
		logging.WithError(err).Errorf(c, "Didn't pass rules check")
		return nil, grpc.Errorf(codes.PermissionDenied, "forbidden - %s", err)
	}
	logging.Infof(c, "Found the matching rule %q in the config rev %s", rule.Name, cfg.Revision)

	// Make sure the requested token lifetime is allowed by the rule.
	if req.ValidityDuration > rule.MaxValidityDuration {
		err = fmt.Errorf(
			"the requested validity duration (%d sec) exceeds the maximum allowed one (%d sec)",
			req.ValidityDuration, rule.MaxValidityDuration)
		logging.WithError(err).Errorf(c, "Validity duration check didn't pass")
		return nil, grpc.Errorf(codes.PermissionDenied, "forbidden - %s", err)
	}

	p := mintParams{
		request:    req,
		cfg:        cfg,
		query:      query,
		rule:       rule,
		serviceVer: serviceVer,
	}
	if r.mintMock != nil {
		return r.mintMock(c, &p)
	}
	return r.mint(c, &p)
}

// mintParams are passed to 'mint' function.
type mintParams struct {
	request    *minter.MintDelegationTokenRequest // the original RPC request
	cfg        *DelegationConfig                  // the currently active config
	query      *RulesQuery                        // extracted from the request
	rule       *admin.DelegationRule              // looked up in the config based on 'query'
	serviceVer string                             // version string to put in the response
}

// mint is called to make the token after the request has been authorized.
func (r *MintDelegationTokenRPC) mint(c context.Context, p *mintParams) (*minter.MintDelegationTokenResponse, error) {
	id, err := GenerateTokenID(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when generating token ID.")
		return nil, grpc.Errorf(codes.Internal, "error when generating token ID - %s", err)
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
	}

	signed, err := SignToken(c, r.Signer, subtok)
	if err != nil {
		logging.WithError(err).Errorf(c, "Error when signing the token.")
		return nil, grpc.Errorf(codes.Internal, "error when signing the token - %s", err)
	}

	// TODO(vadimsh): Record the token in the audit log along with the rule,
	// config revision and request details.

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
	c, abort := clock.WithTimeout(c, 5*time.Second)
	defer abort()

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

	for i := 0; i < len(urls); i++ {
		result := <-ch
		if result.Err != nil {
			if errors.IsTransient(result.Err) {
				return result.Err
			}
			return fmt.Errorf("could not resolve %q to service ID - %s", result.URL, result.Err)
		}
		out.AddIdentity(result.ID)
	}

	return nil
}
