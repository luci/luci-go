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

package ssh

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// TestEncoding covers the implementation of SSH agent message encoding and decoding.
func TestEncoding(t *testing.T) {
	t.Parallel()

	t.Run("encode simple message", func(t *testing.T) {
		t.Parallel()

		msg := AgentMessage{
			Code:    AgentSuccess,
			Payload: []byte("test-payload"),
		}

		var buf bytes.Buffer
		err := encodeAgentMessage(&buf, msg)
		assert.NoErr(t, err)

		// Verify the encoded data. The first 4 bytes are the length prefix.
		var length uint32
		err = binary.Read(&buf, binary.BigEndian, &length)
		assert.NoErr(t, err)
		assert.Loosely(t, length, should.Equal(uint32(1+len(msg.Payload))))

		// The rest is the message.
		body := buf.Bytes()
		assert.Loosely(t, body[0], should.Equal(byte(msg.Code)))
		assert.Loosely(t, body[1:], should.Match(msg.Payload))
	})

	t.Run("decode simple message", func(t *testing.T) {
		t.Parallel()

		expectedCode := AgentFailure
		expectedPayload := []byte("a failure occurred")
		msgLen := 1 + len(expectedPayload)

		// Prepare a buffer with a well-formed message.
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, uint32(msgLen))
		buf.WriteByte(byte(expectedCode))
		buf.Write(expectedPayload)

		// Attempt to decode it.
		msg, err := decodeAgentMessage(&buf)

		// Verify the results.
		assert.NoErr(t, err)
		assert.Loosely(t, msg.Code, should.Equal(expectedCode))
		assert.Loosely(t, msg.Payload, should.Match(expectedPayload))
	})

	t.Run("decode multiple messages", func(t *testing.T) {
		t.Parallel()

		msg1 := AgentMessage{
			Code:    AgentSuccess,
			Payload: []byte{0xDE, 0xCA, 0xCA, 0xFE},
		}
		msg2 := AgentMessage{
			Code:    AgentFailure,
			Payload: []byte{0xCA, 0xFE},
		}

		var buf bytes.Buffer
		err := encodeAgentMessage(&buf, msg1)
		assert.NoErr(t, err)
		err = encodeAgentMessage(&buf, msg2)
		assert.NoErr(t, err)

		// Decode first message.
		decoded1, err := decodeAgentMessage(&buf)
		assert.NoErr(t, err)
		assert.Loosely(t, *decoded1, should.Match(msg1))

		// Decode second message.
		decoded2, err := decodeAgentMessage(&buf)
		assert.NoErr(t, err)
		assert.Loosely(t, *decoded2, should.Match(msg2))
	})

	t.Run("decode with EOF on length read", func(t *testing.T) {
		t.Parallel()
		// A partial length will cause an EOF.
		buf := bytes.NewReader([]byte{0, 0, 0})
		_, err := decodeAgentMessage(buf)
		assert.ErrIsLike(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("decode with EOF on body read", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, uint32(10)) // length 10
		buf.WriteString("short")                         // body shorter than 10

		_, err := decodeAgentMessage(&buf)
		assert.ErrIsLike(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("decode with zero-length message", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		binary.Write(&buf, binary.BigEndian, uint32(0))
		_, err := decodeAgentMessage(&buf)
		assert.Loosely(t, err, should.ErrLike("bad ssh agent message, got message length of 0, but the length should be at least 1"))
	})
}

// TestAgentNetConn covers the AgentNetConn wrapper.
func TestAgentNetConn(t *testing.T) {
	t.Parallel()

	newServerAndClient := func() (AgentNetConn, AgentNetConn) {
		s, c := net.Pipe()
		return NewAgentConn(s), NewAgentConn(c)
	}

	t.Run("read and write", func(t *testing.T) {
		t.Parallel()

		client, server := newServerAndClient()
		defer client.Close()
		defer server.Close()

		msg := AgentMessage{
			Code:    AgentSuccess,
			Payload: []byte("hello"),
		}

		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			wg.Done()
			err := client.Write(msg)
			assert.NoErr(t, err)
		}()

		// Verify on the other side of the pipe that it was encoded correctly.
		receivedMsg, err := server.Read()
		assert.NoErr(t, err)
		assert.Loosely(t, *receivedMsg, should.Match(msg))

		wg.Wait()
	})

	t.Run("close", func(t *testing.T) {
		t.Parallel()

		client, server := newServerAndClient()
		defer client.Close()
		defer server.Close()

		// Close client, which should close the server side of the pipe.
		err := client.Close()
		assert.NoErr(t, err)

		// Reads from the other end should now fail with EOF.
		_, err = server.Read()
		assert.ErrIsLike(t, err, io.EOF)
	})

	t.Run("remote addr", func(t *testing.T) {
		t.Parallel()
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()

		agentConn := NewAgentConn(client)
		// net.Pipe conn's remote addr is its peer's local addr.
		assert.Loosely(t, agentConn.RemoteAddr(), should.Equal(server.LocalAddr()))
	})
}
