// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"fmt"
	"os/exec"
)

func execCommand(program string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command(program, args...)
	inwriter, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdin pipe: %s", err)
	}
	go func() {
		defer inwriter.Close()
		_, _ = inwriter.Write(stdin)
	}()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("failed to run %s: %s (output: %s)", program, err, out)
	}
	return out, nil
}

func RunPython(stdin []byte, args ...string) ([]byte, error) {
	return execCommand("python", stdin, args...)
}
