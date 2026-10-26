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

import { useCallback, useEffect, useMemo } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

export const REPAIR_LABS = ['MTV', 'HTL'] as const;
export type RepairLab = (typeof REPAIR_LABS)[number];
export const DEFAULT_REPAIR_LAB: RepairLab = 'MTV';
export const REPAIR_LAB_SEARCH_PARAM = 'lab';
export const REPAIR_LAB_LOCAL_STORAGE_KEY = 'fleet-console:selected-repair-lab';

export function isRepairLab(val: string): val is RepairLab {
  return (REPAIR_LABS as readonly string[]).includes(val);
}

export function getStoredRepairLab(): RepairLab {
  try {
    const stored = localStorage.getItem(REPAIR_LAB_LOCAL_STORAGE_KEY);
    if (stored && isRepairLab(stored)) {
      return stored;
    }
  } catch {
    // Ignore localStorage errors (e.g. storage disabled).
  }
  return DEFAULT_REPAIR_LAB;
}

export const useRepairLab = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const paramLab = searchParams.get(REPAIR_LAB_SEARCH_PARAM);
  const lab: RepairLab = useMemo(() => {
    if (paramLab && isRepairLab(paramLab)) {
      return paramLab;
    }
    return getStoredRepairLab();
  }, [paramLab]);

  useEffect(() => {
    try {
      localStorage.setItem(REPAIR_LAB_LOCAL_STORAGE_KEY, lab);
    } catch {
      // Ignore localStorage errors.
    }

    if (paramLab !== lab) {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set(REPAIR_LAB_SEARCH_PARAM, lab);
          return next;
        },
        { replace: true },
      );
    }
  }, [lab, paramLab, setSearchParams]);

  const setLab = useCallback(
    (newLab: RepairLab) => {
      try {
        localStorage.setItem(REPAIR_LAB_LOCAL_STORAGE_KEY, newLab);
      } catch {
        // Ignore localStorage errors.
      }
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        next.set(REPAIR_LAB_SEARCH_PARAM, newLab);
        next.delete('cursor');
        return next;
      });
    },
    [setSearchParams],
  );

  return { lab, setLab };
};
