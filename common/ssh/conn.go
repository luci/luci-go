// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package ssh defines utilities for working with SSH protocols.
package ssh

import (
	"io"
	"net"

	"go.chromium.org/luci/common/errors"
)

// AgentMessageCode is an SSH agent message code.
// See https://www.ietf.org/archive/id/draft-miller-ssh-agent-11.html
type AgentMessageCode uint8

// SSH Agent Client message codes.
// These correspond to SSH_AGENTC_* codes in the SSH agent specification.
const (
	AgentcExtension AgentMessageCode = 27
)

// SSH Agent message codes.
// These correspond to SSH_AGENT_* codes in the SSH agent specification.
const (
	AgentFailure          AgentMessageCode = 5
	AgentSuccess          AgentMessageCode = 6
	AgentExtensionFailure AgentMessageCode = 28
)

// AgentExtensionRequest represents an SSH Agent Extension Request.
type AgentExtensionRequest struct {
	// ExtensionType represents the extension's type.
	ExtensionType string

	// ExtensionData is arbitrary amount of bytes to be passed to the extension.
	ExtensionData []byte
}

// AgentMessage represent an SSH Agent message.
type AgentMessage struct {
	Code    AgentMessageCode
	Payload []byte
}

// AgentNetConn represents a network connection between an SSH Agent and its client.
//
// It encodes and decodes SSH Agent messages to/from on-the-wire bytes. See
// https://www.ietf.org/archive/id/draft-miller-ssh-agent-11.html.
type AgentNetConn struct {
	conn net.Conn
}

// Read reads and returns the next message.
func (c AgentNetConn) Read() (*AgentMessage, error) {
	return decodeAgentMessage(c.conn)
}

// Write writes `msg`.
func (c AgentNetConn) Write(msg AgentMessage) error {
	return encodeAgentMessage(c.conn, msg)
}

// Close closes the connection.
func (c AgentNetConn) Close() error {
	return c.conn.Close()
}

// RemoteAddr returns the net.Addr of the remote peer.
func (c AgentNetConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// NewAgentConn wraps a net.Conn to AgentConn.
func NewAgentConn(conn net.Conn) AgentNetConn {
	return AgentNetConn{conn}
}

func decodeAgentMessage(r io.Reader) (*AgentMessage, error) {
	msgBytes, err := decodeLengthPrefix(r)
	if err != nil {
		return nil, err
	}

	if len(msgBytes) == 0 {
		return nil, errors.New("bad ssh agent message, got message length of 0, but the length should be at least 1")
	}

	msg := AgentMessage{
		Code:    AgentMessageCode(msgBytes[0]),
		Payload: msgBytes[1:],
	}
	return &msg, nil
}

func encodeAgentMessage(w io.Writer, msg AgentMessage) error {
	// Message length is 1 byte of `code` + len bytes of payload.
	msgLen := 1 + len(msg.Payload)

	msgBytes := make([]byte, msgLen)
	msgBytes[0] = byte(msg.Code)
	copy(msgBytes[1:], msg.Payload)

	return encodeLengthPrefix(w, msgBytes)
}
