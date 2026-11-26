# 10-Second Bass (404 Easter Egg)

Self-contained, lazy-loaded 10-second repair speedrun mini-game triggered from
the Fleet Console 404 page by clicking the sad bass mascot or pressing `Space`.

- **Zero runtime/bundle cost** until triggered (`React.lazy` in `not_found_page.tsx`).
- **Zero backend/RPC coupling** (pure Canvas 2D + Web Audio + `localStorage`).
- Safe to delete this directory if it ever causes maintenance friction.
