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

package port

import (
	"fmt"
	"math/rand"
	"sync"
	"syscall"
	"time"
)

const (
	// Range of port numbers to choose.
	minPort = 32768
	maxPort = 60000

	// Maximum number of retries to find a free port.
	maxRetries = 256
)

// A private random number generator to avoid messing with any program determinism.
var rng = struct {
	sync.Mutex
	*rand.Rand
}{Rand: rand.New(rand.NewSource(time.Now().UnixNano() + int64(syscall.Getpid())))}

// PickUnusedPort returns a port number that is not currently bound.
// There is an inherent race condition between this function being called and
// any other process on the same computer, so the caller should bind to the
// port as soon as possible.
func PickUnusedPort() (port int, err error) {
	// Start with random port in range [32768, 60000]
	rng.Lock()
	port = minPort + rng.Intn(maxPort-minPort+1)
	rng.Unlock()

	// Check if the port is free, if not look for another one.
	tries := 0
	for tries < maxRetries {
		if isPortFree(port) {
			return port, nil
		}

		port += rng.Intn(100)
		if port > maxPort {
			port = minPort + port%maxPort
		}
		tries++
	}

	return 0, fmt.Errorf("no unused port")
}

func isPortFree(port int) bool {
	return isPortTypeFree(port, syscall.SOCK_STREAM) &&
		isPortTypeFree(port, syscall.SOCK_DGRAM)
}

func isPortTypeFree(port, typ int) bool {
	// For the port to be considered available, the kernel must support at
	// least one of (IPv6, IPv4), and the port must be available on each
	// supported family.
	var probes = []struct {
		family int
		addr   syscall.Sockaddr
	}{
		{syscall.AF_INET6, &syscall.SockaddrInet6{Port: port}},
		{syscall.AF_INET, &syscall.SockaddrInet4{Port: port}},
	}
	gotSocket := false
	for _, probe := range probes {
		// We assume that Socket will succeed iff the kernel supports this
		// address family.
		fd, err := syscall.Socket(probe.family, typ, 0)
		if err != nil {
			continue
		}
		// Now that we have a socket, any subsequent error means the port is unavailable.
		gotSocket = true
		err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		if err != nil {
			syscall.Close(fd)
			return false
		}
		err = syscall.Bind(fd, probe.addr)
		syscall.Close(fd)
		if err != nil {
			return false
		}
	}
	return gotSocket
}
