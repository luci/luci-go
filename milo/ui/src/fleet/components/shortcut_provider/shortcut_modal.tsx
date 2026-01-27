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

import CloseIcon from '@mui/icons-material/Close';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { styled } from '@mui/material/styles';
import { useMemo, useState } from 'react';

import { ShortcutDefinition } from './context';
import { useShortcut } from './hook';
import { ShortcutSequence, formatShortcut } from './utils';

interface ParsedShortcut extends ShortcutDefinition {
  sequences: ShortcutSequence[];
}

const KeyBadge = styled('span')(({ theme }) => ({
  display: 'inline-block',
  padding: '2px 4px',
  minWidth: '20px',
  textAlign: 'center',
  borderRadius: '4px',
  backgroundColor: theme.palette.grey[100],
  border: `1px solid ${theme.palette.grey[300]}`,
  color: theme.palette.text.primary,
  fontFamily: 'Roboto Mono, monospace',
  fontSize: '0.75rem',
  fontWeight: 500,
  margin: '0 2px',
  boxShadow: '0 1px 0px rgba(0,0,0,0.1)',
  lineHeight: '1.2rem',
}));

const CategoryHeader = styled(Typography)(({ theme }) => ({
  marginTop: theme.spacing(2),
  marginBottom: theme.spacing(1),
  color: theme.palette.text.secondary,
  fontWeight: 500,
  // textTransform: 'uppercase',
  fontSize: '0.875rem',
}));

export const ShortcutModal = ({
  shortcuts,
}: {
  shortcuts: ParsedShortcut[];
}) => {
  const [isOpen, setIsOpen] = useState(false);

  useShortcut(
    'Show keyboard shortcuts',
    '?',
    () => {
      setIsOpen((prev) => !prev);
    },
    { category: 'Global' },
  );

  const groupedShortcuts = useMemo(() => {
    const groups: Record<string, ParsedShortcut[]> = {};
    shortcuts.forEach((s) => {
      const category = s.category || 'General';
      if (!groups[category]) {
        groups[category] = [];
      }
      groups[category].push(s);
    });
    return groups;
  }, [shortcuts]);

  const handleClose = () => setIsOpen(false);

  return (
    <Dialog
      open={isOpen}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        elevation: 0,
        variant: 'outlined',
        sx: { borderRadius: 2 },
      }}
    >
      <DialogTitle
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '24px 24px 8px',
        }}
      >
        <Typography variant="h4">Keyboard Shortcuts</Typography>
        <IconButton
          aria-label="close"
          onClick={handleClose}
          sx={{
            color: (theme) => theme.palette.grey[500],
            padding: 0,
          }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent sx={{ p: 0 }}>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))',
            gap: '24px',
            padding: '24px',
            paddingTop: 0,
          }}
        >
          {Object.entries(groupedShortcuts).map(([category, items]) => (
            <div key={category}>
              <CategoryHeader variant="subtitle2">{category}</CategoryHeader>
              <TableContainer
                component={Paper}
                elevation={0}
                variant="outlined"
              >
                <Table size="small">
                  <TableBody>
                    {items.map((s) => (
                      <TableRow
                        key={s.id}
                        sx={{
                          '&:last-child td, &:last-child th': { border: 0 },
                        }}
                      >
                        <TableCell
                          component="th"
                          scope="row"
                          sx={{ padding: '8px 16px' }}
                        >
                          <Typography variant="body2" color="text.primary">
                            {s.description}
                          </Typography>
                        </TableCell>
                        <TableCell
                          align="right"
                          sx={{
                            padding: '8px 16px',
                            display: 'flex',
                            flexDirection: 'column',
                            gap: '4px',
                          }}
                        >
                          {s.sequences?.map((seq, seqIdx) => (
                            <span key={seqIdx} style={{ whiteSpace: 'nowrap' }}>
                              {seqIdx > 0 && (
                                <Typography
                                  component="span"
                                  variant="caption"
                                  sx={{
                                    mx: 1,
                                    color: 'text.disabled',
                                  }}
                                >
                                  or
                                </Typography>
                              )}
                              {formatShortcut(seq).map((chordParts, i) => (
                                <span key={i}>
                                  {i > 0 && (
                                    <span
                                      style={{
                                        margin: '0 4px',
                                        color: '#999',
                                        fontSize: '0.75rem',
                                      }}
                                    >
                                      then
                                    </span>
                                  )}
                                  {chordParts.map((part, partIdx) => (
                                    <span key={partIdx}>
                                      {partIdx > 0 && (
                                        <span
                                          style={{
                                            margin: '0 2px',
                                            color: '#999',
                                          }}
                                        >
                                          +
                                        </span>
                                      )}
                                      <KeyBadge>{part}</KeyBadge>
                                    </span>
                                  ))}
                                </span>
                              ))}
                            </span>
                          ))}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
};
