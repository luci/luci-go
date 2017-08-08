// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package swarming provides access to the .
//
// Usage example:
//
//   import "go.chromium.org/luci/common/api/swarming/swarming/v1"
//   ...
//   swarmingService, err := swarming.New(oauthHttpClient)
package swarming // import "go.chromium.org/luci/common/api/swarming/swarming/v1"

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	context "golang.org/x/net/context"
	ctxhttp "golang.org/x/net/context/ctxhttp"
	gensupport "google.golang.org/api/gensupport"
	googleapi "google.golang.org/api/googleapi"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Always reference these packages, just in case the auto-generated code
// below doesn't.
var _ = bytes.NewBuffer
var _ = strconv.Itoa
var _ = fmt.Sprintf
var _ = json.NewDecoder
var _ = io.Copy
var _ = url.Parse
var _ = gensupport.MarshalJSON
var _ = googleapi.Version
var _ = errors.New
var _ = strings.Replace
var _ = context.Canceled
var _ = ctxhttp.Do

const apiId = "swarming:v1"
const apiName = "swarming"
const apiVersion = "v1"
const basePath = "http://localhost:8080/_ah/api/swarming/v1/"

// OAuth2 scopes used by this API.
const (
	// View your email address
	UserinfoEmailScope = "https://www.googleapis.com/auth/userinfo.email"
)

func New(client *http.Client) (*Service, error) {
	if client == nil {
		return nil, errors.New("client is nil")
	}
	s := &Service{client: client, BasePath: basePath}
	s.Bot = NewBotService(s)
	s.Bots = NewBotsService(s)
	s.Server = NewServerService(s)
	s.Task = NewTaskService(s)
	s.Tasks = NewTasksService(s)
	return s, nil
}

type Service struct {
	client    *http.Client
	BasePath  string // API endpoint base URL
	UserAgent string // optional additional User-Agent fragment

	Bot *BotService

	Bots *BotsService

	Server *ServerService

	Task *TaskService

	Tasks *TasksService
}

func (s *Service) userAgent() string {
	if s.UserAgent == "" {
		return googleapi.UserAgent
	}
	return googleapi.UserAgent + " " + s.UserAgent
}

func NewBotService(s *Service) *BotService {
	rs := &BotService{s: s}
	return rs
}

type BotService struct {
	s *Service
}

func NewBotsService(s *Service) *BotsService {
	rs := &BotsService{s: s}
	return rs
}

type BotsService struct {
	s *Service
}

func NewServerService(s *Service) *ServerService {
	rs := &ServerService{s: s}
	return rs
}

type ServerService struct {
	s *Service
}

func NewTaskService(s *Service) *TaskService {
	rs := &TaskService{s: s}
	return rs
}

type TaskService struct {
	s *Service
}

func NewTasksService(s *Service) *TasksService {
	rs := &TasksService{s: s}
	return rs
}

type TasksService struct {
	s *Service
}

// SwarmingRpcsBootstrapToken: Returns a token to bootstrap a new bot.
type SwarmingRpcsBootstrapToken struct {
	BootstrapToken string `json:"bootstrap_token,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "BootstrapToken") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BootstrapToken") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsBootstrapToken) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsBootstrapToken
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingRpcsBotEvent struct {
	AuthenticatedAs string `json:"authenticated_as,omitempty"`

	// Dimensions: Represents a mapping of string to list of strings.
	Dimensions []*SwarmingRpcsStringListPair `json:"dimensions,omitempty"`

	EventType string `json:"event_type,omitempty"`

	ExternalIp string `json:"external_ip,omitempty"`

	Message string `json:"message,omitempty"`

	Quarantined bool `json:"quarantined,omitempty"`

	State string `json:"state,omitempty"`

	TaskId string `json:"task_id,omitempty"`

	Ts string `json:"ts,omitempty"`

	Version string `json:"version,omitempty"`

	// ForceSendFields is a list of field names (e.g. "AuthenticatedAs") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "AuthenticatedAs") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsBotEvent) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsBotEvent
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingRpcsBotEvents struct {
	Cursor string `json:"cursor,omitempty"`

	Items []*SwarmingRpcsBotEvent `json:"items,omitempty"`

	Now string `json:"now,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Cursor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Cursor") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsBotEvents) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsBotEvents
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsBotInfo: Representation of the BotInfo ndb model.
type SwarmingRpcsBotInfo struct {
	AuthenticatedAs string `json:"authenticated_as,omitempty"`

	BotId string `json:"bot_id,omitempty"`

	Deleted bool `json:"deleted,omitempty"`

	// Dimensions: Represents a mapping of string to list of strings.
	Dimensions []*SwarmingRpcsStringListPair `json:"dimensions,omitempty"`

	ExternalIp string `json:"external_ip,omitempty"`

	FirstSeenTs string `json:"first_seen_ts,omitempty"`

	IsDead bool `json:"is_dead,omitempty"`

	LastSeenTs string `json:"last_seen_ts,omitempty"`

	LeaseExpirationTs string `json:"lease_expiration_ts,omitempty"`

	LeaseId string `json:"lease_id,omitempty"`

	MachineType string `json:"machine_type,omitempty"`

	Quarantined bool `json:"quarantined,omitempty"`

	State string `json:"state,omitempty"`

	TaskId string `json:"task_id,omitempty"`

	TaskName string `json:"task_name,omitempty"`

	Version string `json:"version,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "AuthenticatedAs") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "AuthenticatedAs") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsBotInfo) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsBotInfo
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsBotList: Wraps a list of BotInfo.
type SwarmingRpcsBotList struct {
	Cursor string `json:"cursor,omitempty"`

	DeathTimeout int64 `json:"death_timeout,omitempty,string"`

	// Items: Representation of the BotInfo ndb model.
	Items []*SwarmingRpcsBotInfo `json:"items,omitempty"`

	Now string `json:"now,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Cursor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Cursor") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsBotList) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsBotList
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingRpcsBotTasks struct {
	Cursor string `json:"cursor,omitempty"`

	// Items: Representation of the TaskResultSummary or TaskRunResult ndb
	// model.
	Items []*SwarmingRpcsTaskResult `json:"items,omitempty"`

	Now string `json:"now,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Cursor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Cursor") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsBotTasks) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsBotTasks
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsBotsCount: Returns the count, as requested.
type SwarmingRpcsBotsCount struct {
	Busy int64 `json:"busy,omitempty,string"`

	Count int64 `json:"count,omitempty,string"`

	Dead int64 `json:"dead,omitempty,string"`

	Now string `json:"now,omitempty"`

	Quarantined int64 `json:"quarantined,omitempty,string"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Busy") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Busy") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsBotsCount) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsBotsCount
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsBotsDimensions: Returns all the dimensions and dimension
// possibilities in the fleet.
type SwarmingRpcsBotsDimensions struct {
	// BotsDimensions: Represents a mapping of string to list of strings.
	BotsDimensions []*SwarmingRpcsStringListPair `json:"bots_dimensions,omitempty"`

	Ts string `json:"ts,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "BotsDimensions") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BotsDimensions") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsBotsDimensions) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsBotsDimensions
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsCacheEntry: Describes a named cache that should be
// present on the bot. A CacheEntry in a task specified that the task
// prefers the cache to be present on the bot. A symlink to the cache
// directory is created at /|path|. If cache is not present on the
// machine, the directory is empty. If the tasks makes any changes to
// the contents of the cache directory, they are persisted on the
// machine. If another task runs on the same machine and requests the
// same named cache, even if mapped to a different path, it will see the
// changes.
type SwarmingRpcsCacheEntry struct {
	Name string `json:"name,omitempty"`

	Path string `json:"path,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Name") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Name") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsCacheEntry) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsCacheEntry
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsCancelResponse: Result of a request to cancel a task.
type SwarmingRpcsCancelResponse struct {
	Ok bool `json:"ok,omitempty"`

	WasRunning bool `json:"was_running,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Ok") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Ok") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsCancelResponse) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsCancelResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsCipdInput: Defines CIPD packages to install in task run
// directory.
type SwarmingRpcsCipdInput struct {
	// ClientPackage: A CIPD package to install in the run dir before task
	// execution.
	ClientPackage *SwarmingRpcsCipdPackage `json:"client_package,omitempty"`

	// Packages: A CIPD package to install in the run dir before task
	// execution.
	Packages []*SwarmingRpcsCipdPackage `json:"packages,omitempty"`

	Server string `json:"server,omitempty"`

	// ForceSendFields is a list of field names (e.g. "ClientPackage") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "ClientPackage") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsCipdInput) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsCipdInput
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsCipdPackage: A CIPD package to install in the run dir
// before task execution.
type SwarmingRpcsCipdPackage struct {
	PackageName string `json:"package_name,omitempty"`

	Path string `json:"path,omitempty"`

	Version string `json:"version,omitempty"`

	// ForceSendFields is a list of field names (e.g. "PackageName") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "PackageName") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsCipdPackage) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsCipdPackage
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsCipdPins: Defines pinned CIPD packages that were
// installed during the task.
type SwarmingRpcsCipdPins struct {
	// ClientPackage: A CIPD package to install in the run dir before task
	// execution.
	ClientPackage *SwarmingRpcsCipdPackage `json:"client_package,omitempty"`

	// Packages: A CIPD package to install in the run dir before task
	// execution.
	Packages []*SwarmingRpcsCipdPackage `json:"packages,omitempty"`

	// ForceSendFields is a list of field names (e.g. "ClientPackage") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "ClientPackage") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsCipdPins) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsCipdPins
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsClientPermissions: Reports the client's permissions.
type SwarmingRpcsClientPermissions struct {
	CancelTask bool `json:"cancel_task,omitempty"`

	CancelTasks bool `json:"cancel_tasks,omitempty"`

	DeleteBot bool `json:"delete_bot,omitempty"`

	GetBootstrapToken bool `json:"get_bootstrap_token,omitempty"`

	GetConfigs bool `json:"get_configs,omitempty"`

	PutConfigs bool `json:"put_configs,omitempty"`

	TerminateBot bool `json:"terminate_bot,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "CancelTask") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "CancelTask") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsClientPermissions) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsClientPermissions
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsDeletedResponse: Indicates whether a bot was deleted.
type SwarmingRpcsDeletedResponse struct {
	Deleted bool `json:"deleted,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Deleted") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Deleted") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsDeletedResponse) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsDeletedResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsFileContent: Content of a file.
type SwarmingRpcsFileContent struct {
	Content string `json:"content,omitempty"`

	Version string `json:"version,omitempty"`

	When string `json:"when,omitempty"`

	Who string `json:"who,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Content") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Content") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsFileContent) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsFileContent
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsFileContentRequest: Content of a file.
type SwarmingRpcsFileContentRequest struct {
	Content string `json:"content,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Content") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Content") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsFileContentRequest) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsFileContentRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsFilesRef: Defines a data tree reference, normally a
// reference to a .isolated file.
type SwarmingRpcsFilesRef struct {
	Isolated string `json:"isolated,omitempty"`

	Isolatedserver string `json:"isolatedserver,omitempty"`

	Namespace string `json:"namespace,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Isolated") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Isolated") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsFilesRef) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsFilesRef
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsNewTaskRequest: Description of a new task request as
// described by the client. This message is used to create a new task.
type SwarmingRpcsNewTaskRequest struct {
	ExpirationSecs int64 `json:"expiration_secs,omitempty,string"`

	Name string `json:"name,omitempty"`

	ParentTaskId string `json:"parent_task_id,omitempty"`

	Priority int64 `json:"priority,omitempty,string"`

	// Properties: Important metadata about a particular task.
	Properties *SwarmingRpcsTaskProperties `json:"properties,omitempty"`

	PubsubAuthToken string `json:"pubsub_auth_token,omitempty"`

	PubsubTopic string `json:"pubsub_topic,omitempty"`

	PubsubUserdata string `json:"pubsub_userdata,omitempty"`

	ServiceAccountToken string `json:"service_account_token,omitempty"`

	Tags []string `json:"tags,omitempty"`

	User string `json:"user,omitempty"`

	// ForceSendFields is a list of field names (e.g. "ExpirationSecs") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "ExpirationSecs") to
	// include in API requests with the JSON null value. By default, fields
	// with empty values are omitted from API requests. However, any field
	// with an empty value appearing in NullFields will be sent to the
	// server as null. It is an error if a field in this list has a
	// non-empty value. This may be used to include null fields in Patch
	// requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsNewTaskRequest) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsNewTaskRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

type SwarmingRpcsOperationStats struct {
	Duration float64 `json:"duration,omitempty"`

	InitialNumberItems int64 `json:"initial_number_items,omitempty,string"`

	InitialSize int64 `json:"initial_size,omitempty,string"`

	ItemsCold string `json:"items_cold,omitempty"`

	ItemsHot string `json:"items_hot,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Duration") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Duration") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsOperationStats) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsOperationStats
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

func (s *SwarmingRpcsOperationStats) UnmarshalJSON(data []byte) error {
	type noMethod SwarmingRpcsOperationStats
	var s1 struct {
		Duration gensupport.JSONFloat64 `json:"duration"`
		*noMethod
	}
	s1.noMethod = (*noMethod)(s)
	if err := json.Unmarshal(data, &s1); err != nil {
		return err
	}
	s.Duration = float64(s1.Duration)
	return nil
}

type SwarmingRpcsPerformanceStats struct {
	BotOverhead float64 `json:"bot_overhead,omitempty"`

	IsolatedDownload *SwarmingRpcsOperationStats `json:"isolated_download,omitempty"`

	IsolatedUpload *SwarmingRpcsOperationStats `json:"isolated_upload,omitempty"`

	// ForceSendFields is a list of field names (e.g. "BotOverhead") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BotOverhead") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsPerformanceStats) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsPerformanceStats
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

func (s *SwarmingRpcsPerformanceStats) UnmarshalJSON(data []byte) error {
	type noMethod SwarmingRpcsPerformanceStats
	var s1 struct {
		BotOverhead gensupport.JSONFloat64 `json:"bot_overhead"`
		*noMethod
	}
	s1.noMethod = (*noMethod)(s)
	if err := json.Unmarshal(data, &s1); err != nil {
		return err
	}
	s.BotOverhead = float64(s1.BotOverhead)
	return nil
}

// SwarmingRpcsServerDetails: Reports details about the server.
type SwarmingRpcsServerDetails struct {
	BotVersion string `json:"bot_version,omitempty"`

	DefaultIsolateNamespace string `json:"default_isolate_namespace,omitempty"`

	DefaultIsolateServer string `json:"default_isolate_server,omitempty"`

	DisplayServerUrlTemplate string `json:"display_server_url_template,omitempty"`

	LuciConfig string `json:"luci_config,omitempty"`

	MachineProviderTemplate string `json:"machine_provider_template,omitempty"`

	ServerVersion string `json:"server_version,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "BotVersion") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "BotVersion") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsServerDetails) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsServerDetails
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsStringListPair: Represents a mapping of string to list of
// strings.
type SwarmingRpcsStringListPair struct {
	Key string `json:"key,omitempty"`

	Value []string `json:"value,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Key") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Key") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsStringListPair) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsStringListPair
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsStringPair: Represents a mapping of string to string.
type SwarmingRpcsStringPair struct {
	Key string `json:"key,omitempty"`

	Value string `json:"value,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Key") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Key") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsStringPair) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsStringPair
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTaskList: Wraps a list of TaskResult.
type SwarmingRpcsTaskList struct {
	Cursor string `json:"cursor,omitempty"`

	// Items: Representation of the TaskResultSummary or TaskRunResult ndb
	// model.
	Items []*SwarmingRpcsTaskResult `json:"items,omitempty"`

	Now string `json:"now,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Cursor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Cursor") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTaskList) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTaskList
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTaskOutput: A task's output as a string.
type SwarmingRpcsTaskOutput struct {
	Output string `json:"output,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Output") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Output") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTaskOutput) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTaskOutput
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTaskProperties: Important metadata about a particular
// task.
type SwarmingRpcsTaskProperties struct {
	// Caches: Describes a named cache that should be present on the bot. A
	// CacheEntry in a task specified that the task prefers the cache to be
	// present on the bot. A symlink to the cache directory is created at
	// /|path|. If cache is not present on the machine, the directory is
	// empty. If the tasks makes any changes to the contents of the cache
	// directory, they are persisted on the machine. If another task runs on
	// the same machine and requests the same named cache, even if mapped to
	// a different path, it will see the changes.
	Caches []*SwarmingRpcsCacheEntry `json:"caches,omitempty"`

	// CipdInput: Defines CIPD packages to install in task run directory.
	CipdInput *SwarmingRpcsCipdInput `json:"cipd_input,omitempty"`

	Command []string `json:"command,omitempty"`

	// Dimensions: Represents a mapping of string to string.
	Dimensions []*SwarmingRpcsStringPair `json:"dimensions,omitempty"`

	// Env: Represents a mapping of string to string.
	Env []*SwarmingRpcsStringPair `json:"env,omitempty"`

	ExecutionTimeoutSecs int64 `json:"execution_timeout_secs,omitempty,string"`

	ExtraArgs []string `json:"extra_args,omitempty"`

	GracePeriodSecs int64 `json:"grace_period_secs,omitempty,string"`

	Idempotent bool `json:"idempotent,omitempty"`

	// InputsRef: Defines a data tree reference, normally a reference to a
	// .isolated file.
	InputsRef *SwarmingRpcsFilesRef `json:"inputs_ref,omitempty"`

	IoTimeoutSecs int64 `json:"io_timeout_secs,omitempty,string"`

	Outputs []string `json:"outputs,omitempty"`

	SecretBytes string `json:"secret_bytes,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Caches") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Caches") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTaskProperties) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTaskProperties
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTaskRequest: Description of a task request as registered
// by the server. This message is used when retrieving information about
// an existing task. See NewtaskRequest for more details.
type SwarmingRpcsTaskRequest struct {
	Authenticated string `json:"authenticated,omitempty"`

	CreatedTs string `json:"created_ts,omitempty"`

	ExpirationSecs int64 `json:"expiration_secs,omitempty,string"`

	Name string `json:"name,omitempty"`

	ParentTaskId string `json:"parent_task_id,omitempty"`

	Priority int64 `json:"priority,omitempty,string"`

	// Properties: Important metadata about a particular task.
	Properties *SwarmingRpcsTaskProperties `json:"properties,omitempty"`

	PubsubTopic string `json:"pubsub_topic,omitempty"`

	PubsubUserdata string `json:"pubsub_userdata,omitempty"`

	ServiceAccount string `json:"service_account,omitempty"`

	Tags []string `json:"tags,omitempty"`

	User string `json:"user,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Authenticated") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Authenticated") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTaskRequest) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTaskRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTaskRequestMetadata: Provides the ID of the requested
// TaskRequest.
type SwarmingRpcsTaskRequestMetadata struct {
	// Request: Description of a task request as registered by the server.
	// This message is used when retrieving information about an existing
	// task. See NewtaskRequest for more details.
	Request *SwarmingRpcsTaskRequest `json:"request,omitempty"`

	TaskId string `json:"task_id,omitempty"`

	// TaskResult: Representation of the TaskResultSummary or TaskRunResult
	// ndb model.
	TaskResult *SwarmingRpcsTaskResult `json:"task_result,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Request") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Request") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTaskRequestMetadata) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTaskRequestMetadata
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTaskRequests: Wraps a list of TaskRequest.
type SwarmingRpcsTaskRequests struct {
	Cursor string `json:"cursor,omitempty"`

	// Items: Description of a task request as registered by the server.
	// This message is used when retrieving information about an existing
	// task. See NewtaskRequest for more details.
	Items []*SwarmingRpcsTaskRequest `json:"items,omitempty"`

	Now string `json:"now,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Cursor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Cursor") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTaskRequests) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTaskRequests
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTaskResult: Representation of the TaskResultSummary or
// TaskRunResult ndb model.
type SwarmingRpcsTaskResult struct {
	AbandonedTs string `json:"abandoned_ts,omitempty"`

	// BotDimensions: Represents a mapping of string to list of strings.
	BotDimensions []*SwarmingRpcsStringListPair `json:"bot_dimensions,omitempty"`

	BotId string `json:"bot_id,omitempty"`

	BotVersion string `json:"bot_version,omitempty"`

	ChildrenTaskIds []string `json:"children_task_ids,omitempty"`

	// CipdPins: Defines pinned CIPD packages that were installed during the
	// task.
	CipdPins *SwarmingRpcsCipdPins `json:"cipd_pins,omitempty"`

	CompletedTs string `json:"completed_ts,omitempty"`

	CostSavedUsd float64 `json:"cost_saved_usd,omitempty"`

	CostsUsd []float64 `json:"costs_usd,omitempty"`

	CreatedTs string `json:"created_ts,omitempty"`

	DedupedFrom string `json:"deduped_from,omitempty"`

	Duration float64 `json:"duration,omitempty"`

	ExitCode int64 `json:"exit_code,omitempty,string"`

	Failure bool `json:"failure,omitempty"`

	InternalFailure bool `json:"internal_failure,omitempty"`

	ModifiedTs string `json:"modified_ts,omitempty"`

	Name string `json:"name,omitempty"`

	// OutputsRef: Defines a data tree reference, normally a reference to a
	// .isolated file.
	OutputsRef *SwarmingRpcsFilesRef `json:"outputs_ref,omitempty"`

	PerformanceStats *SwarmingRpcsPerformanceStats `json:"performance_stats,omitempty"`

	PropertiesHash string `json:"properties_hash,omitempty"`

	RunId string `json:"run_id,omitempty"`

	ServerVersions []string `json:"server_versions,omitempty"`

	StartedTs string `json:"started_ts,omitempty"`

	// Possible values:
	//   "BOT_DIED"
	//   "CANCELED"
	//   "COMPLETED"
	//   "EXPIRED"
	//   "PENDING"
	//   "RUNNING"
	//   "TIMED_OUT"
	State string `json:"state,omitempty"`

	Tags []string `json:"tags,omitempty"`

	TaskId string `json:"task_id,omitempty"`

	TryNumber int64 `json:"try_number,omitempty,string"`

	User string `json:"user,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "AbandonedTs") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "AbandonedTs") to include
	// in API requests with the JSON null value. By default, fields with
	// empty values are omitted from API requests. However, any field with
	// an empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTaskResult) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTaskResult
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

func (s *SwarmingRpcsTaskResult) UnmarshalJSON(data []byte) error {
	type noMethod SwarmingRpcsTaskResult
	var s1 struct {
		CostSavedUsd gensupport.JSONFloat64 `json:"cost_saved_usd"`
		Duration     gensupport.JSONFloat64 `json:"duration"`
		*noMethod
	}
	s1.noMethod = (*noMethod)(s)
	if err := json.Unmarshal(data, &s1); err != nil {
		return err
	}
	s.CostSavedUsd = float64(s1.CostSavedUsd)
	s.Duration = float64(s1.Duration)
	return nil
}

// SwarmingRpcsTasksCancelRequest: Request to cancel some subset of
// pending tasks.
type SwarmingRpcsTasksCancelRequest struct {
	Cursor string `json:"cursor,omitempty"`

	Limit int64 `json:"limit,omitempty,string"`

	Tags []string `json:"tags,omitempty"`

	// ForceSendFields is a list of field names (e.g. "Cursor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Cursor") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTasksCancelRequest) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTasksCancelRequest
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTasksCancelResponse: Result of canceling some subset of
// pending tasks.
type SwarmingRpcsTasksCancelResponse struct {
	Cursor string `json:"cursor,omitempty"`

	Matched int64 `json:"matched,omitempty,string"`

	Now string `json:"now,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Cursor") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Cursor") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTasksCancelResponse) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTasksCancelResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTasksCount: Returns the count, as requested.
type SwarmingRpcsTasksCount struct {
	Count int64 `json:"count,omitempty,string"`

	Now string `json:"now,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "Count") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "Count") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTasksCount) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTasksCount
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTasksTags: Returns all the tags and tag possibilities in
// the fleet.
type SwarmingRpcsTasksTags struct {
	// TasksTags: Represents a mapping of string to list of strings.
	TasksTags []*SwarmingRpcsStringListPair `json:"tasks_tags,omitempty"`

	Ts string `json:"ts,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "TasksTags") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "TasksTags") to include in
	// API requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTasksTags) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTasksTags
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// SwarmingRpcsTerminateResponse: Returns the pseudo taskid to wait for
// the bot to shut down.
type SwarmingRpcsTerminateResponse struct {
	TaskId string `json:"task_id,omitempty"`

	// ServerResponse contains the HTTP response code and headers from the
	// server.
	googleapi.ServerResponse `json:"-"`

	// ForceSendFields is a list of field names (e.g. "TaskId") to
	// unconditionally include in API requests. By default, fields with
	// empty values are omitted from API requests. However, any non-pointer,
	// non-interface field appearing in ForceSendFields will be sent to the
	// server regardless of whether the field is empty or not. This may be
	// used to include empty fields in Patch requests.
	ForceSendFields []string `json:"-"`

	// NullFields is a list of field names (e.g. "TaskId") to include in API
	// requests with the JSON null value. By default, fields with empty
	// values are omitted from API requests. However, any field with an
	// empty value appearing in NullFields will be sent to the server as
	// null. It is an error if a field in this list has a non-empty value.
	// This may be used to include null fields in Patch requests.
	NullFields []string `json:"-"`
}

func (s *SwarmingRpcsTerminateResponse) MarshalJSON() ([]byte, error) {
	type noMethod SwarmingRpcsTerminateResponse
	raw := noMethod(*s)
	return gensupport.MarshalJSON(raw, s.ForceSendFields, s.NullFields)
}

// method id "swarming.bot.delete":

type BotDeleteCall struct {
	s          *Service
	botId      string
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	header_    http.Header
}

// Delete: Deletes the bot corresponding to a provided bot_id. At that
// point, the bot will not appears in the list of bots but it is still
// possible to get information about the bot with its bot id is known,
// as historical data is not deleted. It is meant to remove from the DB
// the presence of a bot that was retired, e.g. the VM was shut down
// already. Use 'terminate' instead of the bot is still alive.
func (r *BotService) Delete(botId string) *BotDeleteCall {
	c := &BotDeleteCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.botId = botId
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *BotDeleteCall) Fields(s ...googleapi.Field) *BotDeleteCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *BotDeleteCall) Context(ctx context.Context) *BotDeleteCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *BotDeleteCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *BotDeleteCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "bot/{bot_id}/delete")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"bot_id": c.botId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.bot.delete" call.
// Exactly one of *SwarmingRpcsDeletedResponse or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsDeletedResponse.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *BotDeleteCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsDeletedResponse, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsDeletedResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Deletes the bot corresponding to a provided bot_id. At that point, the bot will not appears in the list of bots but it is still possible to get information about the bot with its bot id is known, as historical data is not deleted. It is meant to remove from the DB the presence of a bot that was retired, e.g. the VM was shut down already. Use 'terminate' instead of the bot is still alive.",
	//   "httpMethod": "POST",
	//   "id": "swarming.bot.delete",
	//   "parameterOrder": [
	//     "bot_id"
	//   ],
	//   "parameters": {
	//     "bot_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "bot/{bot_id}/delete",
	//   "response": {
	//     "$ref": "SwarmingRpcsDeletedResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.bot.events":

type BotEventsCall struct {
	s            *Service
	botId        string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Events: Returns events that happened on a bot.
func (r *BotService) Events(botId string) *BotEventsCall {
	c := &BotEventsCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.botId = botId
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *BotEventsCall) Fields(s ...googleapi.Field) *BotEventsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *BotEventsCall) IfNoneMatch(entityTag string) *BotEventsCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *BotEventsCall) Context(ctx context.Context) *BotEventsCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *BotEventsCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *BotEventsCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "bot/{bot_id}/events")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"bot_id": c.botId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.bot.events" call.
// Exactly one of *SwarmingRpcsBotEvents or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsBotEvents.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *BotEventsCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsBotEvents, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsBotEvents{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns events that happened on a bot.",
	//   "httpMethod": "GET",
	//   "id": "swarming.bot.events",
	//   "parameterOrder": [
	//     "bot_id"
	//   ],
	//   "parameters": {
	//     "bot_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "bot/{bot_id}/events",
	//   "response": {
	//     "$ref": "SwarmingRpcsBotEvents"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.bot.get":

type BotGetCall struct {
	s            *Service
	botId        string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Get: Returns information about a known bot. This includes its state
// and dimensions, and if it is currently running a task.
func (r *BotService) Get(botId string) *BotGetCall {
	c := &BotGetCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.botId = botId
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *BotGetCall) Fields(s ...googleapi.Field) *BotGetCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *BotGetCall) IfNoneMatch(entityTag string) *BotGetCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *BotGetCall) Context(ctx context.Context) *BotGetCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *BotGetCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *BotGetCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "bot/{bot_id}/get")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"bot_id": c.botId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.bot.get" call.
// Exactly one of *SwarmingRpcsBotInfo or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsBotInfo.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *BotGetCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsBotInfo, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsBotInfo{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns information about a known bot. This includes its state and dimensions, and if it is currently running a task.",
	//   "httpMethod": "GET",
	//   "id": "swarming.bot.get",
	//   "parameterOrder": [
	//     "bot_id"
	//   ],
	//   "parameters": {
	//     "bot_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "bot/{bot_id}/get",
	//   "response": {
	//     "$ref": "SwarmingRpcsBotInfo"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.bot.tasks":

type BotTasksCall struct {
	s            *Service
	botId        string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Tasks: Lists a given bot's tasks within the specified date range. In
// this case, the tasks are effectively TaskRunResult since it's
// individual task tries sent to this specific bot. It is impossible to
// search by both tags and bot id. If there's a need, TaskRunResult.tags
// will be added (via a copy from TaskRequest.tags).
func (r *BotService) Tasks(botId string) *BotTasksCall {
	c := &BotTasksCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.botId = botId
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *BotTasksCall) Fields(s ...googleapi.Field) *BotTasksCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *BotTasksCall) IfNoneMatch(entityTag string) *BotTasksCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *BotTasksCall) Context(ctx context.Context) *BotTasksCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *BotTasksCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *BotTasksCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "bot/{bot_id}/tasks")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"bot_id": c.botId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.bot.tasks" call.
// Exactly one of *SwarmingRpcsBotTasks or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsBotTasks.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *BotTasksCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsBotTasks, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsBotTasks{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Lists a given bot's tasks within the specified date range. In this case, the tasks are effectively TaskRunResult since it's individual task tries sent to this specific bot. It is impossible to search by both tags and bot id. If there's a need, TaskRunResult.tags will be added (via a copy from TaskRequest.tags).",
	//   "httpMethod": "GET",
	//   "id": "swarming.bot.tasks",
	//   "parameterOrder": [
	//     "bot_id"
	//   ],
	//   "parameters": {
	//     "bot_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "bot/{bot_id}/tasks",
	//   "response": {
	//     "$ref": "SwarmingRpcsBotTasks"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.bot.terminate":

type BotTerminateCall struct {
	s          *Service
	botId      string
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	header_    http.Header
}

// Terminate: Asks a bot to terminate itself gracefully. The bot will
// stay in the DB, use 'delete' to remove it from the DB afterward. This
// request returns a pseudo-taskid that can be waited for to wait for
// the bot to turn down. This command is particularly useful when a
// privileged user needs to safely debug a machine specific issue. The
// user can trigger a terminate for one of the bot exhibiting the issue,
// wait for the pseudo-task to run then access the machine with the
// guarantee that the bot is not running anymore.
func (r *BotService) Terminate(botId string) *BotTerminateCall {
	c := &BotTerminateCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.botId = botId
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *BotTerminateCall) Fields(s ...googleapi.Field) *BotTerminateCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *BotTerminateCall) Context(ctx context.Context) *BotTerminateCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *BotTerminateCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *BotTerminateCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "bot/{bot_id}/terminate")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"bot_id": c.botId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.bot.terminate" call.
// Exactly one of *SwarmingRpcsTerminateResponse or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *SwarmingRpcsTerminateResponse.ServerResponse.Header or (if a
// response was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *BotTerminateCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTerminateResponse, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTerminateResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Asks a bot to terminate itself gracefully. The bot will stay in the DB, use 'delete' to remove it from the DB afterward. This request returns a pseudo-taskid that can be waited for to wait for the bot to turn down. This command is particularly useful when a privileged user needs to safely debug a machine specific issue. The user can trigger a terminate for one of the bot exhibiting the issue, wait for the pseudo-task to run then access the machine with the guarantee that the bot is not running anymore.",
	//   "httpMethod": "POST",
	//   "id": "swarming.bot.terminate",
	//   "parameterOrder": [
	//     "bot_id"
	//   ],
	//   "parameters": {
	//     "bot_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "bot/{bot_id}/terminate",
	//   "response": {
	//     "$ref": "SwarmingRpcsTerminateResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.bots.count":

type BotsCountCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Count: Counts number of bots with given dimensions.
func (r *BotsService) Count() *BotsCountCall {
	c := &BotsCountCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Cursor sets the optional parameter "cursor":
func (c *BotsCountCall) Cursor(cursor string) *BotsCountCall {
	c.urlParams_.Set("cursor", cursor)
	return c
}

// Dimensions sets the optional parameter "dimensions":
func (c *BotsCountCall) Dimensions(dimensions ...string) *BotsCountCall {
	c.urlParams_.SetMulti("dimensions", append([]string{}, dimensions...))
	return c
}

// IsBusy sets the optional parameter "is_busy":
//
// Possible values:
//   "FALSE"
//   "NONE" (default)
//   "TRUE"
func (c *BotsCountCall) IsBusy(isBusy string) *BotsCountCall {
	c.urlParams_.Set("is_busy", isBusy)
	return c
}

// IsDead sets the optional parameter "is_dead":
//
// Possible values:
//   "FALSE"
//   "NONE" (default)
//   "TRUE"
func (c *BotsCountCall) IsDead(isDead string) *BotsCountCall {
	c.urlParams_.Set("is_dead", isDead)
	return c
}

// IsMp sets the optional parameter "is_mp":
//
// Possible values:
//   "FALSE"
//   "NONE" (default)
//   "TRUE"
func (c *BotsCountCall) IsMp(isMp string) *BotsCountCall {
	c.urlParams_.Set("is_mp", isMp)
	return c
}

// Limit sets the optional parameter "limit":
func (c *BotsCountCall) Limit(limit int64) *BotsCountCall {
	c.urlParams_.Set("limit", fmt.Sprint(limit))
	return c
}

// Quarantined sets the optional parameter "quarantined":
//
// Possible values:
//   "FALSE"
//   "NONE" (default)
//   "TRUE"
func (c *BotsCountCall) Quarantined(quarantined string) *BotsCountCall {
	c.urlParams_.Set("quarantined", quarantined)
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *BotsCountCall) Fields(s ...googleapi.Field) *BotsCountCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *BotsCountCall) IfNoneMatch(entityTag string) *BotsCountCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *BotsCountCall) Context(ctx context.Context) *BotsCountCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *BotsCountCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *BotsCountCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "bots/count")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.bots.count" call.
// Exactly one of *SwarmingRpcsBotsCount or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsBotsCount.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *BotsCountCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsBotsCount, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsBotsCount{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Counts number of bots with given dimensions.",
	//   "httpMethod": "GET",
	//   "id": "swarming.bots.count",
	//   "parameters": {
	//     "cursor": {
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "dimensions": {
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     },
	//     "is_busy": {
	//       "default": "NONE",
	//       "enum": [
	//         "FALSE",
	//         "NONE",
	//         "TRUE"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "is_dead": {
	//       "default": "NONE",
	//       "enum": [
	//         "FALSE",
	//         "NONE",
	//         "TRUE"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "is_mp": {
	//       "default": "NONE",
	//       "enum": [
	//         "FALSE",
	//         "NONE",
	//         "TRUE"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "limit": {
	//       "default": "200",
	//       "format": "int64",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "quarantined": {
	//       "default": "NONE",
	//       "enum": [
	//         "FALSE",
	//         "NONE",
	//         "TRUE"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "bots/count",
	//   "response": {
	//     "$ref": "SwarmingRpcsBotsCount"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.bots.dimensions":

type BotsDimensionsCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Dimensions: Returns the cached set of dimensions currently in use in
// the fleet.
func (r *BotsService) Dimensions() *BotsDimensionsCall {
	c := &BotsDimensionsCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *BotsDimensionsCall) Fields(s ...googleapi.Field) *BotsDimensionsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *BotsDimensionsCall) IfNoneMatch(entityTag string) *BotsDimensionsCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *BotsDimensionsCall) Context(ctx context.Context) *BotsDimensionsCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *BotsDimensionsCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *BotsDimensionsCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "bots/dimensions")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.bots.dimensions" call.
// Exactly one of *SwarmingRpcsBotsDimensions or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsBotsDimensions.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *BotsDimensionsCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsBotsDimensions, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsBotsDimensions{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns the cached set of dimensions currently in use in the fleet.",
	//   "httpMethod": "GET",
	//   "id": "swarming.bots.dimensions",
	//   "path": "bots/dimensions",
	//   "response": {
	//     "$ref": "SwarmingRpcsBotsDimensions"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.bots.list":

type BotsListCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// List: Provides list of known bots. Deleted bots will not be listed.
func (r *BotsService) List() *BotsListCall {
	c := &BotsListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Cursor sets the optional parameter "cursor":
func (c *BotsListCall) Cursor(cursor string) *BotsListCall {
	c.urlParams_.Set("cursor", cursor)
	return c
}

// Dimensions sets the optional parameter "dimensions":
func (c *BotsListCall) Dimensions(dimensions ...string) *BotsListCall {
	c.urlParams_.SetMulti("dimensions", append([]string{}, dimensions...))
	return c
}

// IsBusy sets the optional parameter "is_busy":
//
// Possible values:
//   "FALSE"
//   "NONE" (default)
//   "TRUE"
func (c *BotsListCall) IsBusy(isBusy string) *BotsListCall {
	c.urlParams_.Set("is_busy", isBusy)
	return c
}

// IsDead sets the optional parameter "is_dead":
//
// Possible values:
//   "FALSE"
//   "NONE" (default)
//   "TRUE"
func (c *BotsListCall) IsDead(isDead string) *BotsListCall {
	c.urlParams_.Set("is_dead", isDead)
	return c
}

// IsMp sets the optional parameter "is_mp":
//
// Possible values:
//   "FALSE"
//   "NONE" (default)
//   "TRUE"
func (c *BotsListCall) IsMp(isMp string) *BotsListCall {
	c.urlParams_.Set("is_mp", isMp)
	return c
}

// Limit sets the optional parameter "limit":
func (c *BotsListCall) Limit(limit int64) *BotsListCall {
	c.urlParams_.Set("limit", fmt.Sprint(limit))
	return c
}

// Quarantined sets the optional parameter "quarantined":
//
// Possible values:
//   "FALSE"
//   "NONE" (default)
//   "TRUE"
func (c *BotsListCall) Quarantined(quarantined string) *BotsListCall {
	c.urlParams_.Set("quarantined", quarantined)
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *BotsListCall) Fields(s ...googleapi.Field) *BotsListCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *BotsListCall) IfNoneMatch(entityTag string) *BotsListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *BotsListCall) Context(ctx context.Context) *BotsListCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *BotsListCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *BotsListCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "bots/list")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.bots.list" call.
// Exactly one of *SwarmingRpcsBotList or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsBotList.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *BotsListCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsBotList, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsBotList{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Provides list of known bots. Deleted bots will not be listed.",
	//   "httpMethod": "GET",
	//   "id": "swarming.bots.list",
	//   "parameters": {
	//     "cursor": {
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "dimensions": {
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     },
	//     "is_busy": {
	//       "default": "NONE",
	//       "enum": [
	//         "FALSE",
	//         "NONE",
	//         "TRUE"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "is_dead": {
	//       "default": "NONE",
	//       "enum": [
	//         "FALSE",
	//         "NONE",
	//         "TRUE"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "is_mp": {
	//       "default": "NONE",
	//       "enum": [
	//         "FALSE",
	//         "NONE",
	//         "TRUE"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "limit": {
	//       "default": "200",
	//       "format": "int64",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "quarantined": {
	//       "default": "NONE",
	//       "enum": [
	//         "FALSE",
	//         "NONE",
	//         "TRUE"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "bots/list",
	//   "response": {
	//     "$ref": "SwarmingRpcsBotList"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.server.details":

type ServerDetailsCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Details: Returns information about the server.
func (r *ServerService) Details() *ServerDetailsCall {
	c := &ServerDetailsCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ServerDetailsCall) Fields(s ...googleapi.Field) *ServerDetailsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ServerDetailsCall) IfNoneMatch(entityTag string) *ServerDetailsCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ServerDetailsCall) Context(ctx context.Context) *ServerDetailsCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ServerDetailsCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ServerDetailsCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "server/details")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.server.details" call.
// Exactly one of *SwarmingRpcsServerDetails or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsServerDetails.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ServerDetailsCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsServerDetails, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsServerDetails{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns information about the server.",
	//   "httpMethod": "GET",
	//   "id": "swarming.server.details",
	//   "path": "server/details",
	//   "response": {
	//     "$ref": "SwarmingRpcsServerDetails"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.server.get_bootstrap":

type ServerGetBootstrapCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// GetBootstrap: Retrieves the current or a previous version of
// bootstrap.py. When the file is sourced via luci-config, the version
// parameter is ignored. Eventually the support for 'version' will be
// removed completely.
func (r *ServerService) GetBootstrap() *ServerGetBootstrapCall {
	c := &ServerGetBootstrapCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Version sets the optional parameter "version":
func (c *ServerGetBootstrapCall) Version(version int64) *ServerGetBootstrapCall {
	c.urlParams_.Set("version", fmt.Sprint(version))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ServerGetBootstrapCall) Fields(s ...googleapi.Field) *ServerGetBootstrapCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ServerGetBootstrapCall) IfNoneMatch(entityTag string) *ServerGetBootstrapCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ServerGetBootstrapCall) Context(ctx context.Context) *ServerGetBootstrapCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ServerGetBootstrapCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ServerGetBootstrapCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "server/get_bootstrap")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.server.get_bootstrap" call.
// Exactly one of *SwarmingRpcsFileContent or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsFileContent.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ServerGetBootstrapCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsFileContent, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsFileContent{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Retrieves the current or a previous version of bootstrap.py. When the file is sourced via luci-config, the version parameter is ignored. Eventually the support for 'version' will be removed completely.",
	//   "httpMethod": "GET",
	//   "id": "swarming.server.get_bootstrap",
	//   "parameters": {
	//     "version": {
	//       "format": "int64",
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "server/get_bootstrap",
	//   "response": {
	//     "$ref": "SwarmingRpcsFileContent"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.server.get_bot_config":

type ServerGetBotConfigCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// GetBotConfig: Retrieves the current or a previous version of
// bot_config.py. When the file is sourced via luci-config, the version
// parameter is ignored. Eventually the support for 'version' will be
// removed completely.
func (r *ServerService) GetBotConfig() *ServerGetBotConfigCall {
	c := &ServerGetBotConfigCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Version sets the optional parameter "version":
func (c *ServerGetBotConfigCall) Version(version int64) *ServerGetBotConfigCall {
	c.urlParams_.Set("version", fmt.Sprint(version))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ServerGetBotConfigCall) Fields(s ...googleapi.Field) *ServerGetBotConfigCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ServerGetBotConfigCall) IfNoneMatch(entityTag string) *ServerGetBotConfigCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ServerGetBotConfigCall) Context(ctx context.Context) *ServerGetBotConfigCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ServerGetBotConfigCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ServerGetBotConfigCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "server/get_bot_config")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.server.get_bot_config" call.
// Exactly one of *SwarmingRpcsFileContent or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsFileContent.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ServerGetBotConfigCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsFileContent, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsFileContent{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Retrieves the current or a previous version of bot_config.py. When the file is sourced via luci-config, the version parameter is ignored. Eventually the support for 'version' will be removed completely.",
	//   "httpMethod": "GET",
	//   "id": "swarming.server.get_bot_config",
	//   "parameters": {
	//     "version": {
	//       "format": "int64",
	//       "location": "query",
	//       "type": "string"
	//     }
	//   },
	//   "path": "server/get_bot_config",
	//   "response": {
	//     "$ref": "SwarmingRpcsFileContent"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.server.permissions":

type ServerPermissionsCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Permissions: Returns the caller's permissions.
func (r *ServerService) Permissions() *ServerPermissionsCall {
	c := &ServerPermissionsCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ServerPermissionsCall) Fields(s ...googleapi.Field) *ServerPermissionsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *ServerPermissionsCall) IfNoneMatch(entityTag string) *ServerPermissionsCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ServerPermissionsCall) Context(ctx context.Context) *ServerPermissionsCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ServerPermissionsCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ServerPermissionsCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "server/permissions")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.server.permissions" call.
// Exactly one of *SwarmingRpcsClientPermissions or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *SwarmingRpcsClientPermissions.ServerResponse.Header or (if a
// response was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ServerPermissionsCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsClientPermissions, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsClientPermissions{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns the caller's permissions.",
	//   "httpMethod": "GET",
	//   "id": "swarming.server.permissions",
	//   "path": "server/permissions",
	//   "response": {
	//     "$ref": "SwarmingRpcsClientPermissions"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.server.put_bootstrap":

type ServerPutBootstrapCall struct {
	s                              *Service
	swarmingrpcsfilecontentrequest *SwarmingRpcsFileContentRequest
	urlParams_                     gensupport.URLParams
	ctx_                           context.Context
	header_                        http.Header
}

// PutBootstrap: Stores a new version of bootstrap.py. Warning: if a
// file exists in luci-config, the file stored by this function is
// ignored. Uploads are not blocked in case the file is later deleted
// from luci-config.
func (r *ServerService) PutBootstrap(swarmingrpcsfilecontentrequest *SwarmingRpcsFileContentRequest) *ServerPutBootstrapCall {
	c := &ServerPutBootstrapCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.swarmingrpcsfilecontentrequest = swarmingrpcsfilecontentrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ServerPutBootstrapCall) Fields(s ...googleapi.Field) *ServerPutBootstrapCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ServerPutBootstrapCall) Context(ctx context.Context) *ServerPutBootstrapCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ServerPutBootstrapCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ServerPutBootstrapCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.swarmingrpcsfilecontentrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "server/put_bootstrap")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.server.put_bootstrap" call.
// Exactly one of *SwarmingRpcsFileContent or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsFileContent.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ServerPutBootstrapCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsFileContent, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsFileContent{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Stores a new version of bootstrap.py. Warning: if a file exists in luci-config, the file stored by this function is ignored. Uploads are not blocked in case the file is later deleted from luci-config.",
	//   "httpMethod": "POST",
	//   "id": "swarming.server.put_bootstrap",
	//   "path": "server/put_bootstrap",
	//   "request": {
	//     "$ref": "SwarmingRpcsFileContentRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "SwarmingRpcsFileContent"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.server.put_bot_config":

type ServerPutBotConfigCall struct {
	s                              *Service
	swarmingrpcsfilecontentrequest *SwarmingRpcsFileContentRequest
	urlParams_                     gensupport.URLParams
	ctx_                           context.Context
	header_                        http.Header
}

// PutBotConfig: Stores a new version of bot_config.py. Warning: if a
// file exists in luci-config, the file stored by this function is
// ignored. Uploads are not blocked in case the file is later deleted
// from luci-config.
func (r *ServerService) PutBotConfig(swarmingrpcsfilecontentrequest *SwarmingRpcsFileContentRequest) *ServerPutBotConfigCall {
	c := &ServerPutBotConfigCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.swarmingrpcsfilecontentrequest = swarmingrpcsfilecontentrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ServerPutBotConfigCall) Fields(s ...googleapi.Field) *ServerPutBotConfigCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ServerPutBotConfigCall) Context(ctx context.Context) *ServerPutBotConfigCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ServerPutBotConfigCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ServerPutBotConfigCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.swarmingrpcsfilecontentrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "server/put_bot_config")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.server.put_bot_config" call.
// Exactly one of *SwarmingRpcsFileContent or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsFileContent.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ServerPutBotConfigCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsFileContent, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsFileContent{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Stores a new version of bot_config.py. Warning: if a file exists in luci-config, the file stored by this function is ignored. Uploads are not blocked in case the file is later deleted from luci-config.",
	//   "httpMethod": "POST",
	//   "id": "swarming.server.put_bot_config",
	//   "path": "server/put_bot_config",
	//   "request": {
	//     "$ref": "SwarmingRpcsFileContentRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "SwarmingRpcsFileContent"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.server.token":

type ServerTokenCall struct {
	s          *Service
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	header_    http.Header
}

// Token: Returns a token to bootstrap a new bot.
func (r *ServerService) Token() *ServerTokenCall {
	c := &ServerTokenCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *ServerTokenCall) Fields(s ...googleapi.Field) *ServerTokenCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *ServerTokenCall) Context(ctx context.Context) *ServerTokenCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *ServerTokenCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *ServerTokenCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "server/token")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.server.token" call.
// Exactly one of *SwarmingRpcsBootstrapToken or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsBootstrapToken.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *ServerTokenCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsBootstrapToken, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsBootstrapToken{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns a token to bootstrap a new bot.",
	//   "httpMethod": "POST",
	//   "id": "swarming.server.token",
	//   "path": "server/token",
	//   "response": {
	//     "$ref": "SwarmingRpcsBootstrapToken"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.task.cancel":

type TaskCancelCall struct {
	s          *Service
	taskId     string
	urlParams_ gensupport.URLParams
	ctx_       context.Context
	header_    http.Header
}

// Cancel: Cancels a task. If a bot was running the task, the bot will
// forcibly cancel the task.
func (r *TaskService) Cancel(taskId string) *TaskCancelCall {
	c := &TaskCancelCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.taskId = taskId
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TaskCancelCall) Fields(s ...googleapi.Field) *TaskCancelCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TaskCancelCall) Context(ctx context.Context) *TaskCancelCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TaskCancelCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TaskCancelCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "task/{task_id}/cancel")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"task_id": c.taskId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.task.cancel" call.
// Exactly one of *SwarmingRpcsCancelResponse or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsCancelResponse.ServerResponse.Header or (if a response
// was returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TaskCancelCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsCancelResponse, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsCancelResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Cancels a task. If a bot was running the task, the bot will forcibly cancel the task.",
	//   "httpMethod": "POST",
	//   "id": "swarming.task.cancel",
	//   "parameterOrder": [
	//     "task_id"
	//   ],
	//   "parameters": {
	//     "task_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "task/{task_id}/cancel",
	//   "response": {
	//     "$ref": "SwarmingRpcsCancelResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.task.request":

type TaskRequestCall struct {
	s            *Service
	taskId       string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Request: Returns the task request corresponding to a task ID.
func (r *TaskService) Request(taskId string) *TaskRequestCall {
	c := &TaskRequestCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.taskId = taskId
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TaskRequestCall) Fields(s ...googleapi.Field) *TaskRequestCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *TaskRequestCall) IfNoneMatch(entityTag string) *TaskRequestCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TaskRequestCall) Context(ctx context.Context) *TaskRequestCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TaskRequestCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TaskRequestCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "task/{task_id}/request")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"task_id": c.taskId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.task.request" call.
// Exactly one of *SwarmingRpcsTaskRequest or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsTaskRequest.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TaskRequestCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTaskRequest, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTaskRequest{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns the task request corresponding to a task ID.",
	//   "httpMethod": "GET",
	//   "id": "swarming.task.request",
	//   "parameterOrder": [
	//     "task_id"
	//   ],
	//   "parameters": {
	//     "task_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "task/{task_id}/request",
	//   "response": {
	//     "$ref": "SwarmingRpcsTaskRequest"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.task.result":

type TaskResultCall struct {
	s            *Service
	taskId       string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Result: Reports the result of the task corresponding to a task ID. It
// can be a 'run' ID specifying a specific retry or a 'summary' ID
// hidding the fact that a task may have been retried transparently,
// when a bot reports BOT_DIED. A summary ID ends with '0', a run ID
// ends with '1' or '2'.
func (r *TaskService) Result(taskId string) *TaskResultCall {
	c := &TaskResultCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.taskId = taskId
	return c
}

// IncludePerformanceStats sets the optional parameter
// "include_performance_stats":
func (c *TaskResultCall) IncludePerformanceStats(includePerformanceStats bool) *TaskResultCall {
	c.urlParams_.Set("include_performance_stats", fmt.Sprint(includePerformanceStats))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TaskResultCall) Fields(s ...googleapi.Field) *TaskResultCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *TaskResultCall) IfNoneMatch(entityTag string) *TaskResultCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TaskResultCall) Context(ctx context.Context) *TaskResultCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TaskResultCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TaskResultCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "task/{task_id}/result")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"task_id": c.taskId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.task.result" call.
// Exactly one of *SwarmingRpcsTaskResult or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsTaskResult.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TaskResultCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTaskResult, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTaskResult{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Reports the result of the task corresponding to a task ID. It can be a 'run' ID specifying a specific retry or a 'summary' ID hidding the fact that a task may have been retried transparently, when a bot reports BOT_DIED. A summary ID ends with '0', a run ID ends with '1' or '2'.",
	//   "httpMethod": "GET",
	//   "id": "swarming.task.result",
	//   "parameterOrder": [
	//     "task_id"
	//   ],
	//   "parameters": {
	//     "include_performance_stats": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "task_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "task/{task_id}/result",
	//   "response": {
	//     "$ref": "SwarmingRpcsTaskResult"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.task.stdout":

type TaskStdoutCall struct {
	s            *Service
	taskId       string
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Stdout: Returns the output of the task corresponding to a task ID.
func (r *TaskService) Stdout(taskId string) *TaskStdoutCall {
	c := &TaskStdoutCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.taskId = taskId
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TaskStdoutCall) Fields(s ...googleapi.Field) *TaskStdoutCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *TaskStdoutCall) IfNoneMatch(entityTag string) *TaskStdoutCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TaskStdoutCall) Context(ctx context.Context) *TaskStdoutCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TaskStdoutCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TaskStdoutCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "task/{task_id}/stdout")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	googleapi.Expand(req.URL, map[string]string{
		"task_id": c.taskId,
	})
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.task.stdout" call.
// Exactly one of *SwarmingRpcsTaskOutput or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsTaskOutput.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TaskStdoutCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTaskOutput, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTaskOutput{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns the output of the task corresponding to a task ID.",
	//   "httpMethod": "GET",
	//   "id": "swarming.task.stdout",
	//   "parameterOrder": [
	//     "task_id"
	//   ],
	//   "parameters": {
	//     "task_id": {
	//       "location": "path",
	//       "required": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "task/{task_id}/stdout",
	//   "response": {
	//     "$ref": "SwarmingRpcsTaskOutput"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.tasks.cancel":

type TasksCancelCall struct {
	s                              *Service
	swarmingrpcstaskscancelrequest *SwarmingRpcsTasksCancelRequest
	urlParams_                     gensupport.URLParams
	ctx_                           context.Context
	header_                        http.Header
}

// Cancel: Cancel a subset of pending tasks based on the tags.
// Cancellation happens asynchronously, so when this call returns,
// cancellations will not have completed yet.
func (r *TasksService) Cancel(swarmingrpcstaskscancelrequest *SwarmingRpcsTasksCancelRequest) *TasksCancelCall {
	c := &TasksCancelCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.swarmingrpcstaskscancelrequest = swarmingrpcstaskscancelrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TasksCancelCall) Fields(s ...googleapi.Field) *TasksCancelCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TasksCancelCall) Context(ctx context.Context) *TasksCancelCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TasksCancelCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TasksCancelCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.swarmingrpcstaskscancelrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "tasks/cancel")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.tasks.cancel" call.
// Exactly one of *SwarmingRpcsTasksCancelResponse or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *SwarmingRpcsTasksCancelResponse.ServerResponse.Header or (if
// a response was returned at all) in error.(*googleapi.Error).Header.
// Use googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TasksCancelCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTasksCancelResponse, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTasksCancelResponse{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Cancel a subset of pending tasks based on the tags. Cancellation happens asynchronously, so when this call returns, cancellations will not have completed yet.",
	//   "httpMethod": "POST",
	//   "id": "swarming.tasks.cancel",
	//   "path": "tasks/cancel",
	//   "request": {
	//     "$ref": "SwarmingRpcsTasksCancelRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "SwarmingRpcsTasksCancelResponse"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.tasks.count":

type TasksCountCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Count: Counts number of tasks in a given state.
func (r *TasksService) Count() *TasksCountCall {
	c := &TasksCountCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// End sets the optional parameter "end":
func (c *TasksCountCall) End(end float64) *TasksCountCall {
	c.urlParams_.Set("end", fmt.Sprint(end))
	return c
}

// Start sets the optional parameter "start":
func (c *TasksCountCall) Start(start float64) *TasksCountCall {
	c.urlParams_.Set("start", fmt.Sprint(start))
	return c
}

// State sets the optional parameter "state":
//
// Possible values:
//   "ALL" (default)
//   "BOT_DIED"
//   "CANCELED"
//   "COMPLETED"
//   "COMPLETED_FAILURE"
//   "COMPLETED_SUCCESS"
//   "DEDUPED"
//   "EXPIRED"
//   "PENDING"
//   "PENDING_RUNNING"
//   "RUNNING"
//   "TIMED_OUT"
func (c *TasksCountCall) State(state string) *TasksCountCall {
	c.urlParams_.Set("state", state)
	return c
}

// Tags sets the optional parameter "tags":
func (c *TasksCountCall) Tags(tags ...string) *TasksCountCall {
	c.urlParams_.SetMulti("tags", append([]string{}, tags...))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TasksCountCall) Fields(s ...googleapi.Field) *TasksCountCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *TasksCountCall) IfNoneMatch(entityTag string) *TasksCountCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TasksCountCall) Context(ctx context.Context) *TasksCountCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TasksCountCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TasksCountCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "tasks/count")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.tasks.count" call.
// Exactly one of *SwarmingRpcsTasksCount or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsTasksCount.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TasksCountCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTasksCount, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTasksCount{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Counts number of tasks in a given state.",
	//   "httpMethod": "GET",
	//   "id": "swarming.tasks.count",
	//   "parameters": {
	//     "end": {
	//       "format": "double",
	//       "location": "query",
	//       "type": "number"
	//     },
	//     "start": {
	//       "format": "double",
	//       "location": "query",
	//       "type": "number"
	//     },
	//     "state": {
	//       "default": "ALL",
	//       "enum": [
	//         "ALL",
	//         "BOT_DIED",
	//         "CANCELED",
	//         "COMPLETED",
	//         "COMPLETED_FAILURE",
	//         "COMPLETED_SUCCESS",
	//         "DEDUPED",
	//         "EXPIRED",
	//         "PENDING",
	//         "PENDING_RUNNING",
	//         "RUNNING",
	//         "TIMED_OUT"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "tags": {
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "tasks/count",
	//   "response": {
	//     "$ref": "SwarmingRpcsTasksCount"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.tasks.list":

type TasksListCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// List: Returns tasks results based on the filters. This endpoint is
// significantly slower than 'count'. Use 'count' when possible.
func (r *TasksService) List() *TasksListCall {
	c := &TasksListCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Cursor sets the optional parameter "cursor":
func (c *TasksListCall) Cursor(cursor string) *TasksListCall {
	c.urlParams_.Set("cursor", cursor)
	return c
}

// End sets the optional parameter "end":
func (c *TasksListCall) End(end float64) *TasksListCall {
	c.urlParams_.Set("end", fmt.Sprint(end))
	return c
}

// IncludePerformanceStats sets the optional parameter
// "include_performance_stats":
func (c *TasksListCall) IncludePerformanceStats(includePerformanceStats bool) *TasksListCall {
	c.urlParams_.Set("include_performance_stats", fmt.Sprint(includePerformanceStats))
	return c
}

// Limit sets the optional parameter "limit":
func (c *TasksListCall) Limit(limit int64) *TasksListCall {
	c.urlParams_.Set("limit", fmt.Sprint(limit))
	return c
}

// Sort sets the optional parameter "sort":
//
// Possible values:
//   "ABANDONED_TS"
//   "COMPLETED_TS"
//   "CREATED_TS" (default)
//   "MODIFIED_TS"
func (c *TasksListCall) Sort(sort string) *TasksListCall {
	c.urlParams_.Set("sort", sort)
	return c
}

// Start sets the optional parameter "start":
func (c *TasksListCall) Start(start float64) *TasksListCall {
	c.urlParams_.Set("start", fmt.Sprint(start))
	return c
}

// State sets the optional parameter "state":
//
// Possible values:
//   "ALL" (default)
//   "BOT_DIED"
//   "CANCELED"
//   "COMPLETED"
//   "COMPLETED_FAILURE"
//   "COMPLETED_SUCCESS"
//   "DEDUPED"
//   "EXPIRED"
//   "PENDING"
//   "PENDING_RUNNING"
//   "RUNNING"
//   "TIMED_OUT"
func (c *TasksListCall) State(state string) *TasksListCall {
	c.urlParams_.Set("state", state)
	return c
}

// Tags sets the optional parameter "tags":
func (c *TasksListCall) Tags(tags ...string) *TasksListCall {
	c.urlParams_.SetMulti("tags", append([]string{}, tags...))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TasksListCall) Fields(s ...googleapi.Field) *TasksListCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *TasksListCall) IfNoneMatch(entityTag string) *TasksListCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TasksListCall) Context(ctx context.Context) *TasksListCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TasksListCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TasksListCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "tasks/list")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.tasks.list" call.
// Exactly one of *SwarmingRpcsTaskList or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsTaskList.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TasksListCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTaskList, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTaskList{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns tasks results based on the filters. This endpoint is significantly slower than 'count'. Use 'count' when possible.",
	//   "httpMethod": "GET",
	//   "id": "swarming.tasks.list",
	//   "parameters": {
	//     "cursor": {
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "end": {
	//       "format": "double",
	//       "location": "query",
	//       "type": "number"
	//     },
	//     "include_performance_stats": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "limit": {
	//       "default": "200",
	//       "format": "int64",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "sort": {
	//       "default": "CREATED_TS",
	//       "enum": [
	//         "ABANDONED_TS",
	//         "COMPLETED_TS",
	//         "CREATED_TS",
	//         "MODIFIED_TS"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "start": {
	//       "format": "double",
	//       "location": "query",
	//       "type": "number"
	//     },
	//     "state": {
	//       "default": "ALL",
	//       "enum": [
	//         "ALL",
	//         "BOT_DIED",
	//         "CANCELED",
	//         "COMPLETED",
	//         "COMPLETED_FAILURE",
	//         "COMPLETED_SUCCESS",
	//         "DEDUPED",
	//         "EXPIRED",
	//         "PENDING",
	//         "PENDING_RUNNING",
	//         "RUNNING",
	//         "TIMED_OUT"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "tags": {
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "tasks/list",
	//   "response": {
	//     "$ref": "SwarmingRpcsTaskList"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.tasks.new":

type TasksNewCall struct {
	s                          *Service
	swarmingrpcsnewtaskrequest *SwarmingRpcsNewTaskRequest
	urlParams_                 gensupport.URLParams
	ctx_                       context.Context
	header_                    http.Header
}

// New: Creates a new task. The task will be enqueued in the tasks list
// and will be executed at the earliest opportunity by a bot that has at
// least the dimensions as described in the task request.
func (r *TasksService) New(swarmingrpcsnewtaskrequest *SwarmingRpcsNewTaskRequest) *TasksNewCall {
	c := &TasksNewCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	c.swarmingrpcsnewtaskrequest = swarmingrpcsnewtaskrequest
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TasksNewCall) Fields(s ...googleapi.Field) *TasksNewCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TasksNewCall) Context(ctx context.Context) *TasksNewCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TasksNewCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TasksNewCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	var body io.Reader = nil
	body, err := googleapi.WithoutDataWrapper.JSONReader(c.swarmingrpcsnewtaskrequest)
	if err != nil {
		return nil, err
	}
	reqHeaders.Set("Content-Type", "application/json")
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "tasks/new")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("POST", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.tasks.new" call.
// Exactly one of *SwarmingRpcsTaskRequestMetadata or error will be
// non-nil. Any non-2xx status code is an error. Response headers are in
// either *SwarmingRpcsTaskRequestMetadata.ServerResponse.Header or (if
// a response was returned at all) in error.(*googleapi.Error).Header.
// Use googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TasksNewCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTaskRequestMetadata, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTaskRequestMetadata{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Creates a new task. The task will be enqueued in the tasks list and will be executed at the earliest opportunity by a bot that has at least the dimensions as described in the task request.",
	//   "httpMethod": "POST",
	//   "id": "swarming.tasks.new",
	//   "path": "tasks/new",
	//   "request": {
	//     "$ref": "SwarmingRpcsNewTaskRequest",
	//     "parameterName": "resource"
	//   },
	//   "response": {
	//     "$ref": "SwarmingRpcsTaskRequestMetadata"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.tasks.requests":

type TasksRequestsCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Requests: Returns tasks requests based on the filters. This endpoint
// is slightly slower than 'list'. Use 'list' or 'count' when possible.
func (r *TasksService) Requests() *TasksRequestsCall {
	c := &TasksRequestsCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Cursor sets the optional parameter "cursor":
func (c *TasksRequestsCall) Cursor(cursor string) *TasksRequestsCall {
	c.urlParams_.Set("cursor", cursor)
	return c
}

// End sets the optional parameter "end":
func (c *TasksRequestsCall) End(end float64) *TasksRequestsCall {
	c.urlParams_.Set("end", fmt.Sprint(end))
	return c
}

// IncludePerformanceStats sets the optional parameter
// "include_performance_stats":
func (c *TasksRequestsCall) IncludePerformanceStats(includePerformanceStats bool) *TasksRequestsCall {
	c.urlParams_.Set("include_performance_stats", fmt.Sprint(includePerformanceStats))
	return c
}

// Limit sets the optional parameter "limit":
func (c *TasksRequestsCall) Limit(limit int64) *TasksRequestsCall {
	c.urlParams_.Set("limit", fmt.Sprint(limit))
	return c
}

// Sort sets the optional parameter "sort":
//
// Possible values:
//   "ABANDONED_TS"
//   "COMPLETED_TS"
//   "CREATED_TS" (default)
//   "MODIFIED_TS"
func (c *TasksRequestsCall) Sort(sort string) *TasksRequestsCall {
	c.urlParams_.Set("sort", sort)
	return c
}

// Start sets the optional parameter "start":
func (c *TasksRequestsCall) Start(start float64) *TasksRequestsCall {
	c.urlParams_.Set("start", fmt.Sprint(start))
	return c
}

// State sets the optional parameter "state":
//
// Possible values:
//   "ALL" (default)
//   "BOT_DIED"
//   "CANCELED"
//   "COMPLETED"
//   "COMPLETED_FAILURE"
//   "COMPLETED_SUCCESS"
//   "DEDUPED"
//   "EXPIRED"
//   "PENDING"
//   "PENDING_RUNNING"
//   "RUNNING"
//   "TIMED_OUT"
func (c *TasksRequestsCall) State(state string) *TasksRequestsCall {
	c.urlParams_.Set("state", state)
	return c
}

// Tags sets the optional parameter "tags":
func (c *TasksRequestsCall) Tags(tags ...string) *TasksRequestsCall {
	c.urlParams_.SetMulti("tags", append([]string{}, tags...))
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TasksRequestsCall) Fields(s ...googleapi.Field) *TasksRequestsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *TasksRequestsCall) IfNoneMatch(entityTag string) *TasksRequestsCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TasksRequestsCall) Context(ctx context.Context) *TasksRequestsCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TasksRequestsCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TasksRequestsCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "tasks/requests")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.tasks.requests" call.
// Exactly one of *SwarmingRpcsTaskRequests or error will be non-nil.
// Any non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsTaskRequests.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TasksRequestsCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTaskRequests, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTaskRequests{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns tasks requests based on the filters. This endpoint is slightly slower than 'list'. Use 'list' or 'count' when possible.",
	//   "httpMethod": "GET",
	//   "id": "swarming.tasks.requests",
	//   "parameters": {
	//     "cursor": {
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "end": {
	//       "format": "double",
	//       "location": "query",
	//       "type": "number"
	//     },
	//     "include_performance_stats": {
	//       "location": "query",
	//       "type": "boolean"
	//     },
	//     "limit": {
	//       "default": "200",
	//       "format": "int64",
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "sort": {
	//       "default": "CREATED_TS",
	//       "enum": [
	//         "ABANDONED_TS",
	//         "COMPLETED_TS",
	//         "CREATED_TS",
	//         "MODIFIED_TS"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "start": {
	//       "format": "double",
	//       "location": "query",
	//       "type": "number"
	//     },
	//     "state": {
	//       "default": "ALL",
	//       "enum": [
	//         "ALL",
	//         "BOT_DIED",
	//         "CANCELED",
	//         "COMPLETED",
	//         "COMPLETED_FAILURE",
	//         "COMPLETED_SUCCESS",
	//         "DEDUPED",
	//         "EXPIRED",
	//         "PENDING",
	//         "PENDING_RUNNING",
	//         "RUNNING",
	//         "TIMED_OUT"
	//       ],
	//       "enumDescriptions": [
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         "",
	//         ""
	//       ],
	//       "location": "query",
	//       "type": "string"
	//     },
	//     "tags": {
	//       "location": "query",
	//       "repeated": true,
	//       "type": "string"
	//     }
	//   },
	//   "path": "tasks/requests",
	//   "response": {
	//     "$ref": "SwarmingRpcsTaskRequests"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}

// method id "swarming.tasks.tags":

type TasksTagsCall struct {
	s            *Service
	urlParams_   gensupport.URLParams
	ifNoneMatch_ string
	ctx_         context.Context
	header_      http.Header
}

// Tags: Returns the cached set of tags currently seen in the fleet.
func (r *TasksService) Tags() *TasksTagsCall {
	c := &TasksTagsCall{s: r.s, urlParams_: make(gensupport.URLParams)}
	return c
}

// Fields allows partial responses to be retrieved. See
// https://developers.google.com/gdata/docs/2.0/basics#PartialResponse
// for more information.
func (c *TasksTagsCall) Fields(s ...googleapi.Field) *TasksTagsCall {
	c.urlParams_.Set("fields", googleapi.CombineFields(s))
	return c
}

// IfNoneMatch sets the optional parameter which makes the operation
// fail if the object's ETag matches the given value. This is useful for
// getting updates only after the object has changed since the last
// request. Use googleapi.IsNotModified to check whether the response
// error from Do is the result of In-None-Match.
func (c *TasksTagsCall) IfNoneMatch(entityTag string) *TasksTagsCall {
	c.ifNoneMatch_ = entityTag
	return c
}

// Context sets the context to be used in this call's Do method. Any
// pending HTTP request will be aborted if the provided context is
// canceled.
func (c *TasksTagsCall) Context(ctx context.Context) *TasksTagsCall {
	c.ctx_ = ctx
	return c
}

// Header returns an http.Header that can be modified by the caller to
// add HTTP headers to the request.
func (c *TasksTagsCall) Header() http.Header {
	if c.header_ == nil {
		c.header_ = make(http.Header)
	}
	return c.header_
}

func (c *TasksTagsCall) doRequest(alt string) (*http.Response, error) {
	reqHeaders := make(http.Header)
	for k, v := range c.header_ {
		reqHeaders[k] = v
	}
	reqHeaders.Set("User-Agent", c.s.userAgent())
	if c.ifNoneMatch_ != "" {
		reqHeaders.Set("If-None-Match", c.ifNoneMatch_)
	}
	var body io.Reader = nil
	c.urlParams_.Set("alt", alt)
	urls := googleapi.ResolveRelative(c.s.BasePath, "tasks/tags")
	urls += "?" + c.urlParams_.Encode()
	req, _ := http.NewRequest("GET", urls, body)
	req.Header = reqHeaders
	return gensupport.SendRequest(c.ctx_, c.s.client, req)
}

// Do executes the "swarming.tasks.tags" call.
// Exactly one of *SwarmingRpcsTasksTags or error will be non-nil. Any
// non-2xx status code is an error. Response headers are in either
// *SwarmingRpcsTasksTags.ServerResponse.Header or (if a response was
// returned at all) in error.(*googleapi.Error).Header. Use
// googleapi.IsNotModified to check whether the returned error was
// because http.StatusNotModified was returned.
func (c *TasksTagsCall) Do(opts ...googleapi.CallOption) (*SwarmingRpcsTasksTags, error) {
	gensupport.SetOptions(c.urlParams_, opts...)
	res, err := c.doRequest("json")
	if res != nil && res.StatusCode == http.StatusNotModified {
		if res.Body != nil {
			res.Body.Close()
		}
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
		}
	}
	if err != nil {
		return nil, err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return nil, err
	}
	ret := &SwarmingRpcsTasksTags{
		ServerResponse: googleapi.ServerResponse{
			Header:         res.Header,
			HTTPStatusCode: res.StatusCode,
		},
	}
	target := &ret
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return nil, err
	}
	return ret, nil
	// {
	//   "description": "Returns the cached set of tags currently seen in the fleet.",
	//   "httpMethod": "GET",
	//   "id": "swarming.tasks.tags",
	//   "path": "tasks/tags",
	//   "response": {
	//     "$ref": "SwarmingRpcsTasksTags"
	//   },
	//   "scopes": [
	//     "https://www.googleapis.com/auth/userinfo.email"
	//   ]
	// }

}
