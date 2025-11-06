// Copyright 2015 The LUCI Authors.
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

package service

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
)

// Notification represents a notification about AuthDB change. Must be acked
// once processed.
type Notification struct {
	Revision int64 // new auth DB revision

	service      *AuthService // parent service
	subscription string       // subscription name the change was pulled from
	ackIDs       []string     // IDs of all messages to ack
}

// Acknowledge tells PubSub to stop redelivering this notification.
func (n *Notification) Acknowledge(ctx context.Context) error {
	if n.service != nil { // may be nil if Notification is generated in tests
		return n.service.ackMessages(ctx, n.subscription, n.ackIDs)
	}
	return nil
}

// pubSubMessage is received via PubSub when AuthDB changes.
type pubSubMessage struct {
	AckID   string `json:"ackId"` // set only when pulling
	Message struct {
		MessageID  string            `json:"messageId"`
		Data       string            `json:"data"`
		Attributes map[string]string `json:"attributes"`
	} `json:"message"`
}

// Snapshot contains AuthDB proto message (all user groups and other information
// received from auth_service), along with its revision number, timestamp of
// when it was created, and URL of a service it was fetched from.
type Snapshot struct {
	AuthDB         *protocol.AuthDB
	AuthServiceURL string
	Rev            int64
	Created        time.Time
}

// AuthDBAccess describes how an authorized reader can access the AuthDB.
//
// See RequestAccess.
type AuthDBAccess struct {
	NotificationTopic string // pubsub topic name "project/<project>/topics/<topic>"
	StorageDumpPath   string // GCS storage path "<bucket>/<object>", may be empty
}

// AuthService represents API exposed by auth_service.
//
// It is a fairy low-level API, you must have good reasons for using it.
type AuthService struct {
	// URL is root URL (with protocol) of auth_service (e.g. "https://<host>").
	URL string
	// OAuthScopes is scopes to use for authentication (or nil for defaults).
	OAuthScopes []string

	// pubSubURLRoot is root URL of PubSub service. Mocked in tests.
	pubSubURLRoot string
}

// pubSubURL returns full URL to pubsub API endpoint.
func (s *AuthService) pubSubURL(path string) string {
	if s.pubSubURLRoot != "" {
		return s.pubSubURLRoot + path
	}
	return "https://pubsub.googleapis.com/v1/" + path
}

// oauthScopes returns a list of OAuth scopes to use for authentication.
func (s *AuthService) oauthScopes() []string {
	if s.OAuthScopes != nil {
		return s.OAuthScopes
	}
	return scopes.CloudScopeSet()
}

// RequestAccess asks Auth Service to grant the caller (us) access to the AuthDB
// change notifications PubSub topic and AuthDB GCS dump.
//
// This works only if the caller is in "auth-trusted-services" group. As soon
// as the caller is removed from this group, the access is revoked.
func (s *AuthService) RequestAccess(ctx context.Context) (*AuthDBAccess, error) {
	// See appengine/auth_service/handlers_frontend.py.
	var response struct {
		Topic string `json:"topic"`
		GS    struct {
			AuthDBGSPath string `json:"auth_db_gs_path"`
		} `json:"gs"`
	}
	req := internal.Request{
		Method: "POST",
		URL:    s.URL + "/auth_service/api/v1/authdb/subscription/authorization",
		Scopes: s.oauthScopes(),
		Body:   map[string]string{},
		Out:    &response,
	}
	if err := req.Do(ctx); err != nil {
		return nil, err
	}
	return &AuthDBAccess{
		NotificationTopic: response.Topic,
		StorageDumpPath:   response.GS.AuthDBGSPath, // may be ""
	}, nil
}

// EnsureSubscription creates a new subscription to AuthDB change notifications
// topic or changes its pushURL if it already exists. `subscription` is full
// subscription name e.g. "projects/<projectid>/subscriptions/<subid>". Name of
// the topic is fetched from the auth service. Returns nil if such subscription
// already exists.
func (s *AuthService) EnsureSubscription(ctx context.Context, subscription, pushURL string) error {
	// Subscription already exists?
	var existing struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
		PushConfig struct {
			PushEndpoint string `json:"pushEndpoint"`
		} `json:"pushConfig"`
	}
	req := internal.Request{
		Method: "GET",
		URL:    s.pubSubURL(subscription),
		Scopes: s.oauthScopes(),
		Out:    &existing,
	}
	err := req.Do(ctx)
	if err != nil && existing.Error.Code != 404 {
		return err
	}

	// Create a new subscription if existing is missing.
	if err != nil {
		// Make sure caller has access to the PubSub topic.
		access, err := s.RequestAccess(ctx)
		if err != nil {
			return err
		}
		// Create the subscription.
		var config struct {
			Topic      string `json:"topic"`
			PushConfig struct {
				PushEndpoint string `json:"pushEndpoint"`
			} `json:"pushConfig"`
			AckDeadlineSeconds int `json:"ackDeadlineSeconds"`
		}
		config.Topic = access.NotificationTopic
		config.PushConfig.PushEndpoint = pushURL
		config.AckDeadlineSeconds = 60
		req = internal.Request{
			Method: "PUT",
			URL:    s.pubSubURL(subscription),
			Scopes: s.oauthScopes(),
			Body:   &config,
		}
		return req.Do(ctx)
	}

	// Is existing subscription configured correctly already?
	if existing.PushConfig.PushEndpoint == pushURL {
		return nil
	}

	// Reconfigure existing subscription.
	var request struct {
		PushConfig struct {
			PushEndpoint string `json:"pushEndpoint"`
		} `json:"pushConfig"`
	}
	request.PushConfig.PushEndpoint = pushURL

	req = internal.Request{
		Method: "POST",
		URL:    s.pubSubURL(subscription + ":modifyPushConfig"),
		Scopes: s.oauthScopes(),
		Body:   &request,
	}
	return req.Do(ctx)
}

// DeleteSubscription removes PubSub subscription if it exists.
func (s *AuthService) DeleteSubscription(ctx context.Context, subscription string) error {
	var reply struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	req := internal.Request{
		Method: "DELETE",
		URL:    s.pubSubURL(subscription),
		Scopes: s.oauthScopes(),
		Out:    &reply,
	}
	if err := req.Do(ctx); err != nil && reply.Error.Code != 404 {
		return err
	}
	return nil
}

// PullPubSub pulls pending PubSub messages (from subscription created
// previously by EnsureSubscription), authenticates them, and converts
// them into Notification object. Returns (nil, nil) if no pending messages.
// Does not wait for messages to arrive.
func (s *AuthService) PullPubSub(ctx context.Context, subscription string) (*Notification, error) {
	request := struct {
		ReturnImmediately bool `json:"returnImmediately"`
		MaxMessages       int  `json:"maxMessages"`
	}{true, 1000}

	response := struct {
		ReceivedMessages []pubSubMessage `json:"receivedMessages"`
	}{}

	req := internal.Request{
		Method: "POST",
		URL:    s.pubSubURL(subscription + ":pull"),
		Scopes: s.oauthScopes(),
		Body:   &request,
		Out:    &response,
	}
	if err := req.Do(ctx); err != nil {
		return nil, err
	}
	if len(response.ReceivedMessages) == 0 {
		return nil, nil
	}

	logging.Infof(ctx, "Received PubSub notification: %d", len(response.ReceivedMessages))

	// Grab certs to verify signatures on PubSub messages.
	certs, err := signing.FetchCertificatesFromLUCIService(ctx, s.URL)
	if err != nil {
		return nil, err
	}

	// Find maximum AuthDB revision among all verified messages. Invalid messages
	// are skipped (with logging).
	maxRev := int64(-1)
	ackIDs := ([]string)(nil)
	for _, msg := range response.ReceivedMessages {
		if msg.AckID != "" {
			ackIDs = append(ackIDs, msg.AckID)
		}
		notify, err := s.decodeMessage(&msg, certs)
		if err != nil {
			logging.Warningf(ctx, "Bad signature on message %s - %s", msg.Message.MessageID, err)
			continue
		}
		if notify.GetRevision().GetAuthDbRev() > maxRev {
			maxRev = notify.GetRevision().GetAuthDbRev()
		}
	}

	// All message are invalid and skipped? Try to acknowledge them to remove
	// them from the queue, but don't error if it fails.
	if maxRev == -1 {
		err := s.ackMessages(ctx, subscription, ackIDs)
		if err != nil {
			logging.Warningf(ctx, "Failed to ack some PubSub messages (%v) - %s", ackIDs, err)
		}
		return nil, nil
	}

	logging.Infof(ctx, "Auth service notifies us that the most recent AuthDB revision is %d", maxRev)
	return &Notification{
		Revision:     maxRev,
		service:      s,
		subscription: subscription,
		ackIDs:       ackIDs,
	}, nil
}

// ProcessPubSubPush handles incoming PubSub push notification. `body` is
// the entire body of the push HTTP request. Invalid messages are silently
// skipped by returning nil error (to avoid redelivery). The error is still
// logged though.
func (s *AuthService) ProcessPubSubPush(ctx context.Context, body []byte) (*Notification, error) {
	msg := pubSubMessage{}
	if err := json.Unmarshal(body, &msg); err != nil {
		logging.Errorf(ctx, "auth: bad PubSub notification, not JSON - %s", err)
		return nil, nil
	}

	// It's fine to return error here. Certificate fetch can fail due to bad
	// connectivity and we need a retry.
	certs, err := signing.FetchCertificatesFromLUCIService(ctx, s.URL)
	if err != nil {
		return nil, err
	}

	notify, err := s.decodeMessage(&msg, certs)
	if err != nil {
		logging.Errorf(ctx, "auth: bad PubSub notification - %s", err)
		return nil, nil
	}

	return &Notification{
		Revision: notify.GetRevision().GetAuthDbRev(),
		service:  s,
	}, nil
}

///

// decodeMessage checks that the message payload was signed by Auth service key
// and deserializes it.
func (s *AuthService) decodeMessage(m *pubSubMessage, certs *signing.PublicCertificates) (*protocol.ChangeNotification, error) {
	// Key name used to sign.
	keyName := m.Message.Attributes["X-AuthDB-SigKey-v1"]
	if keyName == "" {
		return nil, fmt.Errorf("X-AuthDB-SigKey-v1 attribute is not set")
	}

	// The signature.
	sigBase64 := m.Message.Attributes["X-AuthDB-SigVal-v1"]
	if sigBase64 == "" {
		return nil, fmt.Errorf("X-AuthDB-SigVal-v1 attribute is not set")
	}
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return nil, fmt.Errorf("the message signature is not valid base64 - %s", err)
	}

	// Message body.
	body, err := base64.StdEncoding.DecodeString(m.Message.Data)
	if err != nil {
		return nil, fmt.Errorf("the message body is not valid base64 - %s", err)
	}

	// Validate.
	if err := certs.CheckSignature(keyName, body, sig); err != nil {
		return nil, err
	}

	// Deserialize.
	out := protocol.ChangeNotification{}
	if err := proto.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ackMessages acknowledges processing of pubsub messages.
func (s *AuthService) ackMessages(ctx context.Context, subscription string, ackIDs []string) error {
	if len(ackIDs) == 0 {
		return nil
	}
	request := struct {
		AckIDs []string `json:"ackIds"`
	}{ackIDs}
	req := internal.Request{
		Method: "POST",
		URL:    s.pubSubURL(subscription + ":acknowledge"),
		Scopes: s.oauthScopes(),
		Body:   &request,
	}
	return req.Do(ctx)
}

// GetLatestSnapshotRevision fetches revision number of the latest AuthDB
// snapshot.
func (s *AuthService) GetLatestSnapshotRevision(ctx context.Context) (int64, error) {
	var out struct {
		Snapshot struct {
			Rev int64 `json:"auth_db_rev"`
		} `json:"snapshot"`
	}
	req := internal.Request{
		Method: "GET",
		URL:    s.URL + "/auth_service/api/v1/authdb/revisions/latest?skip_body=1",
		Scopes: s.oauthScopes(),
		Out:    &out,
	}
	if err := req.Do(ctx); err != nil {
		return 0, err
	}
	return out.Snapshot.Rev, nil
}

// GetSnapshot fetches AuthDB snapshot at given revision, unpacks and
// validates it.
func (s *AuthService) GetSnapshot(ctx context.Context, rev int64) (*Snapshot, error) {
	// Fetch.
	var out struct {
		Snapshot struct {
			Rev          int64  `json:"auth_db_rev"`
			SHA256       string `json:"sha256"`
			Created      int64  `json:"created_ts"`
			DeflatedBody string `json:"deflated_body"`
		} `json:"snapshot"`
	}
	req := internal.Request{
		Method: "GET",
		URL:    fmt.Sprintf("%s/auth_service/api/v1/authdb/revisions/%d", s.URL, rev),
		Scopes: s.oauthScopes(),
		Out:    &out,
	}
	if err := req.Do(ctx); err != nil {
		return nil, err
	}
	if out.Snapshot.Rev != rev {
		return nil, fmt.Errorf(
			"auth: unexpected revision %d of AuthDB snapshot, expecting %d", out.Snapshot.Rev, rev)
	}

	// Decode base64.
	deflated, err := base64.StdEncoding.DecodeString(out.Snapshot.DeflatedBody)
	if err != nil {
		return nil, err
	}

	// Inflate and calculate SHA256 of inflated blob.
	r, err := zlib.NewReader(bytes.NewReader(deflated))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	blob := bytes.Buffer{}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(&blob, hash), r); err != nil {
		return nil, err
	}

	// Make sure SHA256 is correct.
	digest := hex.EncodeToString(hash.Sum(nil))
	if digest != out.Snapshot.SHA256 {
		return nil, fmt.Errorf("auth: wrong SHA256 digest of AuthDB snapshot at rev %d", rev)
	}

	// Unmarshal.
	msg := protocol.ReplicationPushRequest{}
	if err := proto.Unmarshal(blob.Bytes(), &msg); err != nil {
		return nil, err
	}
	revMsg := msg.GetRevision()
	if revMsg == nil || revMsg.GetAuthDbRev() != rev {
		return nil, fmt.Errorf("auth: bad 'revision' field in proto message (%v)", revMsg)
	}

	// Log some stats.
	logging.Infof(
		ctx, "auth: fetched AuthDB snapshot at rev %d (inflated size is %.1f Kb, deflated size is %.1f Kb)",
		rev, float32(len(blob.Bytes()))/1024, float32(len(deflated))/1024)
	logging.Infof(
		ctx, "auth: AuthDB snapshot generated by %s (using components.auth v%s)",
		revMsg.GetPrimaryId(), msg.GetAuthCodeVersion())

	// Validate AuthDB by constructing SnapshotDB from it (with validation).
	authDB := msg.GetAuthDb()
	if authDB == nil {
		return nil, fmt.Errorf("auth: 'auth_db' field is missing in proto message (%v)", &msg)
	}
	if _, err := authdb.NewSnapshotDB(authDB, s.URL, rev, true); err != nil {
		return nil, err
	}
	return &Snapshot{
		AuthDB:         authDB,
		AuthServiceURL: s.URL,
		Rev:            rev,
		Created:        time.Unix(0, out.Snapshot.Created*1000),
	}, nil
}

// DeflateAuthDB serializes AuthDB to byte buffer and compresses it with zlib.
func DeflateAuthDB(msg *protocol.AuthDB) ([]byte, error) {
	blob, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	out := bytes.Buffer{}
	writer := zlib.NewWriter(&out)
	_, err1 := io.Copy(writer, bytes.NewReader(blob))
	err2 := writer.Close()
	if err1 != nil {
		return nil, err1
	}
	if err2 != nil {
		return nil, err2
	}
	return out.Bytes(), nil
}

// InflateAuthDB is reverse of DeflateAuthDB. It decompresses and deserializes
// AuthDB message.
func InflateAuthDB(blob []byte) (*protocol.AuthDB, error) {
	reader, err := zlib.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	inflated := bytes.Buffer{}
	if _, err := io.Copy(&inflated, reader); err != nil {
		return nil, err
	}
	out := &protocol.AuthDB{}
	if err := proto.Unmarshal(inflated.Bytes(), out); err != nil {
		return nil, err
	}
	return out, nil
}
