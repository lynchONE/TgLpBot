## ADDED Requirements

### Requirement: Independent Web Workbench Project
The system SHALL provide an independent web frontend project for desktop usage, separate from the Telegram Mini App project.

#### Scenario: Run web workbench independently
- **WHEN** the user starts the web workbench project locally
- **THEN** the project runs without requiring the Mini App runtime
- **AND** it can call existing backend HTTP APIs directly

### Requirement: Configurable Multi-Widget Workbench
The web workbench SHALL allow users to enable or disable four modules: Hot Pools, GMGN K-line, Positions, and Smart Money.

#### Scenario: Toggle module visibility
- **WHEN** the user toggles module chips in the workbench controls
- **THEN** the selected modules appear in the workspace
- **AND** unselected modules are hidden

### Requirement: Drag-and-Drop Reordering
The web workbench SHALL support drag-and-drop reordering for currently visible modules.

#### Scenario: Reorder visible modules
- **WHEN** the user drags a visible module and drops it on another visible module
- **THEN** the module order updates
- **AND** the workspace renders modules in the new order

### Requirement: Deterministic Layout Rules
The workbench SHALL use deterministic layouts based on selected widget count.

#### Scenario: Four modules selected
- **WHEN** the user selects exactly 4 modules
- **THEN** the workspace renders as 2 rows and 2 columns

#### Scenario: Three modules selected
- **WHEN** the user selects exactly 3 modules
- **THEN** the workspace renders as left-center-right columns

#### Scenario: Two modules selected
- **WHEN** the user selects exactly 2 modules
- **THEN** the workspace renders as a top-and-bottom vertical split

#### Scenario: One module selected
- **WHEN** the user selects exactly 1 module
- **THEN** the workspace renders that module in fullscreen

### Requirement: Integrated Market and Account Views
The workbench SHALL show data in each module using existing backend endpoints.

#### Scenario: Load module data
- **WHEN** valid initData and API base URL are provided
- **THEN** Hot Pools reads from `hot_pools`
- **AND** GMGN K-line reads from `pool_ohlcv`
- **AND** Positions reads from `realtime_positions`
- **AND** Smart Money reads from `smart_money`

### Requirement: Telegram Web Login and Permission Gate
The web workbench SHALL provide Telegram login in the top-right area and gate data access by backend permission checks.

#### Scenario: Login success
- **WHEN** a user logs in through Telegram web login
- **THEN** backend verifies Telegram auth signature
- **AND** backend checks bot/miniapp access permission
- **AND** backend returns web-usable initData plus user profile info

#### Scenario: Trigger login from Telegram icon
- **WHEN** the user clicks the Telegram icon in the top-right area
- **THEN** the web app opens Telegram login QR/auth flow
- **AND** login callback is exchanged with backend for permission-gated initData

#### Scenario: Login denied
- **WHEN** backend permission check fails
- **THEN** login is rejected
- **AND** protected data modules remain unavailable

### Requirement: File-Based Runtime Configuration
The web workbench SHALL read API base URL and Telegram bot username from file-based environment configuration.

#### Scenario: Start with env config
- **WHEN** the web app boots
- **THEN** it reads API base URL from env configuration
- **AND** it reads Telegram bot username from env configuration
- **AND** these values are not required as in-page input fields

### Requirement: OKX-Inspired Visual and Interaction Style
The web workbench SHALL provide a concise, dark high-contrast visual style with clear interactive feedback.

#### Scenario: Interact with cards and controls
- **WHEN** the user hovers, selects, or refreshes modules
- **THEN** the interface provides clear visual transitions and highlighted state feedback
