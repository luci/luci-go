// Copyright 2023 The LUCI Authors.
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

import {
  HeuristicSuspect,
  SuspectConfidenceLevel,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/heuristic.pb';

export function createMockHeuristicSuspect(commitID: string) {
  return HeuristicSuspect.fromPartial({
    gitilesCommit: {
      host: 'not.a.real.host',
      project: 'chromium',
      id: commitID,
      ref: 'ref/main',
      position: 1,
    },
    reviewUrl: `https://chromium-review.googlesource.com/placeholder/+${commitID}`,
    reviewTitle: '[MyApp] Added new functionality to improve my app',
    score: 15,
    justification:
      'The file "dir/a/b/x.cc" was added and it was in the failure log.\n' +
      'The file "content/util.c" was modified. It was related to the file obj/' +
      'content/util.o which was in the failure log.',
    confidenceLevel: SuspectConfidenceLevel.HIGH,
    verificationDetails: {
      status: 'Vindicated',
    },
  });
}
