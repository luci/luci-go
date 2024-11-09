// Copyright 2022 The LUCI Authors.
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

package testutil

import (
	"context"
	"time"

	"cloud.google.com/go/storage"

	"go.chromium.org/luci/common/clock"
)

const (
	// Client email for testing.
	ClientEmail = "serviceacct@example.com"

	// Private key from inactive service account for testing.
	PrivateKey = "-----BEGIN PRIVATE KEY-----\nMIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQCsPzMirIottfQ2\nryjQmPWocSEeGo7f7Q4/tMQXHlXFzo93AGgU2t+clEj9L5loNhLVq+vk+qmnyDz5\nQ04y8jVWyMYzzGNNrGRW/yaYqnqlKZCy1O3bmnNjV7EDbC/jE1ZLBY0U3HaSHfn6\nS9ND8MXdgD0/ulRTWwq6vU8/w6i5tYsU7n2LLlQTl1fQ7/emO9nYcCFJezHZVa0H\nmeWsdHwWsok0skwQYQNIzP3JF9BpR5gJT2gNge6KopDesJeLoLzaX7cUnDn+CAnn\nLuLDwwSsIVKyVxhBFsFXPplgpaQRwmGzwEbf/Xpt9qo26w2UMgn30jsOaKlSeAX8\ncS6ViF+tAgMBAAECggEACKRuJCP8leEOhQziUx8Nmls8wmYqO4WJJLyk5xUMUC22\nSI4CauN1e0V8aQmxnIc0CDkFT7qc9xBmsMoF+yvobbeKrFApvlyzNyM7tEa/exh8\nDGD/IzjbZ8VfWhDcUTwn5QE9DCoon9m1sG+MBNlokB3OVOt8LieAAREdEBG43kJu\nyQTOkY9BGR2AY1FnAl2VZ/jhNDyrme3tp1sW1BJrawzR7Ujo8DzlVcS2geKA9at7\n55ua5GbHz3hfzFgjVXDfnkWzId6aHypUyqHrSn1SqGEbyXTaleKTc6Pgv0PgkJjG\nhZazWWdSuf1T5Xbs0OhAK9qraoAzT6cXXvMEvvPt6QKBgQDXcZKqJAOnGEU4b9+v\nOdoh+nssdrIOBNMu1m8mYbUVYS1aakc1iDGIIWNM3qAwbG+yNEIi2xi80a2RMw2T\n9RyCNB7yqCXXVKLBiwg9FbKMai6Vpk2bWIrzahM9on7AhCax/X2AeOp+UyYhFEy6\nUFG4aHb8THscL7b515ukSuKb5QKBgQDMq+9PuaB0eHsrmL6q4vHNi3MLgijGg/zu\nAXaPygSYAwYW8KglcuLZPvWrL6OG0+CrfmaWTLsyIZO4Uhdj7MLvX6yK7IMnagvk\nL3xjgxSklEHJAwi5wFeJ8ai/1MIuCn8p2re3CbwISKpvf7Sgs/W4196P4vKvTiAz\njcTiSYFIKQKBgCjMpkS4O0TakMlGTmsFnqyOneLmu4NyIHgfPb9cA4n/9DHKLKAT\noaWxBPgatOVWs7RgtyGYsk+XubHkpC6f3X0+15mGhFwJ+CSE6tN+l2iF9zp52vqP\nQwkjzm7+pdhZbmaIpcq9m1K+9lqPWJRz/3XXuqi+5xWIZ7NaxGvRjqaNAoGAdK2b\nutZ2y48XoI3uPFsuP+A8kJX+CtWZrlE1NtmS7tnicdd19AtfmTuUL6fz0FwfW4Su\nlQZfPT/5B339CaEiq/Xd1kDor+J7rvUHM2+5p+1A54gMRGCLRv92FQ4EON0RC1o9\nm2I4SHysdO3XmjmdXmfp4BsgAKJIJzutvtbqlakCgYB+Cb10z37NJJ+WgjDt+yT2\nyUNH17EAYgWXryfRgTyi2POHuJitd64Xzuy6oBVs3wVveYFM6PIKXlj8/DahYX5I\nR2WIzoCNLL3bEZ+nC6Jofpb4kspoAeRporj29SgesK6QBYWHWX2H645RkRGYGpDo\n51gjy9m/hSNqBbH2zmh04A==\n-----END PRIVATE KEY-----\n"
)

// GetSignedURLOptions used for testing.
func GetSignedURLOptions(ctx context.Context) *storage.SignedURLOptions {
	expiration := clock.Now(ctx).UTC().Add(7 * 24 * time.Hour)

	return &storage.SignedURLOptions{
		Scheme:         storage.SigningSchemeV4,
		Method:         "GET",
		Expires:        expiration,
		GoogleAccessID: ClientEmail,
		PrivateKey:     []byte(PrivateKey),
	}
}
