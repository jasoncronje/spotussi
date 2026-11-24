# Spotussi for iPhone

A read-only native app for one Spotussi instance. Scope and decisions: [docs/project/ios-mvp.md](../docs/project/ios-mvp.md) and D41 in [decisions.md](../docs/project/decisions.md). Not released yet.

## Layout

| Path | What it is |
|---|---|
| `Spotussi/` | The app target: screens and the app entry point |
| `SpotussiKit/` | A local Swift package. `SpotussiAPI` is the client generated from `api/openapi.yaml`. `SpotussiKit` holds pairing, the client set-up, the demo data and its generated cover art, the brand's colours and type, number and date formatting, the screens' view data and artwork loading |
| `project.yml` | The XcodeGen spec that `Spotussi.xcodeproj` is generated from |
| `openapi-generator-config.yaml` | Which API operations the client includes |
| `scripts/` | Regenerating the client, re-recording the demo data, drawing the app icon |

## Requirements

Xcode 27 or later. The app runs on iOS 26 and up.

## Build and test

```sh
# Package tests, on the Mac (no simulator needed)
swift test --package-path ios/SpotussiKit

# The app
open ios/Spotussi.xcodeproj
```

To run on a phone, pick your own team under Signing & Capabilities.

## When the API changes

```sh
ios/scripts/gen-api.sh           # regenerate the Swift client
ios/scripts/record-fixtures.py   # re-record the demo data from `serve --mock`
```

The first run of `gen-api.sh` builds Apple's Swift OpenAPI Generator into `~/Library/Caches/spotussi-tools`, which takes a few minutes. CI fails if the checked-in client doesn't match the spec.

## When the project settings change

Edit `project.yml`, then:

```sh
cd ios && xcodegen generate
```

New source files under `Spotussi/` are picked up without this.

## Links

Pairing takes one link, made by Settings → API & MCP → iPhone App on the web:

```
spotussi://pair?url=<base URL>&token=<spt_…>
```

`url` is the instance's origin and `token` is a read-only API token. Plain `http` works for IP addresses and local names; a public name needs `https`.

Once paired, these open a page on the tab in front (`name` is optional and titles the page until it loads):

```
spotussi://artist/<id>    spotussi://album/<id>    spotussi://track/<id>
spotussi://genre/<id>     spotussi://playlist/<id>?name=<name>
```

## Offline

The app keeps the last answer to every request in `Caches/api` (kept out of backups, readable once the phone has been unlocked after a restart) and asks again with its ETag. When the server can’t be reached (no connection, no answer in time, or a proxy answering 502, 503 or 504 for it), screens show what they last saw and a notice says when that was. A refused token or a server error is never answered from there. Unpairing empties it.

## App icon

```sh
swift ios/scripts/make-app-icon.swift
```

Draws the level mark into `Spotussi/Assets.xcassets/AppIcon.appiconset`: the default icon, a dark one and a tinted one.

## Running in the simulator

Build without `CODE_SIGNING_ALLOWED=NO` when you mean to run the app: an unsigned build can't write to the Keychain, so pairing ends in "Pairing didn't finish". CI only compiles, so it keeps signing off.

## Shipping to TestFlight

Signing is left to the owner: `DEVELOPMENT_TEAM` is empty in `project.yml`. The version is 0.1.0; each upload needs a higher build number (`CURRENT_PROJECT_VERSION`).

Once, in [App Store Connect](https://appstoreconnect.apple.com): Apps → + → New App, platform iOS, name Spotussi, bundle ID `app.spotussi.ios` (register it under Certificates, Identifiers & Profiles first if it isn't listed), any SKU.

Each build, in Xcode:

1. Open `ios/Spotussi.xcodeproj`.
2. Target Spotussi → Signing & Capabilities: tick Automatically manage signing and pick your team.
3. Target Spotussi → General → Identity: raise Build above the last upload.
4. Choose Any iOS Device (arm64) as the destination, then Product → Archive.
5. In the Organizer: Distribute App → TestFlight & App Store → Distribute.
6. In App Store Connect → TestFlight, wait for processing (10 to 30 minutes), then add internal testers. External testers also need Test Information (a description, a feedback address, a privacy policy URL) and Beta App Review; tell the reviewer to tap Try the demo on the first screen. For the App Store itself (blockers, metadata, review notes), see [app-store.md](../docs/project/app-store.md).

The same from the command line (on the MacBook Air, through the build slot), with your team ID:

```sh
xcodebuild archive -project ios/Spotussi.xcodeproj -scheme Spotussi -configuration Release \
  -destination 'generic/platform=iOS' -archivePath ios/build/Spotussi.xcarchive \
  -allowProvisioningUpdates DEVELOPMENT_TEAM=ABCDE12345 CURRENT_PROJECT_VERSION=2

mkdir -p ios/build
cat > ios/build/ExportOptions.plist <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>method</key><string>app-store-connect</string>
  <key>destination</key><string>upload</string>
  <key>teamID</key><string>ABCDE12345</string>
</dict>
</plist>
PLIST

xcodebuild -exportArchive -archivePath ios/build/Spotussi.xcarchive \
  -exportOptionsPlist ios/build/ExportOptions.plist -exportPath ios/build/export -allowProvisioningUpdates
```

`-allowProvisioningUpdates` needs the Apple account signed in under Xcode → Settings → Accounts. Export compliance is answered in `Info.plist` (`ITSAppUsesNonExemptEncryption` is NO: the app uses only the system's HTTPS), and `PrivacyInfo.xcprivacy` declares no tracking and no collected data, so the App Privacy answer is Data Not Collected.
