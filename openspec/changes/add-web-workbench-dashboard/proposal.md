# Change: Add Independent Web Workbench Dashboard

## Why
The project currently has Telegram bot + mobile Mini App flows, but lacks a desktop-oriented web workspace for multi-module monitoring and interaction.

## What Changes
- Add a new standalone `webapp/` frontend project (independent from `miniapp/`).
- Add a configurable workbench with four modules:
  - Hot Pools
  - GMGN K-line
  - Positions
  - Smart Money
- Add drag-and-drop widget reordering in the workbench.
- Add widget composition rules:
  - 4 widgets -> 2x2 layout
  - 3 widgets -> 3-column layout
  - 2 widgets -> vertical 2-row layout
  - 1 widget -> fullscreen layout
- Reuse existing backend APIs for data access.
- Add Telegram login on web:
  - user scans/logs in with Telegram from the top-right action
  - backend verifies Telegram auth signature
  - backend checks bot/miniapp access permission
  - backend issues WebApp-compatible initData and returns user profile info
- Move API base URL and bot username to file-based env configuration.
- Apply an OKX-inspired visual style with strong interaction feedback.

## Impact
- Affected specs: `web-workbench` (new capability)
- Affected code:
  - `webapp/*` (new project)
  - `openspec/changes/add-web-workbench-dashboard/*` (this proposal)
