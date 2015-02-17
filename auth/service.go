// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"infra/libs/build"
	"infra/libs/logging"
)

// IdentityKind is enum like type with possible kinds of identities.
type IdentityKind string

const (
	// IdentityKindUnknown is used when server return identity kind not recognized
	// by the client.
	IdentityKindUnknown IdentityKind = ""
	// IdentityKindAnonymous is used to represent anonymous callers.
	IdentityKindAnonymous IdentityKind = "anonymous"
	// IdentityKindBot is used to represent bots. Identity name is bot's IP.
	IdentityKindBot IdentityKind = "bot"
	// IdentityKindService is used to represent AppEngine apps when they use
	// X-Appengine-Inbound-AppId header to authenticate. Identity name is app ID.
	IdentityKindService IdentityKind = "service"
	// IdentityKindUser is used to represent end users or OAuth service accounts.
	// Identity name is an associated email (user email or service account email).
	IdentityKindUser IdentityKind = "user"
)

var (
	// ErrAccessDenied is returned by GroupsService methods if caller doesn't have
	// enough permissions to execute an action. Corresponds to 403 HTTP status.
	ErrAccessDenied = errors.New("Access denied (HTTP 403)")

	// ErrNoSuchItem is returned by GroupsService methods if requested item
	// (e.g. a group) doesn't exist. Corresponds to 404 HTTP status.
	ErrNoSuchItem = errors.New("No such item (HTTP 404)")
)

// Identity represents some caller that can make requests. It generalizes
// accounts of real people, bot accounts and service-to-service accounts.
type Identity struct {
	// Kind describes what sort of identity this struct represents.
	Kind IdentityKind
	// Name defines concrete instance of identity, its meaning depends on kind.
	Name string
}

// Group is a named list of identities, included subgroups and glob-like
// patterns (e.g. "user:*@example.com).
type Group struct {
	// Name is a name of the group.
	Name string
	// Description is a human readable description of the group.
	Description string
	// Members is a list of identities included in the group explicitly.
	Members []Identity
	// Globs is a list of glob-like patterns for identities.
	Globs []string
	// Nested is a list of group names included into this group.
	Nested []string
}

// String returns human readable representation of Identity.
func (i Identity) String() string {
	switch i.Kind {
	case IdentityKindUnknown:
		return "unknown"
	case IdentityKindAnonymous:
		return "anonymous"
	case IdentityKindUser:
		return i.Name
	default:
		return fmt.Sprintf("%s:%s", i.Kind, i.Name)
	}
}

// GroupsService knows how to talk to Groups API backend. Server side code
// is in https://code.google.com/p/swarming repository.
type GroupsService struct {
	client     *http.Client
	serviceURL string
	logger     logging.Logger
}

// NewGroupsService constructs new instance of GroupsService that talks to given
// service URL via given http.Client. If serviceURL is empty string, the default
// backend will be used. If httpClient is nil, default authenticated client will
// be used.
func NewGroupsService(serviceURL string, httpClient *http.Client, logger logging.Logger) (c *GroupsService, err error) {
	if serviceURL == "" {
		serviceURL = defaultGroupsBackend()
	}
	if httpClient == nil {
		httpClient, err = AuthenticatedClient(true, DefaultAuthenticator)
		if err != nil {
			return
		}
	}
	if logger == nil {
		logger = logging.DefaultLogger
	}
	c = &GroupsService{
		client:     httpClient,
		serviceURL: serviceURL,
		logger:     logger,
	}
	return
}

// ServiceURL returns a string with root URL of a Groups backend.
func (s *GroupsService) ServiceURL() string {
	return s.serviceURL
}

// FetchCallerIdentity returns caller's own Identity as seen by the server.
func (s *GroupsService) FetchCallerIdentity() (Identity, error) {
	var response struct {
		Identity string `json:"identity"`
	}
	err := s.doGet("/auth/api/v1/accounts/self", &response)
	if err != nil {
		return Identity{}, err
	}
	return parseIdentity(response.Identity)
}

// FetchGroup returns a group definition. It does not fetch nested groups.
// Returns ErrNoSuchItem error if no such group.
func (s *GroupsService) FetchGroup(name string) (Group, error) {
	var response struct {
		Group struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Members     []string `json:"members"`
			Globs       []string `json:"globs"`
			Nested      []string `json:"nested"`
		} `json:"group"`
	}
	err := s.doGet("/auth/api/v1/groups/"+name, &response)
	if err != nil {
		return Group{}, err
	}
	if response.Group.Name != name {
		return Group{}, fmt.Errorf(
			"Unexpected group name in server response: '%s', expecting '%s'",
			response.Group.Name, name)
	}
	g := Group{
		Name:        response.Group.Name,
		Description: response.Group.Description,
		Members:     make([]Identity, 0, len(response.Group.Members)),
		Globs:       response.Group.Globs,
		Nested:      response.Group.Nested,
	}
	for _, str := range response.Group.Members {
		ident, err := parseIdentity(str)
		if err != nil {
			s.logger.Warningf("auth: failed to parse an identity in a group, ignoring it - %s", err)
		} else {
			g.Members = append(g.Members, ident)
		}
	}
	return g, nil
}

// doGet sends GET HTTP requests and decodes JSON into response. It retries
// multiple times on transient errors.
func (s *GroupsService) doGet(path string, response interface{}) error {
	if len(path) == 0 || path[0] != '/' {
		return fmt.Errorf("Path should start with '/': %s", path)
	}
	url := s.serviceURL + path
	for attempt := 0; attempt < 5; attempt++ {
		if attempt != 0 {
			s.logger.Warningf("auth: retrying request to %s", url)
			sleep(2 * time.Second)
		}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		resp, err := doRequest(s.client, req)
		if err != nil {
			return err
		}
		// Success?
		if resp.StatusCode < 300 {
			defer resp.Body.Close()
			return json.NewDecoder(resp.Body).Decode(response)
		}
		// Fatal error?
		if resp.StatusCode >= 300 && resp.StatusCode < 500 {
			defer resp.Body.Close()
			switch resp.StatusCode {
			case 403:
				return ErrAccessDenied
			case 404:
				return ErrNoSuchItem
			default:
				body, _ := ioutil.ReadAll(resp.Body)
				return fmt.Errorf("Unexpected reply (HTTP %d):\n%s", resp.StatusCode, string(body))
			}
		}
		// Retry.
		resp.Body.Close()
	}
	return fmt.Errorf("Request to %s failed after 5 attempts", url)
}

// defaultGroupsBackend return URL of a service to talk to if not overridden in
// NewGroupsService.
func defaultGroupsBackend() string {
	if build.ReleaseBuild {
		return "https://chrome-infra-auth.appspot.com"
	}
	return "https://chrome-infra-auth-dev.appspot.com"
}

// parseIdentity takes a string of form "<kind>:<name>" and returns Identity
// struct.
func parseIdentity(str string) (Identity, error) {
	chunks := strings.Split(str, ":")
	if len(chunks) != 2 {
		return Identity{}, fmt.Errorf("Invalid identity string: '%s'", str)
	}
	kind := IdentityKind(chunks[0])
	name := chunks[1]
	switch kind {
	case IdentityKindAnonymous:
		if name != "anonymous" {
			return Identity{}, fmt.Errorf("Invalid anonymous identity: '%s'", str)
		}
		return Identity{
			Kind: IdentityKindAnonymous,
			Name: "anonymous",
		}, nil
	case IdentityKindBot, IdentityKindService, IdentityKindUser:
		return Identity{
			Kind: kind,
			Name: name,
		}, nil
	default:
		return Identity{}, fmt.Errorf("Unrecognized identity kind: '%s'", str)
	}
}

// sleep is mocked in tests.
var sleep = time.Sleep

// doRequest is mocked in tests.
var doRequest = func(c *http.Client, req *http.Request) (*http.Response, error) {
	return c.Do(req)
}
