// src/common/styles/status_styles.ts
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

import CancelPresentationIcon from '@mui/icons-material/CancelPresentation'; // For Aborted
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import InfoIcon from '@mui/icons-material/Info';
import RepeatOnIcon from '@mui/icons-material/RepeatOn'; // For Flaky
import SkipNextIcon from '@mui/icons-material/SkipNext'; // For Skipped
import WarningAmberIcon from '@mui/icons-material/WarningAmber';

// Define the semantic status types available in the system
export type SemanticStatusType =
  | 'success'
  | 'failure' // Maps to --failure-color or --gm3-color-error
  | 'error' // Explicitly for errors, likely same as failure
  | 'warning'
  | 'info'
  | 'neutral'
  | 'accent'
  | 'flaky'
  | 'skipped'
  | 'unexpectedly_skipped'
  | 'exonerated'
  | 'expected' // Usually maps to success
  | 'scheduled'
  | 'started'
  | 'infra_failure' // Maps to --critical-failure-color
  | 'canceled'
  | 'unknown';

// Define the structure for a complete style set for a status
export interface StatusStyle {
  icon?: React.ElementType;
  textColor: string; // CSS variable string e.g., 'var(--gm3-color-error)'
  backgroundColor: string; // CSS variable string e.g., 'var(--gm3-color-error-container)'
  borderColor?: string; // Optional: CSS variable string
  // For specific cases like an icon on its own colored background chip
  iconColor?: string; // Can be different from textColor, e.g., a darker shade for better contrast on a light background
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
      return {
        icon: CheckCircleOutlineIcon,
        textColor: 'var(--gm3-color-success, var(--success-color))', // Fallback to older variable
        backgroundColor:
          'var(--gm3-color-success-container, var(--success-bg-color))',
        borderColor: 'var(--gm3-color-success, var(--success-color))',
        iconColor: 'var(--gm3-color-success, var(--success-color))', // Icon color on its container
      };
    case 'error':
    case 'failure':
      return {
        icon: ErrorOutlineIcon,
        textColor: 'var(--gm3-color-error, var(--failure-color))',
        backgroundColor:
          'var(--gm3-color-error-container, var(--failure-bg-color))',
        borderColor: 'var(--gm3-color-error, var(--failure-color))',
        iconColor: 'var(--gm3-color-error, var(--failure-color))',
      };
    case 'infra_failure':
    case 'unexpectedly_skipped': // As per color_classes.css, this uses critical failure
      return {
        icon: ErrorOutlineIcon,
        textColor: 'var(--critical-failure-color)',
        backgroundColor: 'var(--critical-failure-bg-color)',
        borderColor: 'var(--critical-failure-color)',
        iconColor: 'var(--critical-failure-color)',
      };
    case 'warning':
      return {
        icon: WarningAmberIcon,
        textColor: 'var(--gm3-color-warning-dark, var(--warning-color))', // Prefer darker for text on light bg
        backgroundColor:
          'var(--gm3-color-warning-container, var(--warning-bg-color))',
        borderColor: 'var(--gm3-color-warning-dark, var(--warning-color))',
        iconColor: 'var(--gm3-color-warning-dark, var(--warning-color))',
      };
    case 'flaky':
      return {
        icon: RepeatOnIcon,
        textColor: 'var(--warning-text-color, var(--gm3-color-warning-dark))',
        backgroundColor:
          'var(--warning-bg-color, var(--gm3-color-warning-container))',
        borderColor: 'var(--warning-color, var(--gm3-color-warning-dark))',
        iconColor: 'var(--warning-text-color, var(--gm3-color-warning-dark))',
      };
    case 'info':
      return {
        icon: InfoIcon,
        textColor:
          'var(--gm3-color-on-surface-medium, var(--light-text-color))',
        backgroundColor: 'var(--gm3-color-surface-container, #e8f0fe)',
        borderColor: 'var(--gm3-color-primary, var(--active-text-color))',
        iconColor: 'var(--gm3-color-primary, var(--active-text-color))',
      };
    case 'accent':
      return {
        // Icon might not always be needed for 'accent' style, depends on usage
        icon: InfoIcon,
        textColor: 'var(--gm3-color-primary, var(--active-text-color))',
        backgroundColor:
          'var(--gm3-color-primary-container, var(--light-active-color))',
        borderColor: 'var(--gm3-color-primary, var(--active-text-color))',
        iconColor: 'var(--gm3-color-primary, var(--active-text-color))',
      };
    case 'exonerated':
      return {
        icon: CheckCircleOutlineIcon,
        textColor: 'var(--exonerated-color)',
        backgroundColor: 'var(--gm3-color-surface-container-low, #f0f4f8)', // A muted background
        borderColor: 'var(--exonerated-color)',
        iconColor: 'var(--exonerated-color)',
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
      };
    case 'canceled':
      return {
        icon: CancelPresentationIcon,
        textColor: 'var(--canceled-color, var(--exonerated-color))',
        backgroundColor:
          'var(--canceled-bg-color, var(--gm3-color-surface-container-low))',
        borderColor: 'var(--canceled-color, var(--exonerated-color))',
        iconColor: 'var(--canceled-color, var(--exonerated-color))',
      };
    case 'scheduled':
      return {
        // Icon might not be typical for 'scheduled' in all contexts
        icon: HelpOutlineIcon,
        textColor: 'var(--scheduled-color)',
        backgroundColor: 'var(--scheduled-bg-color)',
        borderColor: 'var(--scheduled-color)',
        iconColor: 'var(--scheduled-color)',
      };
    case 'started':
      return {
        // Icon might not be typical for 'started' in all contexts
        icon: HelpOutlineIcon,
        textColor: 'var(--started-color)',
        backgroundColor: 'var(--started-bg-color)',
        borderColor: 'var(--started-color)',
        iconColor: 'var(--started-color)',
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
      };
  }
}
