// Copyright 2018 The LUCI Authors.
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

package projectscope

import (
	"fmt"
	"sync"
)

type GcpProjectResolver interface {
	Resolve(service string) (string, error)
	Add(service string, gcpProject string)
	Remove(service string)
}

type gcpProjectResolver struct {
	m    map[string]string
	lock sync.RWMutex
}

func (s *gcpProjectResolver) Resolve(service string) (string, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	if project, ok := s.m[service]; ok {
		return project, nil
	}
	return "", fmt.Errorf("key not found: %s", service)
}

func (s *gcpProjectResolver) Add(service string, gcpProject string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.m[service] = gcpProject
}

func (s *gcpProjectResolver) Remove(service string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.m, service)
}

var DefaultGcpProjectResolver = &gcpProjectResolver{}
