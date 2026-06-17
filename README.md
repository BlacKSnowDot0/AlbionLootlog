# AlbionLootlog

AlbionLootlog is a headless loot logger for Albion Online. Start it, loot in
game, and it writes loot records to both the console and a CSV file in the
current folder.

It is a focused Go port of the loot logging path from
[Triky313/AlbionOnline-StatisticsAnalysis](https://github.com/Triky313/AlbionOnline-StatisticsAnalysis).
There is no UI, no overlay, no memory reading, and no client modification. It
only listens to local network traffic and decodes Albion's Photon packets.

## Current Status

Working:

- Npcap/libpcap capture on Albion's Photon UDP ports: `5055`, `5056`, `5058`
- Photon command parsing and Protocol18 deserialization
- Protocol code generation from upstream `EventCodes.cs` and `OperationCodes.cs`
- `OtherGrabbedLoot` loot records, including silver pickups
- UTC CSV output using the same default filename pattern as upstream
- GitHub Actions for CI, protocol auto-update, and tagged Windows releases

Known limitations:

- Item names are not resolved yet; CSV currently stores numeric `item_index`.
- Guild/alliance/cluster enrichment is not implemented yet.
- Major upstream handler logic changes still need human review.

## Output

By default, the program writes a CSV file in the current folder:

```text
log-2026-06-17-02-47-50utc.csv
```

The filename intentionally matches upstream's export naming:

```csharp
$"log-{DateTime.UtcNow:yyyy-MM-dd-hh-mm-ss}utc.csv"
```

That means:

- Timestamp source: UTC
- Hour format: 12-hour `hh`, matching upstream exactly
- Folder: current working directory

CSV row timestamps are also UTC and stored in the `timestamp_utc` column.

Example console output:

```text
time=2026-06-17T21:16:33.986+03:30 level=INFO msg=loot type=silver looter=BlackSnowDot from="" quantity=180500
```

Example CSV row:

```csv
timestamp_utc,event,looter,from,is_silver,item_index,item,quantity,object_id,amount,body
2026-06-17T17:46:33.9860000Z,grabbed_loot,BlackSnowDot,,true,0,,180500,,,
```

## Requirements

- Windows 10 or newer
- [Npcap](https://npcap.com/) installed
- Administrator terminal if your Npcap install requires admin capture

For building from source:

- Go 1.24+
- CGO-capable C compiler
- Npcap SDK headers and libraries

## Build

Use the PowerShell helper on Windows:

```powershell
.\build.ps1          # build AlbionLootlog.exe
.\build.ps1 -Test    # run tests
.\build.ps1 -Run     # build and run
```

The helper sets the CGO flags for the default Npcap install path:

```text
C:\Program Files\Npcap\Include
C:\Program Files\Npcap\Lib\x64
```

If your Npcap SDK is elsewhere, edit the two paths at the top of `build.ps1`.

## Run

Auto-detect adapter and start logging:

```powershell
.\AlbionLootlog.exe
```

Verify file output without starting Albion:

```powershell
.\AlbionLootlog.exe -self-test
```

List adapters:

```powershell
.\AlbionLootlog.exe -list-devices
```

Use one specific adapter:

```powershell
.\AlbionLootlog.exe -device "\Device\NPF_{GUID}"
```

Write CSV to a specific path:

```powershell
.\AlbionLootlog.exe -csv my-loot.csv
```

Enable parser/capture diagnostics:

```powershell
.\AlbionLootlog.exe -debug
```

## Flags

| Flag | Default | Description |
| --- | --- | --- |
| `-device` | auto | Pcap device name. Empty captures all eligible devices. |
| `-filter` | Photon BPF | Override the BPF packet filter. |
| `-csv` | `log-<utc timestamp>utc.csv` | CSV output path. Empty disables file output. |
| `-list-devices` | false | Print capturable adapters and exit. |
| `-self-test` | false | Write one sample loot row and exit. |
| `-debug` | false | Enable debug logging. |
| `-health-warn-after` | `5m` | Warn if no loot events are decoded after this duration. |

When the process stops, it prints parser counters:

```text
photonPackets=... photonMessages=... photonEvents=... lootEvents=...
```

Use those counters to separate capture problems from parser or handler problems:

- `photonPackets=0`: wrong adapter, missing permissions, VPN/ExitLag routing, or filter issue
- `photonPackets>0` and `photonEvents=0`: Photon parser mismatch
- `photonEvents>0` and `lootEvents=0`: event code or loot handler mismatch
- `lootEvents>0`: CSV should contain loot rows

## Project Layout

```text
cmd/lootlogger/          CLI entry point
internal/capture/        Npcap/gopacket capture layer
internal/photon/         Photon command parser + Protocol18 deserializer
internal/protocol/       Generated event/operation code constants
internal/loot/           Loot event handlers and CSV/console sink
tools/codegen/           Upstream C# enum to Go constant generator
.github/workflows/       CI, protocol updater, release automation
```

## Protocol Auto-Update

Albion patches can shift Photon event and operation codes. Upstream tracks those
codes in positional C# enums. This project mirrors them into Go generated files:

```powershell
go run .\tools\codegen
```

The scheduled updater workflow does two different things:

- Safe enum changes: regenerate `internal/protocol/*_gen.go`, test, and push the
  generated update automatically.
- Risky handler changes: hash selected upstream loot/capture source files and
  open a review PR if their logic changes.

This is intentionally not "AI reverse engineering". It trusts upstream's manual
protocol updates and keeps this Go port synchronized with them.

## CI, Builder, Release

`.github/workflows/ci.yml` runs on pushes and pull requests:

- Linux pure-Go build/test for protocol, parser, loot, and codegen packages
- Windows full build/test with Npcap SDK and CGO

`.github/workflows/release.yml` builds a Windows x64 executable when a version
tag is pushed:

```powershell
git tag v0.1.0
git push origin v0.1.0
```

The release artifact is named:

```text
AlbionLootlog-v0.1.0-windows-x64.exe
```

`.github/workflows/sync-protocol.yml` is the protocol auto-updater.

## Legal / Safety Notes

The upstream project documents that passive network monitoring is tolerated when
the tool only observes traffic and does not modify the game client or service.
AlbionLootlog is passive and read-only by design.

Do not add:

- memory reading
- packet injection
- client modification
- overlays
- automation that plays the game

## License

GPL-3.0, matching the upstream project this port is based on. See `LICENSE`.
