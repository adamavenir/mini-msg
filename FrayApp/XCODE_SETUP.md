# Xcode Project Setup Guide

This guide walks through creating the Xcode project for FrayApp.

## Prerequisites

1. Xcode 16+ installed
2. Go installed (for building libfray.dylib)
3. macOS 26 SDK

## Create Xcode Project

1. Open Xcode
2. File → New → Project
3. Choose "macOS" → "App"
4. Configure:
   - Product Name: `Fray`
   - Team: Your development team
   - Organization Identifier: `com.fray`
   - Interface: SwiftUI
   - Language: Swift
   - Storage: None
5. Save in `FrayApp/` directory (will create `Fray.xcodeproj`)

## Project Structure

After creation, organize files:

```
FrayApp/
├── Fray.xcodeproj
├── Fray/
│   ├── FrayApp.swift          # Already created
│   ├── ContentView.swift      # Already created
│   ├── FrayBridge/           # Already created
│   │   ├── FrayBridge.swift
│   │   ├── FrayTypes.swift
│   │   ├── FrayError.swift
│   │   ├── libfray.h
│   │   └── module.modulemap
│   ├── Models/
│   ├── Views/
│   ├── Styles/
│   └── Services/
├── FrayTests/
├── Scripts/
│   └── build-dylib.sh
└── Resources/
```

## Build Settings

### Deployment Target
- Set "macOS Deployment Target" to 26.0

### Swift Version
- Set "Swift Language Version" to 6.0

### Library Search Paths
Add to "Library Search Paths":
```
$(PROJECT_DIR)/../build
```

### Header Search Paths
Add to "Header Search Paths":
```
$(PROJECT_DIR)/Fray/FrayBridge
```

### Other Linker Flags
Add to "Other Linker Flags":
```
-lfray
```

### Swift Compiler - Custom Flags
Add to "Other Swift Flags":
```
-import-objc-header $(PROJECT_DIR)/Fray/FrayBridge/libfray.h
```

## Build Phases

### 1. Run Script - Build libfray.dylib

Add a "Run Script" phase BEFORE "Compile Sources":

```bash
"${PROJECT_DIR}/Scripts/build-dylib.sh"
```

Input Files:
```
$(PROJECT_DIR)/../cmd/libfray/main.go
```

Output Files:
```
$(PROJECT_DIR)/../build/libfray.dylib
```

### 2. Copy Files - Bundle dylib

Add a "Copy Files" phase:
- Destination: Frameworks
- Files: `../build/libfray.dylib`

## Signing & Capabilities

### Code Signing
- Enable "Automatically manage signing"
- Select your Development Team

### Entitlements
Create `Fray.entitlements`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.app-sandbox</key>
    <false/>
    <key>com.apple.security.files.user-selected.read-write</key>
    <true/>
</dict>
</plist>
```

Note: App Sandbox is disabled because we need filesystem access to read `.fray/` directories.

## Info.plist Additions

Add to Info.plist:
```xml
<key>CFBundleDisplayName</key>
<string>Fray</string>
<key>LSApplicationCategoryType</key>
<string>public.app-category.developer-tools</string>
```

## Build and Run

1. Select "My Mac" as run destination
2. Build (Cmd+B) to compile dylib and Swift code
3. Run (Cmd+R) to launch the app

## Troubleshooting

### "Library not found for -lfray"
- Ensure `build/libfray.dylib` exists
- Run `make dylib` from project root first
- Check Library Search Paths includes `$(PROJECT_DIR)/../build`

### "Cannot find 'FrayOpenDatabase' in scope"
- Ensure `-import-objc-header` flag is set correctly
- Verify `libfray.h` exists in FrayBridge/

### dylib not loading at runtime
- Check Copy Files phase copies to Frameworks
- Verify `@rpath/libfray.dylib` in app bundle

## Menu Bar Target (FrayMenuBar)

The menu bar app is a separate target that shares the FrayBridge code.

### Create Target

1. File → New → Target
2. Choose "macOS" → "App"
3. Configure:
   - Product Name: `FrayMenuBar`
   - Team: Same as main app
   - Interface: SwiftUI
   - Language: Swift
4. Click "Finish"

### Configure Target

1. **Info.plist**: Add `LSUIElement` = `YES` (hides dock icon)
2. **Build Settings**: Same Library/Header Search Paths as main target
3. **Build Phases**: Same dylib build and copy phases

### Share Code

Add these files to BOTH targets (check both targets in File Inspector):
- `FrayBridge/FrayBridge.swift`
- `FrayBridge/FrayTypes.swift`
- `FrayBridge/FrayError.swift`
- `Styles/FrayColors.swift` (for presence colors)

### Menu Bar Source Files

Add to FrayMenuBar target only:
- `FrayMenuBar/FrayMenuBarApp.swift`
- `FrayMenuBar/MenuBarContentView.swift`

## Next Steps

After project is created and building:
1. Import existing Swift files from `Fray/` directory
2. Import `FrayMenuBar/` files into the menu bar target
3. Test both apps work independently
