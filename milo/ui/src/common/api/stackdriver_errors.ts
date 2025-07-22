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

import StackdriverErrorReporter from 'stackdriver-errors-js';

export const errorHandler = new StackdriverErrorReporter();
if (
  ['luci-milo.appspot.com', 'ci.chromium.org'].includes(
    window.location.hostname,
  )
) {
  errorHandler.start({
    key: 'AIzaSyDxVV8kLK8CozsA1iKiPx6OjukSKQKmVbY',
    projectId: 'luci-milo',
  });
} else {
  errorHandler.start({
    key: 'AIzaSyAyY1lwrHvFsIUrxyTuUDZZF1xTF6GbY08',
    projectId: 'luci-milo-dev',
  });
}
