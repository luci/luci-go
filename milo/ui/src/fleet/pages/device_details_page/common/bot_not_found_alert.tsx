// Copyright 2026 The LUCI Authors.
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

import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';

interface BotNotFoundAlertProps {
  dutId?: string;
}

export const BotNotFoundAlert = ({ dutId }: BotNotFoundAlertProps) => {
  return (
    <AlertWithFeedback
      severity="warning"
      title="Bot not found!"
      bugErrorMessage={`Bot not found for device: ${dutId}`}
    >
      <p>
        Oh no! No bots were found for this device <code>dut_id={dutId}</code>.
      </p>
    </AlertWithFeedback>
  );
};
