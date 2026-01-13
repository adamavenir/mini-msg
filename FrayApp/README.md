# Fray macOS Client

Native SwiftUI client for Fray, using a Go FFI bridge to communicate with the fray data layer.

## Prerequisites

- macOS 14.0+
- Xcode 15+ (tested with Xcode 16)
- Go 1.21+
- xcodegen (`brew install xcodegen`)

## Quick Start

```bash
# 1. Build the Go FFI library
cd /path/to/fray
CGO_ENABLED=1 go build -buildmode=c-shared -o build/libfray.dylib ./cmd/libfray

# 2. Generate the Xcode project
cd FrayApp
xcodegen

# 3. Open and build
open Fray.xcodeproj
# Build with ⌘B, Run with ⌘R
```

## Project Structure

```
FrayApp/
├── Fray/                    # Main app target
│   ├── FrayApp.swift        # App entry point
│   ├── ContentView.swift    # Main view with NavigationSplitView
│   ├── FrayBridge/          # Go FFI bridge
│   │   ├── FrayBridge.swift # Swift wrapper for C functions
│   │   ├── FrayTypes.swift  # Data models matching Go types
│   │   ├── FrayError.swift  # Error handling
│   │   ├── libfray.h        # C header from cgo
│   │   └── module.modulemap # Swift module definition
│   ├── Models/              # ViewModels
│   │   ├── MessagesViewModel.swift
│   │   ├── ThreadsViewModel.swift
│   │   ├── AgentsViewModel.swift
│   │   └── AppState.swift
│   ├── Views/               # SwiftUI views
│   │   ├── SidebarView.swift
│   │   ├── MessageListView.swift
│   │   ├── MessageBubble.swift
│   │   ├── MessageInputArea.swift
│   │   ├── ActivityPanelView.swift
│   │   ├── AgentActivityRow.swift
│   │   ├── CommandPalette.swift
│   │   ├── ThreadBreadcrumb.swift
│   │   ├── KeyboardNavigation.swift
│   │   └── CodeBlockView.swift
│   ├── Styles/              # Design system
│   │   ├── FrayColors.swift
│   │   ├── FraySpacing.swift
│   │   ├── FrayTypography.swift
│   │   └── FrayAccessibility.swift
│   ├── Services/            # Platform services
│   │   ├── NotificationService.swift
│   │   ├── WindowStateService.swift
│   │   └── AccessibilityService.swift
│   └── Fray.entitlements    # App entitlements
├── FrayMenuBar/             # Menu bar app target
│   ├── FrayMenuBarApp.swift
│   └── MenuBarContentView.swift
├── project.yml              # xcodegen project definition
├── Fray.xcodeproj/          # Generated Xcode project
└── XCODE_SETUP.md           # Manual setup instructions
```

## Architecture

### FFI Bridge

The app communicates with Fray's Go backend via a C-shared library:

1. `cmd/libfray/main.go` exports C functions using cgo
2. `libfray.dylib` is built and linked into the app
3. `FrayBridge.swift` wraps C calls in a Swift-friendly API
4. `FrayTypes.swift` defines Swift structs matching Go types

### Data Flow

```
SwiftUI View
    ↓
ViewModel (@Observable)
    ↓
FrayBridge (Swift)
    ↓
libfray.dylib (Go → C)
    ↓
JSONL files / SQLite cache
```

## Features

### Main App
- **Sidebar**: Thread list with hierarchy, agent avatars
- **Message List**: Messages with markdown, reactions, threading
- **Activity Panel**: Agent presence with real-time updates
- **Command Palette**: ⌘K for quick navigation
- **Keyboard Shortcuts**: ⌘0 sidebar, ⌘I activity, ⌘K palette

### Menu Bar App
- Unread count badge
- Active agents list
- Recent messages preview
- Quick compose

## Configuration

### Build Settings (project.yml)

Key settings in the xcodegen project:

```yaml
LIBRARY_SEARCH_PATHS: $(PROJECT_DIR)/../build
HEADER_SEARCH_PATHS: $(PROJECT_DIR)/Fray/FrayBridge
OTHER_LDFLAGS: -lfray
OTHER_SWIFT_FLAGS: -import-objc-header $(PROJECT_DIR)/Fray/FrayBridge/libfray.h
ENABLE_APP_SANDBOX: NO  # Required for filesystem access
```

### Pre-build Script

The project includes a pre-build script to compile `libfray.dylib`. If this fails:

1. Ensure Go is in your PATH
2. Or build manually: `CGO_ENABLED=1 go build -buildmode=c-shared -o build/libfray.dylib ./cmd/libfray`

## Troubleshooting

### "Cannot find 'FrayOpenDatabase' in scope"
- Check that `libfray.h` exists in `Fray/FrayBridge/`
- Verify `OTHER_SWIFT_FLAGS` includes the import-objc-header flag

### "Library not loaded: @rpath/libfray.dylib"
- Ensure the dylib is built in `../build/`
- Check that Copy Files phase copies to Frameworks

### Build script fails
- Go may not be in Xcode's PATH
- Solution: Build dylib manually before Xcode build

### Swift API errors (onKeyPress, etc.)
- These may occur with newer macOS SDKs
- Check `ContentView.swift` and `KeyboardNavigation.swift` for compatibility

## Development

### Regenerating the Project

After modifying `project.yml`:

```bash
cd FrayApp
xcodegen
```

### Updating FFI Types

When Go types change:
1. Update `FrayTypes.swift` to match
2. Update `FrayBridge.swift` if function signatures change
3. Rebuild `libfray.dylib`

### Adding New Views

1. Create SwiftUI view in `Views/`
2. Use design tokens from `FrayColors`, `FraySpacing`, `FrayTypography`
3. Add accessibility modifiers from `FrayAccessibility`

## License

Part of the Fray project.
