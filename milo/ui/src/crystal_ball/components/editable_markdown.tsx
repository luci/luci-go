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

import { Edit } from '@mui/icons-material';
import {
  Box,
  Card,
  CardContent,
  IconButton,
  TextField,
  Typography,
} from '@mui/material';
import { useMemo, useState } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { renderMarkdown } from '@/common/tools/markdown/utils';

interface EditableMarkdownProps {
  /**
   * The initial markdown content to be rendered.
   */
  initialMarkdown: string;
  /**
   * Callback triggered when the content is saved (e.g. on blur).
   */
  onSave: (markdown: string) => void;
}

export function EditableMarkdown({
  initialMarkdown,
  onSave,
}: EditableMarkdownProps) {
  const [markdown, setMarkdown] = useState(initialMarkdown);
  const [isEditing, setIsEditing] = useState(false);

  const handleSave = () => {
    onSave(markdown);
    setIsEditing(false);
  };

  const renderedHtml = useMemo(() => renderMarkdown(markdown), [markdown]);

  return (
    <Card variant="outlined">
      <CardContent>
        {isEditing ? (
          <TextField
            multiline
            rows={4}
            fullWidth
            value={markdown}
            onChange={(e) => setMarkdown(e.target.value)}
            placeholder="Enter markdown content..."
            variant="outlined"
            onBlur={handleSave}
          />
        ) : (
          <Box sx={{ position: 'relative' }}>
            {markdown.trim() ? (
              <SanitizedHtml html={renderedHtml} />
            ) : (
              <Typography sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
                No content. Click to edit.
              </Typography>
            )}
            <IconButton
              size="small"
              onClick={() => setIsEditing(true)}
              sx={{ position: 'absolute', top: 0, right: 0 }}
              title="Edit"
            >
              <Edit fontSize="small" />
            </IconButton>
          </Box>
        )}
      </CardContent>
    </Card>
  );
}
