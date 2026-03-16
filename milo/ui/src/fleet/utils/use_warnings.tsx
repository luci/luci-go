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

import { Alert } from '@mui/material';
import { useCallback, useState } from 'react';

export const useWarnings = () => {
  const [warnings, setWarnings] = useState<Record<string, NodeJS.Timeout>>({});

  const addWarning = useCallback((newWarning: string) => {
    setWarnings((warnings) => {
      if (newWarning in warnings) {
        warnings[newWarning]?.refresh?.(); // care for race conditions
        return warnings;
      }

      const t = setTimeout(
        () =>
          setWarnings((warnings) => {
            delete warnings[newWarning];
            return { ...warnings };
          }),
        5_000,
      );
      return { ...warnings, [newWarning]: t };
    });
  }, []);

  return [Object.keys(warnings), addWarning] as const;
};

export const WarningNotifications = ({ warnings }: { warnings: string[] }) => (
  <>
    {warnings.map((message, i) => (
      <Alert
        key={message}
        sx={{
          position: 'fixed',
          top: 64 + 10 + 55 * i,
          right: 10,
          zIndex: 10_000,
        }}
        severity="warning"
      >
        {message}
      </Alert>
    ))}
  </>
);
