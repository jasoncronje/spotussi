# Spotussi for iPhone

A read-only native app for one Spotussi instance. Scope and decisions: [docs/project/ios-mvp.md](../docs/project/ios-mvp.md) and D41 in [decisions.md](../docs/project/decisions.md). Not released yet.

## Layout

| Path | What it is |
|---|---|
| `Spotussi/` | The app target: screens and the app entry point |
| `SpotussiKit/` | A local Swift package. `SpotussiAPI` is the client generated from `api/openapi.yaml`. `SpotussiKit` holds pairing, the client set-up, the demo data and the brand's colours and type |
| `project.yml` | The XcodeGen spec that `Spotussi.xcodeproj` is generated from |
| `openapi-generator-config.yaml` | Which API operations the client includes |
| `scripts/` | Regenerating the client and re-recording the demo data |

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

## Pairing

The app takes one link, made by Settings → API & MCP → iPhone App on the web:

```
spotussi://pair?url=<base URL>&token=<spt_…>
```

`url` is the instance's origin and `token` is a read-only API token. Plain `http` works for IP addresses and local names; a public name needs `https`.

## Running in the simulator

Build without `CODE_SIGNING_ALLOWED=NO` when you mean to run the app: an unsigned build can't write to the Keychain, so pairing ends in "Pairing didn't finish". CI only compiles, so it keeps signing off.
