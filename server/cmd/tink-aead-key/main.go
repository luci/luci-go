// Copyright 2021 The LUCI Authors.
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

// Executable tink-aead-key allows to generate and rotate Tink AES 256 GCM
// key sets stored in a clear text JSON form in Google Secret Manager.
//
// Usage:
//   $ tink-aead-key login
//   $ tink-aead-key create sm://<project>/<secret>
//   $ tink-aead-key rotate sm://<project>/<secret>
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/golang/protobuf/proto"
	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
	"github.com/maruel/subcommands"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// KeyAccessor knows how to read and write secret byte blobs.
type KeyAccessor interface {
	Read() ([]byte, error)
	Write(blob []byte) error
	Close() error
}

////////////////////////////////////////////////////////////////////////////////

type fileSystemAccessor struct {
	f *os.File
}

func newFileSystemAccessor(path string) (KeyAccessor, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	return &fileSystemAccessor{f}, nil
}

func (a *fileSystemAccessor) Read() ([]byte, error) {
	if _, err := a.f.Seek(0, os.SEEK_SET); err != nil {
		return nil, err
	}
	return io.ReadAll(a.f)
}

func (a *fileSystemAccessor) Write(blob []byte) error {
	if _, err := a.f.Seek(0, os.SEEK_SET); err != nil {
		return err
	}
	if _, err := a.f.Write(blob); err != nil {
		return err
	}
	return a.f.Truncate(int64(len(blob)))
}

func (a *fileSystemAccessor) Close() error {
	return a.f.Close()
}

////////////////////////////////////////////////////////////////////////////////

type secretManagerAccessor struct {
	ctx     context.Context
	client  *secretmanager.Client
	project string
	secret  string
}

func newSecretManagerAccessor(ctx context.Context, uri string, ts oauth2.TokenSource) (KeyAccessor, error) {
	chunks := strings.Split(strings.TrimPrefix(uri, "sm://"), "/")
	if len(chunks) != 2 {
		return nil, fmt.Errorf("sm://... URL should have form sm://<project>/<secret>")
	}
	client, err := secretmanager.NewClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to setup Secret Manager client: %w", err)
	}
	return &secretManagerAccessor{
		ctx:     ctx,
		client:  client,
		project: chunks[0],
		secret:  chunks[1],
	}, nil
}

func (a *secretManagerAccessor) Read() ([]byte, error) {
	latest, err := a.client.AccessSecretVersion(a.ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", a.project, a.secret),
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get the latest version of the secret: %w", err)
	}
	return latest.Payload.Data, nil
}

func (a *secretManagerAccessor) Write(blob []byte) error {
	attempt := 0
	for {
		attempt += 1

		version, err := a.client.AddSecretVersion(a.ctx, &secretmanagerpb.AddSecretVersionRequest{
			Parent: fmt.Sprintf("projects/%s/secrets/%s", a.project, a.secret),
			Payload: &secretmanagerpb.SecretPayload{
				Data: blob,
			},
		})

		switch {
		case err == nil:
			// Best effort cleanup of the previous version which is not needed
			// anymore: we always store the entire Tink key set in the latest version.
			if err := a.destroyPreviousVersion(version.Name); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to destroy the previous secret version: %s.\n", err)
			}
			return nil

		case status.Code(err) == codes.NotFound && attempt == 1:
			// Perhaps the secret object doesn't exist yet. Try to create it.
			_, err = a.client.CreateSecret(a.ctx, &secretmanagerpb.CreateSecretRequest{
				Parent:   fmt.Sprintf("projects/%s", a.project),
				SecretId: a.secret,
				Secret: &secretmanagerpb.Secret{
					Replication: &secretmanagerpb.Replication{
						Replication: &secretmanagerpb.Replication_Automatic_{},
					},
				},
			})
			if err != nil {
				return fmt.Errorf("failed to create the secret: %w", err)
			}
			continue // retry AddSecretVersion

		default:
			return fmt.Errorf("failed to create the secret version: %w", err)
		}
	}
}

func (a *secretManagerAccessor) Close() error {
	return a.client.Close()
}

func (a *secretManagerAccessor) destroyPreviousVersion(latest string) error {
	// The version name has format projects/.../secrets/.../versions/<number>.
	// We want to grab the version number to peek at the previous one (if any).
	idx := strings.LastIndex(latest, "/")
	if idx == -1 {
		return fmt.Errorf("unexpected version name format %q", latest)
	}
	version, err := strconv.ParseInt(latest[idx+1:], 10, 64)
	if err != nil {
		return fmt.Errorf("unexpected version name format %q", latest)
	}
	previous := version - 1
	if previous == 0 {
		return nil
	}
	_, err = a.client.DestroySecretVersion(a.ctx, &secretmanagerpb.DestroySecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/%d", a.project, a.secret, previous),
	})
	return err
}

////////////////////////////////////////////////////////////////////////////////
// CLI boilerplate.

var authOpts = chromeinfra.SetDefaultAuthOptions(auth.Options{
	Scopes: []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
	},
})

func main() {
	os.Exit(subcommands.Run(&cli.Application{
		Name:  "tink-aead-key",
		Title: "Create or rotate Tink AES 256 GCM keys.",
		Context: func(ctx context.Context) context.Context {
			return gologger.StdConfig.Use(ctx)
		},
		Commands: []*subcommands.Command{
			subcommands.CmdHelp,

			authcli.SubcommandLogin(authOpts, "login", false),
			authcli.SubcommandLogout(authOpts, "logout", false),

			{
				UsageLine: "create <key>",
				ShortDesc: "creates a new encryption key",
				LongDesc: text.Doc(`
					Creates a new encryption key at the given path.

					The path can be a file system path or a sm://<project>/<key> reference
					to a Cloud Secret Manager key to create.
				`),
				CommandRun: func() subcommands.CommandRun {
					return (&commandRun{}).init(createKey)
				},
			},

			{
				UsageLine: "rotate <key>",
				ShortDesc: "rotates an existing encryption key",
				LongDesc: text.Doc(`
					Rotates an existing encryption key at the given path.

					If there's no such key, it is created instead. Keeps only 10 last keys
					in the key set.

					The path can be a file system path or a sm://<project>/<key> reference
					to a Cloud Secret Manager key to create.
				`),
				CommandRun: func() subcommands.CommandRun {
					return (&commandRun{}).init(rotateKey)
				},
			},
		},
	}, nil))
}

type commandRun struct {
	subcommands.CommandRunBase
	authFlags authcli.Flags

	cb func(ctx context.Context, accessor KeyAccessor) error
}

func (c *commandRun) init(cb func(ctx context.Context, accessor KeyAccessor) error) *commandRun {
	c.authFlags.Register(&c.Flags, authOpts)
	c.cb = cb
	return c
}

func (c *commandRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, c, env)

	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Expecting exactly one positional argument: the key path.\n")
		return 2
	}
	keyPath := args[0]

	authOpts, err := c.authFlags.Options()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Bad auth options: %s.\n", err)
		return 2
	}

	var accessor KeyAccessor
	if strings.HasPrefix(keyPath, "sm://") {
		var ts oauth2.TokenSource
		switch ts, err = auth.NewAuthenticator(ctx, auth.SilentLogin, authOpts).TokenSource(); {
		case err == auth.ErrLoginRequired:
			fmt.Fprintf(os.Stderr, "Need to login first. Run `auth-login` subcommand.\n")
			return 3
		case err != nil:
			fmt.Fprintf(os.Stderr, "Failed to setup credentials: %s.\n", err)
			return 2
		default:
			accessor, err = newSecretManagerAccessor(ctx, keyPath, ts)
		}
	} else {
		accessor, err = newFileSystemAccessor(keyPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to access the key: %s.\n", err)
		return 2
	}
	defer accessor.Close()

	if err = c.cb(ctx, accessor); err != nil {
		fmt.Fprintf(os.Stderr, "Operation failed: %s.\n", err)
		return 1
	}
	if err = accessor.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to flush the key: %s.\n", err)
		return 1
	}
	return 0
}

////////////////////////////////////////////////////////////////////////////////
// Actual implementation of commands.

// How many keys to keep in a key set. Older keys are deleted.
const maxKeysInKeyset = 10

func dumpKeysetInfo(kh *keyset.Handle) {
	fmt.Printf("%s\n", protojson.Format(proto.MessageV2(kh.KeysetInfo())))
}

func createKey(ctx context.Context, accessor KeyAccessor) error {
	// Check there's no key or it can't be read (which can happen if the caller
	// has the permission to create keys in Secret Manager, but not read them).
	// Other unexpected errors ignored here will be detected when attempting
	// to write the key.
	if blob, err := accessor.Read(); err == nil && len(blob) != 0 {
		return fmt.Errorf("the key already exists, refusing to override it")
	}

	// Generate the new key set with a single random key.
	kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return err
	}

	// Serialize it to a clear text JSON.
	buf := &bytes.Buffer{}
	if err = insecurecleartextkeyset.Write(kh, keyset.NewJSONWriter(buf)); err != nil {
		return err
	}

	// And write it to the storage.
	if err := accessor.Write(buf.Bytes()); err != nil {
		return err
	}
	fmt.Println("The new key was generated and stored:")
	dumpKeysetInfo(kh)
	return nil
}

func rotateKey(ctx context.Context, accessor KeyAccessor) error {
	// Read the existing key set, check it actually exists.
	blob, err := accessor.Read()
	if err != nil {
		return err
	}
	if len(blob) == 0 {
		fmt.Println("There's no such key, generating a new one.")
		return createKey(ctx, accessor)
	}

	// Load the existing key set.
	kh, err := insecurecleartextkeyset.Read(keyset.NewJSONReader(bytes.NewReader(blob)))
	if err != nil {
		return err
	}

	// Rotate keys there.
	mgr := keyset.NewManagerFromHandle(kh)
	if err := mgr.Rotate(aead.AES256GCMKeyTemplate()); err != nil {
		return err
	}

	// Delete older keys to avoid growing the key set indefinitely.
	if kh, err = truncateKeySet(kh); err != nil {
		return err
	}

	// Serialize the resulting key set to clear text JSON.
	buf := &bytes.Buffer{}
	if err = insecurecleartextkeyset.Write(kh, keyset.NewJSONWriter(buf)); err != nil {
		return err
	}

	// And write the result to the storage.
	if err := accessor.Write(buf.Bytes()); err != nil {
		return err
	}
	fmt.Println("The key was rotated:")
	dumpKeysetInfo(kh)
	return nil
}

func truncateKeySet(kh *keyset.Handle) (*keyset.Handle, error) {
	// Convert the handle to protos with the list of keys.
	protos := &keyset.MemReaderWriter{}
	if err := insecurecleartextkeyset.Write(kh, protos); err != nil {
		return nil, err
	}

	// We are using insecurecleartextkeyset, so should have no encrypted key sets.
	if protos.EncryptedKeyset != nil {
		panic("EncryptedKeyset is unexpectedly not nil")
	}

	// If there's no need to truncate anything, just return the original handler.
	l := len(protos.Keyset.Key)
	if l <= maxKeysInKeyset {
		return kh, nil
	}

	// More recent keys are at the end. Truncate the head.
	protos.Keyset.Key = protos.Keyset.Key[l-maxKeysInKeyset:]

	// Make sure the primary key is still in the key set.
	found := false
	for _, k := range protos.Keyset.Key {
		if k.KeyId == protos.Keyset.PrimaryKeyId {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("the primary key is not in the last %d of keys", maxKeysInKeyset)
	}

	// Convert protos back to the handle.
	return insecurecleartextkeyset.Read(protos)
}
