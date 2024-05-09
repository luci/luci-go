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

//go:build !copybara
// +build !copybara

package internal

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"go.chromium.org/luci/auth/loginsessionspb"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/system/terminal"
	"go.chromium.org/luci/grpc/prpc"
)

var spinnerChars = []rune("⣾⣽⣻⢿⡿⣟⣯⣷")

type loginSessionTokenProvider struct {
	loginSessionsHost string
	clientID          string
	clientSecret      string
	scopes            []string
	transport         http.RoundTripper
	cacheKey          CacheKey
}

func init() {
	NewLoginSessionTokenProvider = func(ctx context.Context, loginSessionsHost, clientID, clientSecret string, scopes []string, transport http.RoundTripper) (TokenProvider, error) {
		return &loginSessionTokenProvider{
			loginSessionsHost: loginSessionsHost,
			clientID:          clientID,
			clientSecret:      clientSecret,
			scopes:            scopes,
			transport:         transport,
			// Reuse the same key as userAuthTokenProvider to share refresh tokens
			// between two methods. They are compatible.
			cacheKey: CacheKey{
				Key:    fmt.Sprintf("user/%s", clientID),
				Scopes: scopes,
			},
		}, nil
	}
}

func (p *loginSessionTokenProvider) RequiresInteraction() bool {
	return true
}

func (p *loginSessionTokenProvider) Lightweight() bool {
	return false
}

func (p *loginSessionTokenProvider) Email() string {
	// We don't know the email before user logs in.
	return UnknownEmail
}

func (p *loginSessionTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *loginSessionTokenProvider) MintToken(ctx context.Context, base *Token) (*Token, error) {
	// It is never correct to use this login method on bots.
	if os.Getenv("SWARMING_HEADLESS") == "1" {
		return nil, errors.Reason("interactive login flow is forbidden on bots").Err()
	}
	// Check if stdout is really a terminal a real user can interact with.
	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		return nil, errors.Reason("interactive login flow requires the stdout to be attached to a terminal").Err()
	}

	// The list of scopes is displayed on the consent page as well, but show it
	// in the terminal too, for clarity.
	fmt.Println("Getting a refresh token with following OAuth scopes:")
	for _, scope := range p.scopes {
		fmt.Printf("  * %s\n", scope)
	}
	fmt.Println()

	// pRPC client to use for interacting with the sessions server.
	httpClient := &http.Client{Transport: p.transport}
	sessions := loginsessionspb.NewLoginSessionsClient(&prpc.Client{
		C:    httpClient,
		Host: p.loginSessionsHost,
	})

	// Generate a code verifier (a random string) and corresponding challenge for
	// PKCE protocol. They are used to make sure only us can exchange the
	// authorization code for tokens (because only we know the code verifier).
	codeVerifier := oauth2.GenerateVerifier()
	codeChallenge := oauth2.S256ChallengeFromVerifier(codeVerifier)

	// Collect some information about the running environment to show to the user
	// so they can understand better what is invoking the login session. Both are
	// best effort and optional (and easily spoofable).
	executable, _ := os.Executable()
	hostname, _ := os.Hostname()

	// Start a new login session that we'll ask the user to complete.
	session, err := sessions.CreateLoginSession(ctx, &loginsessionspb.CreateLoginSessionRequest{
		OauthClientId:          p.clientID,
		OauthScopes:            p.scopes,
		OauthS256CodeChallenge: codeChallenge,
		ExecutableName:         filepath.Base(executable),
		ClientHostname:         hostname,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create the login session").Err()
	}

	useFancyUI, doneUI := EnableVirtualTerminal()
	if !useFancyUI {
		// TODO(crbug/chromium/1411203): Support mode without virtual terminal.
		logging.Warningf(ctx, "Virtual terminal is not enabled.")
	} else {
		defer doneUI()
	}

	fmt.Printf(
		"Visit this link to complete the login flow in the browser. Do not share it with anyone!\n\n%s\n\n",
		session.LoginFlowUrl,
	)

	animationCtrl := startAnimation(ctx)
	defer animationCtrl("", 0)
	animationCtrl(session.ConfirmationCode, session.ConfirmationCodeRefresh.AsDuration())

	// Start polling the session until it moves to a finished state.
	sessionID := session.Id
	sessionPassword := session.Password
	confirmationCode := session.ConfirmationCode
	for session.State == loginsessionspb.LoginSession_PENDING {
		// Use the poll interval suggested by the server, unless it is super small.
		sleep := session.PollInterval.AsDuration()
		if sleep < time.Second {
			sleep = time.Second
		}
		if clock.Sleep(ctx, sleep).Incomplete() {
			return nil, ctx.Err()
		}
		session, err = sessions.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
			LoginSessionId:       sessionID,
			LoginSessionPassword: sessionPassword,
		})
		if err != nil {
			return nil, errors.Annotate(err, "failed to poll the login session").Err()
		}
		// Send new confirmation code to the loop that renders it.
		if confirmationCode != session.ConfirmationCode {
			confirmationCode = session.ConfirmationCode
			animationCtrl(confirmationCode, session.ConfirmationCodeRefresh.AsDuration())
		}
	}
	animationCtrl("", 0)

	// A session can expire or fail (if the user cancels it).
	if session.State != loginsessionspb.LoginSession_SUCCEEDED {
		fmt.Printf("Login session failed with status %s!\n", session.State)
		if session.OauthError != "" {
			fmt.Printf("OAuth error: %s\n", session.OauthError)
		}
		return nil, errors.Reason("the login flow failed").Err()
	}

	// We've got the authorization code and the redirect URL needed to complete
	// the flow on our side. Note that we need to use codeVerifier here that only
	// we know.
	tok, err := (&oauth2.Config{
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
		RedirectURL:  session.OauthRedirectUrl,
		Endpoint:     google.Endpoint,
	}).Exchange(ctx,
		session.OauthAuthorizationCode,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		return nil, err
	}
	return processProviderReply(ctx, tok, "")
}

func (p *loginSessionTokenProvider) RefreshToken(ctx context.Context, prev, base *Token) (*Token, error) {
	// Clear expiration time to force token refresh. Do not use 0 since it means
	// that token never expires.
	t := prev.Token
	t.Expiry = time.Unix(1, 0)
	cfg := &oauth2.Config{
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
		Endpoint:     google.Endpoint,
	}
	switch newTok, err := grabToken(cfg.TokenSource(ctx, &t)); {
	case err == nil:
		return processProviderReply(ctx, newTok, prev.Email)
	case transient.Tag.In(err):
		logging.Warningf(ctx, "Transient error when refreshing the token - %s", err)
		return nil, err
	default:
		logging.Warningf(ctx, "Bad refresh token - %s", err)
		return nil, ErrBadRefreshToken
	}
}

// codeAndExp represents a login confirmation code and its expiration time.
type codeAndExp struct {
	code string        // the code itself
	exp  time.Time     // moment it expires
	life time.Duration // its lifetime when we got it
}

// animator defines the functions that must be implemented to render
// the code to the terminal.
type animator interface {
	// setup prepares the terminal and/or animator to display the confirmation code.
	setup()

	// updateCode updates the confirmation code being displayed by the animator.
	updateCode(string)

	// refreshAnimation updates any spinner animation using the remaining lifetime of
	// current confirmation code.
	refreshAnimation(context.Context, codeAndExp)

	// finish is expected to print any information after the login has been completed.
	finish()
}

// printTerminalCursorDown is a helper function to move the terminal cursor down.
func printTerminalCursorDown(lines int) {
	fmt.Printf("\033[%dB", lines)
}

// printTerminalCursorUp is a helper function to move the terminal cursor up.
func printTerminalCursorUp(lines int) {
	fmt.Printf("\033[%dA", lines)
}

// printTerminalLine resets the cursor to the start of the current line and prints
// a message, erasing anything previously displayed on the current line.
func printTerminalLine(msg string, args ...any) {
	fmt.Printf("\r\033[2K"+msg+"\n", args...)
}

// dumbTerminal is a terminal that does not support some cursor control characters.
type dumbTerminal struct{}

func (dt *dumbTerminal) setup() {}

func (dt *dumbTerminal) updateCode(code string) {
	fmt.Printf("%s\r", code)
}

// refreshAnimation does nothing for dumbTerminal, we do not animate.
func (dt *dumbTerminal) refreshAnimation(ctx context.Context, current codeAndExp) {}

func (dt *dumbTerminal) finish() {
	fmt.Printf("Done!\n")
}

func dumbAnimator() *dumbTerminal {
	return &dumbTerminal{}
}

// smartTerminal is a terminal that supports the cursor control characters needed to animate the code refresh.
type smartTerminal struct {
	round int
}

func (st *smartTerminal) setup() {
	// allocate lines, these will be overridden in smartAnimator.
	fmt.Printf("\n\n\n\n")
}

// printCode uses control characters with a smart terminal to move the terminal cursor up and down,
// overwriting previous code.
func (st *smartTerminal) updateCode(code string) {
	printTerminalCursorUp(4)
	printTerminalLine("%s", code)
	printTerminalCursorDown(3)
}

// refreshAnimation re-renders the loading bar animation and spinner animation as the
// code expiry goes down.
func (st *smartTerminal) refreshAnimation(ctx context.Context, current codeAndExp) {
	spinner := string(spinnerChars[st.round%len(spinnerChars)])
	st.round += 1

	// Calculate a portion of code's lifetime left.
	ratio := float32(time.Until(current.exp).Seconds() / current.life.Seconds())

	// Convert it into a number of progress bar characters to print.
	total := len(current.code)
	filled := int(ratio*float32(total)) + 1
	if filled < 0 {
		filled = 0
	} else if filled > total {
		filled = total
	}

	// Redraw everything but code.
	printTerminalCursorUp(3)
	printTerminalLine("%s%s", strings.Repeat("─", filled), strings.Repeat(" ", total-filled))
	printTerminalLine("")
	printTerminalLine("Waiting for the login flow to complete in the browser %s", spinner)
}

func (st *smartTerminal) finish() {
	// Redraw the last line replacing the spinner with "Done".
	printTerminalCursorUp(1)
	printTerminalLine("Waiting for the login flow to complete in the browser. Done!\n")
}

func smartAnimator() *smartTerminal {
	return &smartTerminal{
		round: 0,
	}
}

// startAnimation starts background rendering of the most recent confirmation
// code (plus some cute spinner animation).
//
// Returns a function that can be used to control the animation. Passing it
// a non-empty string would replace the confirmation code. Passing it an empty
// string would stop the animation.
func startAnimation(ctx context.Context) (ctrl func(string, time.Duration)) {
	spinCh := make(chan codeAndExp)
	done := false

	spinWG := sync.WaitGroup{}
	spinWG.Add(1)

	fmt.Printf("When asked, use this confirmation code (it refreshes with time):\n\n")
	var a animator
	a = dumbAnimator()
	if !IsDumbTerminal() {
		a = smartAnimator()
	}

	prevCode := ""

	go func() {
		defer spinWG.Done()
		current := codeAndExp{}
		a.setup()
	loop:
		for {
			select {
			case code, ok := <-spinCh:
				if !ok {
					break loop
				}
				current = code
				if current.code != prevCode {
					a.updateCode(current.code)
					prevCode = current.code
				}
			case res := <-clock.After(ctx, 100*time.Millisecond):
				if res.Err != nil {
					break loop
				}
			}

			// Wait until we get the first confirmation code before rendering
			// anything. This should be fast, since we already know it by the time
			// the goroutine starts, so we just wait for local goroutines to
			// "synchronize".
			if current.code == "" {
				continue
			}
			a.refreshAnimation(ctx, current)
		}
		a.finish()
	}()
	return func(code string, exp time.Duration) {
		if !done {
			if code == "" {
				done = true
				close(spinCh)
				spinWG.Wait()
			} else {
				spinCh <- codeAndExp{code, time.Now().Add(exp), exp}
			}
		}
	}
}

// processProviderReply transforms oauth2.Token into Token by extracting some
// useful information from it.
//
// May make an RPC to the token info endpoint.
func processProviderReply(ctx context.Context, tok *oauth2.Token, email string) (*Token, error) {
	// If have the ID token, parse its payload to see the expiry and the email.
	// Note that we don't verify the signature. We just got the token from the
	// provider we trust.
	var claims *IDTokenClaims
	var idToken string
	var err error
	if idToken, _ = tok.Extra("id_token").(string); idToken != "" {
		if claims, err = ParseIDTokenClaims(idToken); err != nil {
			return nil, err
		}
	} else {
		idToken = NoIDToken
	}

	// ID token has the freshest email.
	if claims != nil && claims.EmailVerified && claims.Email != "" {
		email = claims.Email
	} else if email == "" {
		// If we still don't know the email associated with the credentials, make
		// an RPC to the token info endpoint to get it.
		if email, err = grabEmail(ctx, tok); err != nil {
			return nil, err
		}
	}

	// We rely on `tok` expiry to know when to refresh both the access and ID
	// tokens. Usually they have roughly the same expiry. Check this.
	if claims != nil {
		idTokenExpiry := time.Unix(claims.Exp, 0)
		delta := idTokenExpiry.Sub(tok.Expiry)
		if delta < 0 {
			delta = -delta
		}
		if delta > time.Minute {
			logging.Warningf(ctx, "The ID token and access tokens have unexpectedly large discrepancy in expiration times: %v", delta)
		}
		if idTokenExpiry.Before(tok.Expiry) {
			tok.Expiry = idTokenExpiry
		}
	}

	return &Token{
		Token:   *tok,
		IDToken: idToken,
		Email:   email,
	}, nil
}

// grabEmail fetches an email associated with the given token.
//
// May return (NoEmail, nil) if the token can't be resolved into an email.
func grabEmail(ctx context.Context, tok *oauth2.Token) (string, error) {
	info, err := googleoauth.GetTokenInfo(ctx, googleoauth.TokenInfoParams{
		AccessToken: tok.AccessToken,
	})
	if err != nil {
		return "", err
	}
	if info.Email == "" {
		return NoEmail, nil
	}
	return info.Email, nil
}
