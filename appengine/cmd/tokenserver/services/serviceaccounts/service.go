// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package serviceaccounts implements ServiceAccounts API.
//
// Code defined here is either invoked by an administrator or by the service
// itself (from implementation of other services).
package serviceaccounts

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/oauth2/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	luciiam "github.com/luci/luci-go/common/gcloud/iam"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/appengine/cmd/tokenserver/certchecker"
	"github.com/luci/luci-go/appengine/cmd/tokenserver/model"
	"github.com/luci/luci-go/common/api/tokenserver/v1"
)

// accountIDRegexp is regular expression for allowed service account emails.
var accountIDRegexp = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])$`)

// Server implements tokenserver.ServiceAccountsServer RPC interface.
//
// It assumes authorization has happened already.
type Server struct {
	transport     http.RoundTripper // used in unit tests to mock OAuth
	iamBackendURL string            // used in unit tests to mock IAM API
	ownEmail      string            // used in unit tests to mock GAE Info API
}

// CreateServiceAccount creates Google Cloud IAM service account associated
// with given CN.
func (s *Server) CreateServiceAccount(c context.Context, r *tokenserver.CreateServiceAccountRequest) (*tokenserver.CreateServiceAccountResponse, error) {
	// Validate FQDN and load proper config.
	accountID, domain, err := validateFQDN(r.Fqdn)
	if err != nil {
		return nil, err
	}
	cfg, err := s.getCAConfig(c, r.Ca)
	if err != nil {
		return nil, err
	}
	domainCfg, err := domainConfig(cfg, domain)
	if err != nil {
		return nil, err
	}

	// Create the service account.
	account, err := s.DoCreateServiceAccount(c, CreateServiceAccountParams{
		Project:   domainCfg.CloudProjectName,
		AccountID: accountID,
		FQDN:      r.Fqdn,
		Force:     r.Force,
	})
	if err != nil {
		return nil, err
	}
	return &tokenserver.CreateServiceAccountResponse{
		ServiceAccount: account,
	}, nil
}

// CreateServiceAccountParams is passed to DoCreateServiceAccount.
type CreateServiceAccountParams struct {
	Project   string // cloud project name to create account in
	AccountID string // account ID, the email will be <accountID>@...
	FQDN      string // FQDN of a host to create an account for
	Force     bool   // true to skip datastore check and call IAM API no matter what
}

// DoCreateServiceAccount does the actual job of CreateServiceAccount.
//
// It exists as a separate method so that other parts of the token server
// implementation may call it directly, bypassing some steps done by
// CreateServiceAccount.
//
// Assumes 'params' is validated already. Returns grpc.Errorf on errors.
func (s *Server) DoCreateServiceAccount(c context.Context, params CreateServiceAccountParams) (*tokenserver.ServiceAccount, error) {
	// Normalize fqdn for comparison.
	fqdn := strings.ToLower(params.FQDN)

	// Checks the datastore, perhaps we already registered it.
	if !params.Force {
		account, err := fetchAccountInfo(c, params.Project, params.AccountID)
		if err != nil {
			return nil, grpc.Errorf(codes.Internal, "transient error when fetching account info from datastore - %s", err)
		}
		if account != nil {
			// Two FQDN's happen to have identical hostname? This is a big deal and
			// generally not supported.
			if account.Fqdn != fqdn {
				return nil, grpc.Errorf(codes.FailedPrecondition, "the account is already assigned to %q", account.Fqdn)
			}
			return account, nil
		}
	}

	// Account display name must be < 100 bytes. We don't use it for anything
	// very important, so just trim.
	displayName := fqdn
	if len(fqdn) >= 100 {
		displayName = fqdn[:100]
	}

	// Attempt to create. May fail with http.StatusConflict if already exists.
	expectedEmail := serviceAccountEmail(params.Project, params.AccountID)
	logging.Infof(c, "Creating account %q", expectedEmail)
	account, err := s.iamCreateAccount(c, params.Project, params.AccountID, displayName)

	switch apiErr, _ := err.(*googleapi.Error); {
	case err == nil:
		logging.Infof(c, "Account %q created", expectedEmail)
		if account.Email != expectedEmail {
			// Log the error, but proceed anyway, since the expectedEmail is
			// important only when dealing with rare race conditions. Returning error
			// here would block the token server operations completely in case Google
			// decides to unexpectedly change service account email format.
			logging.Errorf(
				c, "Real account email %q doesn't match expected one %q",
				account.Email, expectedEmail)
		}

	case apiErr != nil && apiErr.Code == http.StatusConflict:
		logging.Warningf(c, "Account %q already exists, fetching its details", expectedEmail)
		account, err = s.iamGetAccount(c, params.Project, expectedEmail)
		if err != nil {
			return nil, grpc.Errorf(
				codes.Internal, "unexpected error when fetching account details for %q - %s",
				expectedEmail, err)
		}
		// In an unlikely event of a race condition when creating two accounts with
		// identical hostnames but different FQDNs, the display names here will not
		// match. Ask the client to retry (to read account state from the datastore
		// and fail properly).
		if account.DisplayName != displayName {
			return nil, grpc.Errorf(
				codes.Internal, "expected account %q to have display name %q, but it is %q",
				expectedEmail, displayName, account.DisplayName)
		}

	case err != nil:
		return nil, grpc.Errorf(codes.Internal, "cloud IAM call to create a service account failed - %s", err)
	}

	// Account has been created (or existed before). Allow the token server to
	// use its 'signBlob' API by granting it 'serviceAccountActor' role on the
	// service account resource.
	logging.Infof(c, "Setting IAM policy of %q...", account.Email)
	err = s.iamGrantActorRole(c, params.Project, account.Email)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "failed to modify IAM policy of %q - %s", account.Email, err)
	}
	logging.Infof(c, "IAM policy of %q adjusted", account.Email)

	// Store the account state in the datastore to avoid redoing all this again.
	result, err := storeAccountInfo(c, params.Project, params.AccountID, fqdn, account)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "failed to save account info - %s", err)
	}
	return result, nil
}

// getCAConfig returns CertificateAuthorityConfig for a CA.
//
// Returns grpc.Error on errors.
func (s *Server) getCAConfig(c context.Context, ca string) (*tokenserver.CertificateAuthorityConfig, error) {
	var entity *model.CA

	checker, err := certchecker.GetCertChecker(c, ca)
	if err != nil {
		goto fail
	}
	entity, err = checker.GetCA(c)
	if err != nil {
		goto fail
	}
	return entity.ParsedConfig, nil

fail:
	if errors.IsTransient(err) {
		return nil, grpc.Errorf(codes.Internal, "transient error when fetching CA config - %s", err)
	}
	return nil, grpc.Errorf(codes.Unknown, "error when fetching CA config - %s", err)
}

// httpClient returns an authenticating client with deadline set to 20 sec.
//
// Note that on GAE the only way to set a deadline for HTTP request is to grab
// a new transport based on a context with deadline.
//
// context/ctxhttp does not work, since it attempts to cancel in-flight requests
// on timeout and GAE ignores that.
func (s *Server) httpClient(c context.Context) (*http.Client, error) {
	transport := s.transport
	if transport != nil {
		return &http.Client{Transport: transport}, nil
	}
	c, _ = clock.WithTimeout(c, 20*time.Second)
	transport, err := client.Transport(c, []string{iam.CloudPlatformScope}, nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport}, nil
}

// iamService returns configured *iam.Service to talk to Service Accounts API.
func (s *Server) iamService(c context.Context) (*iam.Service, error) {
	client, err := s.httpClient(c)
	if err != nil {
		return nil, err
	}
	service, err := iam.New(client)
	if err != nil {
		return nil, err
	}
	if s.iamBackendURL != "" {
		service.BasePath = s.iamBackendURL
	}
	return service, nil
}

// iamCreateAccount wraps ServiceAccounts.Create IAM API call.
func (s *Server) iamCreateAccount(c context.Context, project, accountID, displayName string) (*iam.ServiceAccount, error) {
	service, err := s.iamService(c)
	if err != nil {
		return nil, err
	}
	call := service.Projects.ServiceAccounts.Create(
		"projects/"+project,
		&iam.CreateServiceAccountRequest{
			AccountId: accountID,
			ServiceAccount: &iam.ServiceAccount{
				DisplayName: displayName,
			},
		})
	return call.Context(c).Do()
}

// iamGetAccount wraps ServiceAccounts.Get IAM API call.
func (s *Server) iamGetAccount(c context.Context, project, email string) (*iam.ServiceAccount, error) {
	service, err := s.iamService(c)
	if err != nil {
		return nil, err
	}
	call := service.Projects.ServiceAccounts.Get(serviceAccountResource(project, email))
	return call.Context(c).Do()
}

// iamGrantActorRole modifies service account IAM policy to allow the token
// server to use 'signBlob' API call needed to produce access tokens.
func (s *Server) iamGrantActorRole(c context.Context, project, email string) error {
	httpClient, err := s.httpClient(c)
	if err != nil {
		return err
	}
	iam := &luciiam.Client{
		Client:   httpClient,
		BasePath: s.iamBackendURL,
	}

	// Need to know who we are to add ourselves to the policy binding.
	ownEmail, err := s.getServiceOwnEmail(c)
	if err != nil {
		return fmt.Errorf("failed to grab service's own email - %s", err)
	}

	// Convert to an IAM principal name.
	principal := ""
	if strings.HasSuffix(ownEmail, ".gserviceaccount.com") {
		principal = "serviceAccount:" + ownEmail
	} else {
		if !info.Get(c).IsDevAppServer() {
			panic("this branch must not be reachable in prod")
		}
		principal = "user:" + ownEmail
	}

	resource := serviceAccountResource(project, email)
	return iam.ModifyIAMPolicy(c, resource, func(p *luciiam.Policy) error {
		p.GrantRole("roles/iam.serviceAccountActor", principal)
		return nil
	})
}

// getServiceOwnEmail returns an email address associated with the token server.
//
// On real GAE it just asks GAE Info API. On dev server we have to ask the
// outside world (Google Userinfo API) for this information, since GAE API
// returns incorrect mocked stuff.
//
// Finally, in unit tests just returns s.ownEmail.
func (s *Server) getServiceOwnEmail(c context.Context) (string, error) {
	if s.ownEmail != "" {
		return s.ownEmail, nil
	}

	// On prod use whatever GAE tells us.
	if !info.Get(c).IsDevAppServer() {
		return info.Get(c).ServiceAccount()
	}

	// On dev server just boldly ask Google about the token we use in the
	// authenticated calls, since GAE API returns whatever is in app.yaml, not
	// what is actually used by dev_appserver (which is the token of a user logged
	// in via 'gcloud auth' mechanism).
	httpClient, err := s.httpClient(c)
	if err != nil {
		return "", err
	}
	service, err := oauth2.New(httpClient)
	if err != nil {
		return "", err
	}
	resp, err := service.Userinfo.V2.Me.Get().Do()
	if err != nil {
		return "", err
	}
	logging.Infof(c, "Devserver is running as %q", resp.Email)
	return resp.Email, nil
}

////////////////////////////////////////////////////////////////////////////////

// validateFQDN splits FQDN into hostname and domain name, validating them.
func validateFQDN(fqdn string) (host, domain string, err error) {
	fqdn = strings.ToLower(fqdn)
	chunks := strings.SplitN(fqdn, ".", 2)
	if len(chunks) != 2 {
		return "", "", grpc.Errorf(codes.InvalidArgument, "not a valid FQDN %q", fqdn)
	}
	// Host name is used as service account ID. Validate it.
	// See https://cloud.google.com/iam/reference/rest/v1/projects.serviceAccounts/create.
	// The account ID is also limited in length to 6-30 chars (this is not
	// documented though).
	host, domain = chunks[0], chunks[1]
	if len(host) < 6 || len(host) >= 30 || !accountIDRegexp.MatchString(host) {
		return "", "", grpc.Errorf(
			codes.InvalidArgument, "the hostname (%q) must match %q and be 6-30 characters long, it doesn't",
			host, accountIDRegexp.String())
	}
	return host, domain, nil
}

// domainConfig returns DomainConfig for a domain.
//
// Returns grpc.Error when there's no such config.
func domainConfig(cfg *tokenserver.CertificateAuthorityConfig, domain string) (*tokenserver.DomainConfig, error) {
	for _, domainCfg := range cfg.KnownDomains {
		for _, domainInCfg := range domainCfg.Domain {
			if domainInCfg == domain {
				return domainCfg, nil
			}
		}
	}
	return nil, grpc.Errorf(codes.InvalidArgument, "the domain %q is not whitelisted in the config", domain)
}

// serviceAccountEmail returns expected email address of a service account.
//
// Unfortunately we had to hardcode this rule here to properly handle race
// conditions when creating an account: /v1/projects.serviceAccounts/get API
// accepts emails, not short account IDs we pass to 'create' API call. So if
// 'create' call fails with "Already exists" error, we had to guess an email to
// fetch details of the existing account.
func serviceAccountEmail(project, accountID string) string {
	return fmt.Sprintf("%s@%s.iam.gserviceaccount.com", accountID, project)
}

// serviceAccountResource returns full resource name of a service accounts.
func serviceAccountResource(project, email string) string {
	return fmt.Sprintf("projects/%s/serviceAccounts/%s", project, email)
}

// storeAccountInfo puts details about a service account in the datastore.
//
// Returns the resulting tokenserver.ServiceAccount object.
func storeAccountInfo(c context.Context, project, accountID, fqdn string, acc *iam.ServiceAccount) (*tokenserver.ServiceAccount, error) {
	entity := model.ServiceAccount{
		ID:             project + "/" + accountID,
		ProjectID:      project,
		UniqueID:       acc.UniqueId,
		Email:          acc.Email,
		DisplayName:    acc.DisplayName,
		OAuth2ClientID: acc.Oauth2ClientId,
		FQDN:           fqdn,
		Registered:     clock.Now(c).UTC(),
	}
	if err := datastore.Get(c).Put(&entity); err != nil {
		return nil, err
	}
	return entity.GetProto(), nil
}

// fetchAccountInfo returns service account details stored in the datastore.
//
// Returns (nil, nil) if no such service account was registered yet.
func fetchAccountInfo(c context.Context, project, accountID string) (*tokenserver.ServiceAccount, error) {
	entity := model.ServiceAccount{
		ID: project + "/" + accountID,
	}
	switch err := datastore.Get(c).Get(&entity); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, errors.WrapTransient(err)
	}
	return entity.GetProto(), nil
}
