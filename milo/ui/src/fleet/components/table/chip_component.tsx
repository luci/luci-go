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

import Chip from '@mui/material/Chip';

import { useSettings } from '@/fleet/hooks/use_settings';
import { colors } from '@/fleet/theme/colors';

export interface ChipComponentProps {
  label: string;
  color?: string;
  url?: string;
  openInNewTab?: boolean;
  onClick?: () => void;
}

export const ChipComponent = ({
  label,
  color,
  url,
  openInNewTab = true,
  onClick,
}: ChipComponentProps) => {
  const [settings] = useSettings();
  const density = settings?.table?.density;

  const variant =
    !color || color === colors.transparent ? 'outlined' : 'filled';

  return (
    <Chip
      label={label}
      variant={variant}
      size={density === 'compact' ? 'small' : 'medium'}
      sx={{
        backgroundColor: color,
        width: 'fit-content',
        fontWeight: 500,
      }}
      href={url}
      target={url ? (openInNewTab ? '_blank' : '_self') : undefined}
      rel={url && openInNewTab ? 'noopener noreferrer' : undefined}
      component={url ? 'a' : 'div'}
      clickable={Boolean(url || onClick)}
      onClick={onClick}
    />
  );
};
