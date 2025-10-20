// Copyright 2025 The LUCI Authors.
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

import { GenAiSuspect } from '@/proto/go.chromium.org/luci/bisection/proto/v1/genai.pb';

export function createMockGenAiSuspect(verfied: boolean) {
  return GenAiSuspect.fromPartial({
    commit: {
      host: 'not.a.real.host',
      project: 'chromium',
      id: 'ac52e3',
      ref: 'ref/main',
      position: 1,
    },
    reviewUrl: 'https://chromium-review.googlesource.com/placeholder/ac52e3',
    reviewTitle: '[MyApp] Added new functionality to improve my app',
    verified: verfied,
    verificationDetails: {
      status: 'Vindicated',
    },
    justification: 'this is a justification',
  });
}
