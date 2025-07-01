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

import CancelPresentationIcon from '@mui/icons-material/CancelPresentation';
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import InfoIcon from '@mui/icons-material/Info';
import RepeatOnIcon from '@mui/icons-material/RepeatOn';
import SkipNextIcon from '@mui/icons-material/SkipNext';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';

import { Invocation_State } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

// Define the semantic status types available in the system.
// This is a mix of various different types of status, e.g. invoaction, verdict, result and UI errors.
// TODO: remove any unneeded entries from this list .
export type SemanticStatusType =
  | 'success'
  | 'passed'
  | 'failed'
  | 'error'
  | 'warning'
  | 'info'
  | 'neutral'
  | 'accent'
  | 'flaky'
  | 'skipped'
  | 'unexpectedly_skipped'
  | 'exonerated'
  | 'expected'
  | 'scheduled'
  | 'started'
  | 'infra_failure'
  | 'canceled'
  | 'execution_errored'
  | 'precluded'
  | 'unknown';

export interface StatusStyle {
  icon?: React.ElementType;
  textColor: string; // For text on neutral surfaces (e.g. white/gray page background).
  backgroundColor: string;
  borderColor?: string;
  iconColor?: string; // For standalone icons or icons on neutral surfaces.
  onBackgroundColor: string; // High-contrast text color for use ON the container background.
}

/**
 * Retrieves a style object for a given semantic status.
 * Colors are CSS variable strings (e.g., 'var(--gm3-color-success)')
 * which are defined in global CSS (e.g., style.css).
 */
export function getStatusStyle(statusType: SemanticStatusType): StatusStyle {
  switch (statusType) {
    case 'success':
    case 'expected':
    case 'passed':
      return {
        icon: CheckCircleOutlineIcon,
        textColor: 'var(--gm3-color-success, var(--success-color))',
        backgroundColor:
          'var(--gm3-color-success-container, var(--success-bg-color))',
        borderColor: 'var(--gm3-color-success-outline, var(--success-color))',
        iconColor: 'var(--gm3-color-success, var(--success-color))',
        onBackgroundColor:
          'var(--gm3-color-on-success-container, var(--success-color))',
      };
    case 'error':
    case 'failed':
      return {
        icon: ErrorOutlineIcon,
        textColor: 'var(--gm3-color-error, var(--failure-color))',
        backgroundColor:
          'var(--gm3-color-error-container, var(--failure-bg-color))',
        borderColor: 'var(--gm3-color-error-outline, var(--failure-color))',
        iconColor: 'var(--gm3-color-error, var(--failure-color))',
        onBackgroundColor:
          'var(--gm3-color-on-error-container, var(--failure-color))',
      };
    case 'infra_failure':
    case 'unexpectedly_skipped':
    case 'execution_errored':
      return {
        icon: ErrorOutlineIcon,
        textColor: 'var(--critical-failure-color)',
        backgroundColor: 'var(--critical-failure-bg-color)',
        borderColor: 'var(--critical-failure-color)',
        iconColor: 'var(--critical-failure-color)',
        onBackgroundColor: 'var(--critical-failure-color)',
      };
    case 'warning':
      return {
        icon: WarningAmberIcon,
        textColor: 'var(--gm3-color-warning, var(--warning-color))',
        backgroundColor:
          'var(--gm3-color-warning-container, var(--warning-bg-color))',
        borderColor: 'var(--gm3-color-warning-outline, var(--warning-color))',
        iconColor: 'var(--gm3-color-warning, var(--warning-color))',
        onBackgroundColor:
          'var(--gm3-color-on-warning-container, var(--warning-color))',
      };
    case 'flaky':
      return {
        icon: RepeatOnIcon,
        textColor: 'var(--gm3-color-warning, var(--warning-color))',
        backgroundColor:
          'var(--gm3-color-warning-container, var(--warning-bg-color))',
        borderColor: 'var(--gm3-color-warning-outline, var(--warning-color))',
        iconColor: 'var(--gm3-color-warning, var(--warning-color))',
        onBackgroundColor:
          'var(--gm3-color-on-warning-container, var(--warning-color))',
      };
    case 'info':
      return {
        icon: InfoIcon,
        textColor: 'var(--gm3-color-info, var(--light-text-color))',
        backgroundColor: 'var(--gm3-color-info-container, #e8f0fe)',
        borderColor: 'var(--gm3-color-info-outline, var(--active-text-color))',
        iconColor: 'var(--gm3-color-info, var(--active-text-color))',
        onBackgroundColor:
          'var(--gm3-color-on-info-container, var(--light-text-color))',
      };
    case 'accent':
      return {
        icon: InfoIcon,
        textColor: 'var(--gm3-color-primary, var(--active-text-color))',
        backgroundColor:
          'var(--gm3-color-primary-container, var(--light-active-color))',
        borderColor:
          'var(--gm3-color-primary-outline, var(--active-text-color))',
        iconColor: 'var(--gm3-color-primary, var(--active-text-color))',
        onBackgroundColor:
          'var(--gm3-color-on-primary-container, var(--active-text-color))',
      };
    case 'exonerated':
      return {
        icon: CheckCircleOutlineIcon,
        textColor: 'var(--exonerated-color)',
        backgroundColor: 'var(--gm3-color-surface-container-low, #f0f4f8)',
        borderColor: 'var(--exonerated-color)',
        iconColor: 'var(--exonerated-color)',
        onBackgroundColor: 'var(--exonerated-color)',
      };
    case 'skipped':
      return {
        icon: SkipNextIcon,
        textColor:
          'var(--gm3-color-on-surface-variant, var(--greyed-out-text-color))',
        backgroundColor:
          'var(--gm3-color-surface-container-low, var(--canceled-bg-color))',
        borderColor: 'var(--gm3-color-outline-variant, var(--canceled-color))',
        iconColor:
          'var(--gm3-color-on-surface-variant, var(--greyed-out-text-color))',
        onBackgroundColor:
          'var(--gm3-color-on-surface-variant, var(--greyed-out-text-color))',
      };
    case 'canceled':
    case 'precluded':
      return {
        icon: CancelPresentationIcon,
        textColor: 'var(--canceled-color, var(--exonerated-color))',
        backgroundColor:
          'var(--canceled-bg-color, var(--gm3-color-surface-container-low))',
        borderColor: 'var(--canceled-color, var(--exonerated-color))',
        iconColor: 'var(--canceled-color, var(--exonerated-color))',
        onBackgroundColor: 'var(--canceled-color, var(--exonerated-color))',
      };
    case 'scheduled':
      return {
        // Icon might not be typical for 'scheduled' in all contexts
        icon: HelpOutlineIcon,
        textColor: 'var(--scheduled-color)',
        backgroundColor: 'var(--scheduled-bg-color)',
        borderColor: 'var(--scheduled-color)',
        iconColor: 'var(--scheduled-color)',
        onBackgroundColor:
          'var(--gm3-color-on-surface-medium, var(--default-text-color))',
      };
    case 'started':
      return {
        // Icon might not be typical for 'started' in all contexts
        icon: HelpOutlineIcon,
        textColor: 'var(--started-color)',
        backgroundColor: 'var(--started-bg-color)',
        borderColor: 'var(--started-color)',
        iconColor: 'var(--started-color)',
        onBackgroundColor:
          'var(--gm3-color-on-surface-medium, var(--default-text-color))',
      };
    case 'neutral':
    case 'unknown':
    default:
      return {
        icon: HelpOutlineIcon,
        textColor:
          'var(--gm3-color-on-surface-medium, var(--default-text-color))',
        backgroundColor:
          'var(--gm3-color-surface-container-low, var(--block-background-color))',
        borderColor: 'var(--gm3-color-outline-variant, var(--divider-color))',
        iconColor:
          'var(--gm3-color-on-surface-variant, var(--light-text-color))',
        onBackgroundColor:
          'var(--gm3-color-on-surface-medium, var(--default-text-color))',
      };
  }
}

export function semanticStatusForTestVariant(
  variant: TestVariant,
): SemanticStatusType {
  if (variant.statusOverride === TestVerdict_StatusOverride.EXONERATED) {
    return 'exonerated';
  }
  switch (variant.statusV2) {
    case TestVerdict_Status.FAILED:
      return 'failed';
    case TestVerdict_Status.FLAKY:
      return 'flaky';
    case TestVerdict_Status.EXECUTION_ERRORED:
      return 'execution_errored';
    case TestVerdict_Status.PASSED:
      return 'passed';
    case TestVerdict_Status.SKIPPED:
      return 'skipped';
    case TestVerdict_Status.PRECLUDED:
      return 'precluded';
    case TestVerdict_Status.STATUS_UNSPECIFIED:
    default:
      break;
  }
  return 'unknown';
}

export function semanticStatusForInvocationState(
  invocationState: Invocation_State,
): SemanticStatusType {
  switch (invocationState) {
    case Invocation_State.ACTIVE:
      return 'info';
    case Invocation_State.FINALIZING:
      return 'warning';
    case Invocation_State.FINALIZED:
      return 'success';
    case Invocation_State.STATE_UNSPECIFIED:
    default:
      break;
  }
  return 'unknown';
}
