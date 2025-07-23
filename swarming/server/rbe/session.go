// Copyright 2023 The LUCI Authors.
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

// Package rbe implements communication with RBE APIs.
package rbe

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/swarming/internal/remoteworkers"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botsession"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/hmactoken"
)

// SessionServer serves handlers for creating and updating RBE bot sessions.
type SessionServer struct {
	rbe        remoteworkers.BotsClient
	hmacSecret *hmactoken.Secret // to generate session tokens
	backendVer string            // to put into the DebugInfo in the tokens
	cfg        *cfg.Provider
}

// NewSessionServer creates a new session server given an RBE client connection.
func NewSessionServer(ctx context.Context, cc []grpc.ClientConnInterface, hmacSecret *hmactoken.Secret, backendVer string, cfg *cfg.Provider) *SessionServer {
	return &SessionServer{
		rbe:        botsConnectionPool(cc),
		hmacSecret: hmacSecret,
		backendVer: backendVer,
		cfg:        cfg,
	}
}

////////////////////////////////////////////////////////////////////////////////
// Structs used by all handlers.

// WorkerProperties are RBE worker properties unrelated to actual scheduling.
//
// They aren't validated by Swarming and just passed along to RBE. The RBE bots
// obtain them via some external mechanism (e.g. the GCE metadata server).
//
// They are optional and currently used only on bots managed by RBE Worker
// Provider.
type WorkerProperties struct {
	// PoolID will be used as `rbePoolID` bot session property.
	PoolID string `json:"pool_id"`
	// PoolVersion will be used as `rbePoolVersion` bot session property.
	PoolVersion string `json:"pool_version"`
}

////////////////////////////////////////////////////////////////////////////////
// CreateBotSession handler.

// CreateBotSessionRequest is a body of `/bot/rbe/session/create` request.
type CreateBotSessionRequest struct {
	// Session is a serialized Swarming Bot Session proto.
	Session []byte `json:"session"`

	// BotVersion identifies the bot software. It is reported to RBE as is.
	BotVersion string `json:"bot_version,omitempty"`

	// WorkerProperties are passed to RBE as worker properties.
	WorkerProperties *WorkerProperties `json:"worker_properties,omitempty"`
}

func (r *CreateBotSessionRequest) ExtractSession() []byte { return r.Session }
func (r *CreateBotSessionRequest) ExtractDebugRequest() any {
	return &CreateBotSessionRequest{
		BotVersion:       r.BotVersion,
		WorkerProperties: r.WorkerProperties,
	}
}

// CreateBotSessionResponse is a body of `/bot/rbe/session/create` response.
type CreateBotSessionResponse struct {
	// Session is a serialized Swarming Bot Session proto.
	//
	// It is derived from the session in the request, except it has RBE session
	// info populated now.
	Session []byte `json:"session"`

	// SessionID is an RBE bot session ID as encoded in the token.
	//
	// Primarily for the bot debug log.
	SessionID string `json:"session_id"`
}

// CreateBotSession is an RPC handler that creates a new bot session.
func (srv *SessionServer) CreateBotSession(ctx context.Context, body *CreateBotSessionRequest, r *botsrv.Request) (botsrv.Response, error) {
	// A non-RBE bot should not be attempting to open an RBE session.
	rbeInstance := r.Session.BotConfig.RbeInstance
	if rbeInstance == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "the bot is not in RBE mode")
	}

	cfg, err := srv.cfg.FreshEnough(ctx, r.Session.LastSeenConfig.AsTime())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch service config")
	}
	// Actually open the RBE session. This should not block, since we aren't
	// picking up any tasks yet (indicated by INITIALIZING status).
	rbeSession, err := srv.rbe.CreateBotSession(ctx, &remoteworkers.CreateBotSessionRequest{
		Parent:     rbeInstance,
		BotSession: rbeBotSession("", remoteworkers.BotStatus_INITIALIZING, r.Dimensions, cfg, body.BotVersion, body.WorkerProperties, nil, r.Session),
	})
	if err != nil {
		// Return the exact same gRPC error in a reply. This is fine, we trust the
		// bot, it has already been authorized. It is useful for debugging to see
		// the original RBE errors in the bot logs.
		return nil, err
	}
	logging.Infof(ctx, "%s: %s", r.Session.BotId, rbeSession.Name)
	for _, lease := range rbeSession.Leases {
		logging.Errorf(ctx, "Unexpected lease when just opening the session: %s", lease)
	}

	// Associate the RBE session with the Swarming session.
	sessionTok, err := srv.updateSessionToken(ctx, r.Session, rbeSession.Name)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not marshal session token: %s", err)
	}

	return &CreateBotSessionResponse{
		Session:   sessionTok,
		SessionID: rbeSession.Name,
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// UpdateBotSession handler.

// Lease is a JSON representation of a relevant subset of remoteworkers.Lease.
type Lease struct {
	// ID is the unique reservation ID treated as an opaque string. Required.
	ID string `json:"id"`

	// State is a lease state as stringy remoteworkers.LeaseState enum. Required.
	//
	// Possible values:
	//   * PENDING
	//   * ACTIVE
	//   * COMPLETED
	//   * CANCELLED
	State string `json:"state"`

	// Payload is the reservation payload.
	//
	// Note it is serialized using regular JSON rules, i.e. fields are in
	// "snake_case".
	Payload *internalspb.TaskPayload `json:"payload,omitempty"`

	// Result is the execution result.
	//
	// Note it is serialized using regular JSON rules, i.e. fields are in
	// "snake_case".
	Result *internalspb.TaskResult `json:"result,omitempty"`
}

// UpdateBotSessionRequest is a body of `/bot/rbe/session/update` request.
//
// If PollToken is present, it will be used to refresh the state stored in the
// session token.
type UpdateBotSessionRequest struct {
	// Session is a serialized Swarming Bot Session proto.
	//
	// It should have an RBE session info inside, as populated by
	// `/bot/rbe/session/create`.
	Session []byte `json:"session"`

	// BotVersion identifies the bot software. It is reported to RBE as is.
	BotVersion string `json:"bot_version,omitempty"`

	// WorkerProperties are passed to RBE as worker properties.
	WorkerProperties *WorkerProperties `json:"worker_properties,omitempty"`

	// The intended bot session status as stringy remoteworkers.BotStatus enum.
	//
	// Possible values:
	//   * OK
	//   * UNHEALTHY
	//   * HOST_REBOOTING
	//   * BOT_TERMINATING
	//   * INITIALIZING
	//   * MAINTENANCE
	Status string `json:"status"`

	// Nonblocking is true if the bot doesn't want to block waiting for new
	// leases to appear.
	Nonblocking bool `json:"nonblocking"`

	// The lease the bot is currently working or have just finished working on.
	//
	// Allowed lease states here are:
	//   * ACTIVE: the bot is still working on the lease.
	//   * COMPLETED: the bot has finished working on the lease. Result field
	//     should be populated. This state is also used to report the bot is done
	//     working on a canceled lease.
	//
	// Payload field is always ignored.
	Lease *Lease `json:"lease,omitempty"`
}

func (r *UpdateBotSessionRequest) ExtractSession() []byte { return r.Session }
func (r *UpdateBotSessionRequest) ExtractDebugRequest() any {
	return &UpdateBotSessionRequest{
		BotVersion:       r.BotVersion,
		WorkerProperties: r.WorkerProperties,
		Status:           r.Status,
		Nonblocking:      r.Nonblocking,
		Lease:            r.Lease,
	}
}

// UpdateBotSessionResponse is a body of `/bot/rbe/session/update` response.
type UpdateBotSessionResponse struct {
	// Session is a serialized Swarming Bot Session proto.
	//
	// It is derived from the session in the request, except it has its expiration
	// time bumped.
	Session []byte `json:"session"`

	// The session status as seen by the server, as remoteworkers.BotStatus enum.
	//
	// Possible values:
	//   * OK: if the session is healthy.
	//   * BOT_TERMINATING: if the session has expired.
	//   * UNHEALTHY: if the session is unhealthy, for example if the bot ID
	//     changes.
	Status string `json:"status"`

	// The lease the bot should be working on or should cancel now, if any.
	//
	// Possible lease states here:
	//   * PENDING: the bot should start working on this new lease. It has Payload
	//     field populated. Can only happen in reply to a bot reporting no lease
	//     or a completed lease.
	//   * ACTIVE: the bot should keep working on the lease it just reported. Can
	//     only happen in reply to a bot reporting an active lease. Payload is not
	//     populate (the bot should know it already).
	//   * CANCELLED: the bot should stop working on the lease it just reported.
	//     Once the bot is done working on the lease, it should update the session
	//     again, marking the lease as COMPLETED. Payload is not populated.
	//
	// If the bot was stuck for a while and the RBE canceled the lease as lost,
	// this field will be unpopulated, even if the bot reported an active lease.
	// The bot should give up on the current lease ASAP, without even reporting
	// its result back (because the server gave up on it already anyway).
	Lease *Lease `json:"lease,omitempty"`
}

// UpdateBotSession is an RPC handler that updates a bot session.
func (srv *SessionServer) UpdateBotSession(ctx context.Context, body *UpdateBotSessionRequest, r *botsrv.Request) (botsrv.Response, error) {
	// A bot without an RBE session should not be attempting to update it.
	rbeSessionID := r.Session.RbeBotSessionId
	if rbeSessionID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "no active RBE session")
	}

	logging.Infof(ctx, "%s: %s", r.Session.BotId, rbeSessionID)
	logSession(ctx, "Input", body.Status, body.Lease)

	// Need a recognizable status enum.
	botStatus := remoteworkers.BotStatus(remoteworkers.BotStatus_value[body.Status])
	if botStatus == remoteworkers.BotStatus_BOT_STATUS_UNSPECIFIED {
		if body.Status == "" {
			return nil, status.Errorf(codes.InvalidArgument, "missing session status")
		}
		return nil, status.Errorf(codes.InvalidArgument, "unrecognized session status %q", body.Status)
	}

	// Convert our Lease to the RBE remoteworkers.Lease. We expect only ACTIVE or
	// COMPLETED leases, see UpdateBotSessionRequest comment.
	var leaseIn *remoteworkers.Lease
	if body.Lease != nil {
		leaseIn = &remoteworkers.Lease{
			Id:    body.Lease.ID,
			State: remoteworkers.LeaseState(remoteworkers.LeaseState_value[body.Lease.State]),
		}
		switch leaseIn.State {
		case remoteworkers.LeaseState_ACTIVE:
			// This is a "keep alive" update.
		case remoteworkers.LeaseState_COMPLETED:
			// This is a result-reporting update. Populate the result, if any.
			leaseIn.Status = &statuspb.Status{} // means "OK"
			if body.Lease.Result != nil {
				var err error
				if leaseIn.Result, err = anypb.New(body.Lease.Result); err != nil {
					return nil, status.Errorf(codes.Internal, "failed to serialize TaskResult: %s", err)
				}
			}
		case remoteworkers.LeaseState_LEASE_STATE_UNSPECIFIED:
			if body.Lease.State == "" {
				return nil, status.Errorf(codes.InvalidArgument, "missing lease state")
			}
			return nil, status.Errorf(codes.InvalidArgument, "unrecognized lease state %q", body.Lease.State)
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unexpected lease state %q", body.Lease.State)
		}
	}

	// If there are no pending leases, RBE seems to block for `<rpc deadline>-10s`
	// (not doing anything at all if the RPC deadline is less than 10s).
	var timeout time.Duration
	if body.Nonblocking {
		// RPCs with timeout of less that 10s are treated by RBE as non-blocking.
		// Note the timeout is propagated via gRPC metadata headers, it is like an
		// implicit RPC parameters. This should be pretty deterministic.
		timeout = 9 * time.Second
	} else {
		// Since we are running on GAE, we are limited by 1m total. Tell RBE we
		// have ~50s, it will block for ~40s, giving us ~20s of spare time.
		//
		// Randomize this timeout a bit to avoid freshly restarted bots call us
		// in synchronized "waves".
		//
		// TODO(vadimsh): This needs more tuning, in particular in combination with
		// GAE's `max_concurrent_requests` parameter.
		timeout = randomDuration(45*time.Second, 55*time.Second)
	}

	rpcCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cfg, err := srv.cfg.FreshEnough(ctx, r.Session.LastSeenConfig.AsTime())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch service config")
	}

	session, err := srv.rbe.UpdateBotSession(rpcCtx, &remoteworkers.UpdateBotSessionRequest{
		Name:       rbeSessionID,
		BotSession: rbeBotSession(rbeSessionID, botStatus, r.Dimensions, cfg, body.BotVersion, body.WorkerProperties, leaseIn, r.Session),
	})

	if err != nil {
		// If the bot was just polling for new work, treat DEADLINE_EXCEEDED as
		// "no work available". Otherwise we may end up replying with a lot of
		// errors and GAE treats this as a signal that the instance is unhealthy
		// and kills it.
		if status.Code(err) == codes.DeadlineExceeded && leaseIn == nil && botStatus == remoteworkers.BotStatus_OK {
			logging.Warningf(ctx, "Deadline exceeded when polling for new leases")
			sessionTok, err := srv.updateSessionToken(ctx, r.Session, rbeSessionID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "could not marshal session token: %s", err)
			}
			return &UpdateBotSessionResponse{
				Session: sessionTok,
				Status:  "OK",
			}, nil
		}

		// If the bot ID changed, for example if the bot's effective bot ID changed,
		// treat the session as unhealthy.
		//
		// The error message should be like "cannot change bot ID: expected <old_bot_id>, got <botSession.BotId>".
		if status.Code(err) == codes.FailedPrecondition && strings.Contains(err.Error(), "cannot change bot ID") {
			logging.Warningf(ctx, "bot ID changed: %s", err)
			sessionTok, err := srv.updateSessionToken(ctx, r.Session, rbeSessionID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "could not marshal session token: %s", err)
			}
			return &UpdateBotSessionResponse{
				Session: sessionTok,
				Status:  "UNHEALTHY",
			}, nil
		}

		// Return the exact same gRPC error in a reply. This is fine, we trust the
		// bot, it has already been authorized. It is useful for debugging to see
		// the original RBE errors.
		return nil, err
	}

	// The RBE backend always replies with either OK or BOT_TERMINATING status.
	// Note that it replies with OK status even if we told it we want the session
	// terminated. The only time it replies with BOT_TERMINATING is when the
	// session was *already* dead (either closed by the bot previously or timed
	// out by the RBE server).
	acceptingLeases := botStatus == remoteworkers.BotStatus_OK
	switch session.Status {
	case remoteworkers.BotStatus_OK:
		// Do nothing. This is fine. Trust `botStatus` was applied.
	case remoteworkers.BotStatus_BOT_TERMINATING:
		// The session was already closed previously.
		acceptingLeases = false
	default: // i.e. all other "unhealthy" or "not ready" statuses
		logging.Errorf(ctx, "Unexpected status change from RBE: %s => %s", botStatus, session.Status)
		acceptingLeases = false
	}

	if !acceptingLeases {
		// RBE should not assign leases to a terminating or unhealthy bot.
		for _, lease := range session.Leases {
			logging.Errorf(ctx, "Unexpected RBE lease: %s", lease)
		}
		session.Leases = nil
	}

	// The lease we'll report to the bot.
	var leaseOut *remoteworkers.Lease
	var leasePayload *internalspb.TaskPayload

	// If a bot reported an ACTIVE lease the RBE server should either ack it as
	// ACTIVE as well or report it as CANCELED. Additionally if the bot was stuck
	// and didn't ping the lease in a while, the RBE server marks the lease as
	// lost and silently ignores it, i.e. doesn't return it in session.Leases.
	if leaseIn != nil && leaseIn.State == remoteworkers.LeaseState_ACTIVE {
		// Find the reported lease in the response. There should be no other leases.
		for _, lease := range session.Leases {
			if lease.Id == leaseIn.Id {
				leaseOut = lease
				if leaseOut.State != remoteworkers.LeaseState_ACTIVE && leaseOut.State != remoteworkers.LeaseState_CANCELLED {
					return nil, status.Errorf(codes.Internal, "unexpected ACTIVE lease state transition to %s", leaseOut.State)
				}
				if leaseOut.Payload != nil {
					logging.Errorf(ctx, "Unexpected payload in the lease, dropping it")
					leaseOut.Payload = nil
				}
			} else {
				logging.Errorf(ctx, "Unexpected RBE lease: %s", lease)
			}
		}
		if leaseOut == nil {
			logging.Warningf(ctx, "The bot lost the lease")
		}
	}

	// If a bot reported no lease at all or a COMPLETED lease, the server should
	// return at most one new lease in PENDING state with its payload populated.
	if leaseIn == nil || leaseIn.State == remoteworkers.LeaseState_COMPLETED {
		// Fish out a PENDING lease, if any, ignoring everything else (there should
		// not be anything else there).
		for _, lease := range session.Leases {
			if leaseOut != nil {
				logging.Errorf(ctx, "Unexpected RBE lease: %s", lease)
				continue
			}
			if lease.State == remoteworkers.LeaseState_PENDING {
				leaseOut = lease
			} else {
				logging.Errorf(ctx, "Unexpected non-pending RBE lease: %s", lease)
			}
		}
		if leaseOut != nil {
			// Check this PENDING lease has the payload in a format we understand.
			leasePayload = &internalspb.TaskPayload{}
			if err := leaseOut.Payload.UnmarshalTo(leasePayload); err != nil {
				// TODO(vadimsh): This is a fatally broken task with missing or
				// unrecognized payload, need to tell the RBE to drop it otherwise it
				// will haunt this bot until its expiration.
				logging.Errorf(ctx, "Failed to unmarshal lease payload:\n%s", prettyProto(leaseOut))
				return nil, status.Errorf(codes.Internal, "failed to unmarshal pending lease payload: %s", err)
			}
		}
	}

	// Convert the output lease to the API response form.
	var respLease *Lease
	if leaseOut != nil {
		respLease = &Lease{
			ID:      leaseOut.Id,
			State:   remoteworkers.LeaseState_name[int32(leaseOut.State)],
			Payload: leasePayload,
		}
	}

	// Bump the session expiration.
	sessionTok, err := srv.updateSessionToken(ctx, r.Session, rbeSessionID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not marshal session token: %s", err)
	}

	resp := &UpdateBotSessionResponse{
		Session: sessionTok,
		Status:  remoteworkers.BotStatus_name[int32(session.Status)],
		Lease:   respLease,
	}
	logSession(ctx, "Output", resp.Status, resp.Lease)
	return resp, nil
}

////////////////////////////////////////////////////////////////////////////////
// Helpers.

// updateSessionToken generates a new session token with bumped expiry.
func (srv *SessionServer) updateSessionToken(ctx context.Context, s *internalspb.Session, rbeSessionID string) ([]byte, error) {
	s.RbeBotSessionId = rbeSessionID
	s.DebugInfo = botsession.DebugInfo(ctx, srv.backendVer)
	s.Expiry = timestamppb.New(clock.Now(ctx).Add(botsession.Expiry))
	return botsession.Marshal(s, srv.hmacSecret)
}

// rbeBotSession constructs remoteworkers.BotSession based on validated bot
// dimensions and the current lease.
func rbeBotSession(
	sessionID string,
	status remoteworkers.BotStatus,
	dims botsrv.BotDimensions,
	cfg *cfg.Config,
	botVersion string,
	workerProps *WorkerProperties,
	lease *remoteworkers.Lease,
	swarmingSession *internalspb.Session,
) *remoteworkers.BotSession {
	botID, props := toDeviceProperties(dims, cfg, swarmingSession)

	// These are used to associated the RBE worker with its worker provider pool.
	var workerPropsList []*remoteworkers.Worker_Property
	if workerProps != nil {
		if workerProps.PoolID != "" {
			workerPropsList = append(workerPropsList, &remoteworkers.Worker_Property{
				Key:   "rbePoolID",
				Value: workerProps.PoolID,
			})
		}
		if workerProps.PoolVersion != "" {
			workerPropsList = append(workerPropsList, &remoteworkers.Worker_Property{
				Key:   "rbePoolVersion",
				Value: workerProps.PoolVersion,
			})
		}
	}

	var leases []*remoteworkers.Lease
	if lease != nil {
		leases = []*remoteworkers.Lease{lease}
	}

	return &remoteworkers.BotSession{
		BotId:   botID,
		Name:    sessionID,
		Version: botVersion,
		Status:  status,
		Leases:  leases,
		Worker: &remoteworkers.Worker{
			Properties: workerPropsList,
			Devices: []*remoteworkers.Device{
				{
					Handle:     "primary",
					Properties: props,
				},
			},
		},
	}
}

func toDeviceProperties(dims botsrv.BotDimensions, cfg *cfg.Config, swarmingSession *internalspb.Session) (string, []*remoteworkers.Device_Property) {
	// Note that at this point `dims` are validated already by botsrv.Server and
	// we can panic on unexpected values.
	var pool string
	for _, kv := range dims {
		if key, val, ok := strings.Cut(kv, ":"); ok {
			if key == "pool" {
				pool = val
				break
			}
		}
	}
	if pool == "" {
		panic(`"pool" dimension is required`)
	}

	poolCfg := cfg.Pool(pool)
	var infoDimRes []*regexp.Regexp
	if poolCfg != nil {
		infoDimRes = poolCfg.InformationalDimensionRe
	}

	botID := ""
	props := make([]*remoteworkers.Device_Property, 0, len(dims))
	for _, kv := range dims {
		if key, val, ok := strings.Cut(kv, ":"); ok {
			if key == "id" {
				if botID != "" {
					panic(fmt.Sprintf("duplicate `id` dimension in %q", dims))
				}
				botID = val
				continue
			}

			if key == swarmingSession.BotConfig.RbeEffectiveBotIdDimension {
				continue
			}

			var informational bool
			for _, re := range infoDimRes {
				if re.MatchString(key) {
					informational = true
					break
				}
			}
			if informational {
				continue
			}
			props = append(props, &remoteworkers.Device_Property{
				Key:   "label:" + key,
				Value: val,
			})
		}
	}
	if botID == "" {
		panic("bot ID is missing in dimensions")
	}

	if swarmingSession.BotConfig.RbeEffectiveBotId != "" {
		botID = swarmingSession.BotConfig.RbeEffectiveBotId
	}

	return botID, props
}

// randomDuration returns a uniformly distributed random number in range [a, b).
func randomDuration(a, b time.Duration) time.Duration {
	return a + time.Duration(rand.Int63n(int64(b-a)))
}

// logSession logs some basic information about the session.
func logSession(ctx context.Context, direction, status string, lease *Lease) {
	if lease != nil {
		logging.Infof(ctx, "%s: %s, lease %s %s", direction, status, lease.State, lease.ID)
	} else {
		logging.Infof(ctx, "%s: %s, no lease", direction, status)
	}
}
