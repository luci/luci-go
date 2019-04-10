// Copyright 2019 The LUCI Authors.
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

// Package pager implements paging using Unix command "less".
package pager

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/common/system/terminal"
)

// IsEPipe returns true if err represents syscal.EPIPE. Can be used to check if
// writing to stdin of a subprocess failed because the subprocess exited.
func IsEPipe(err error) bool {
	if pe, ok := err.(*os.PathError); ok {
		err = pe.Err
	}
	if se, ok := err.(*os.SyscallError); ok {
		err = se.Err
	}
	return err == syscall.EPIPE
}

func done(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Main implements paging using command "less" if it is available. If os.Stdout
// is not terminal or less is not available in $PATH (e.g. on Windows), Main
// calls fn with out set to os.Stdout. Otherwise creates less subprocess,
// directs its stdout to os.Stdout and calls fn with out set to less stdin. fn's
// context is canceled if the user quits less.
//
// If less exits with a non-zero exit code, Main returns it, otherwise it
// returns the exit code returned by fn.
//
// Example:
//
//   func main() int {
//   	return Main(context.Background(), func(ctx context.Context, out io.WriteCloser) int {
//   		for i := 0; i < 100000 && ctx.Err() == nil; i++ {
//   			fmt.Fprintln(out, i)
//   		}
//   		return 0
//   	})
//   }
//
// After less exits, even succesfully, out.Write returns an error that
// satisfies IsEPipe. If fn checks errors returned by out.Write, it should use
// IsEPipe to test this condition and should return zero code in this case:
//
//  func main() int {
//  	return Main(context.Background(), func(ctx context.Context, out io.WriteCloser) int {
//  		for i := 0; i < 100000 && ctx.Err() == nil; i++ {
//  			if _, err := fmt.Fprintln(out, i); err != nil {
//  				if IsEPipe(err) {
//  					return 0
//  				}
//  				return 1
//  			}
//  		}
//  		return 0
//  	})
//  }
func Main(ctx context.Context, fn func(ctx context.Context, out io.WriteCloser) int) int {
	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		return fn(ctx, os.Stdout)
	}
	lessPath, err := exec.LookPath("less")
	if err != nil {
		// Example in https://godoc.org/os/exec#LookPath
		// implies that an err is returned if the file is not found.
		return fn(ctx, os.Stdout)
	}

	cmd := exec.Command(lessPath, "-FXr")
	cmd.Stdout = os.Stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return done(err)
	}

	if err := cmd.Start(); err != nil {
		return done(err)
	}

	// Swallow interrupts. Less is supposed to be quit by pressing q.
	// In particular, it does not respond to Ctrl+C.
	sigC := make(chan os.Signal, 1)
	signal.Notify(sigC, os.Interrupt, os.Kill)
	defer signal.Stop(sigC)

	ctx, cancel := context.WithCancel(ctx)
	exitCodeC := make(chan int, 1)
	go func() {
		exitCode := fn(ctx, stdin)
		// Exit less when fn exits.
		stdin.Close()
		exitCodeC <- exitCode
	}()

	err = cmd.Wait()
	cancel()

	exitCode, ok := exitcode.Get(err)
	if !ok {
		return done(err)
	}
	if exitCode != 0 {
		return exitCode
	}

	return <-exitCodeC
}
