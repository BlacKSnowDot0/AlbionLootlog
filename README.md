# ⚔️ AlbionLootlog

![CI](https://github.com/BlacKSnowDot0/AlbionLootlog/actions/workflows/ci.yml/badge.svg)
![Protocol Sync](https://github.com/BlacKSnowDot0/AlbionLootlog/actions/workflows/sync-protocol.yml/badge.svg)
![Release](https://github.com/BlacKSnowDot0/AlbionLootlog/actions/workflows/release.yml/badge.svg)
[![Latest Release](https://img.shields.io/github/v/release/BlacKSnowDot0/AlbionLootlog?label=latest)](https://github.com/BlacKSnowDot0/AlbionLootlog/releases/latest)
![License](https://img.shields.io/github/license/BlacKSnowDot0/AlbionLootlog)
![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)

**Headless Albion Online loot logger.**  
Launch it, play normally, and it prints loot to the console while saving a CSV in the current folder.

No UI. No overlay. No memory reading. No client modification. Just passive packet monitoring.

## ✨ Why This Exists

[AlbionOnline-StatisticsAnalysis](https://github.com/Triky313/AlbionOnline-StatisticsAnalysis) is a big C# desktop tool with many features. AlbionLootlog keeps only the part I wanted: **loot logging**.

It is built as a small Go CLI that starts fast, writes clean CSV files, and can be auto-updated when upstream protocol codes change.

## 📦 Download

Get the latest Windows build from:

👉 **[Releases](https://github.com/BlacKSnowDot0/AlbionLootlog/releases/latest)**

Download:

```text
AlbionLootlog-vX.Y.Z-windows-x64.exe
```

You also need [Npcap](https://npcap.com/) installed.

## 🚀 Quick Start

Open PowerShell in the folder with the exe:

```powershell
.\AlbionLootlog.exe
```

Then loot something in Albion.

You should see console lines like:

```text
level=INFO msg=loot type=silver looter=BlackSnowDot from="" quantity=180500
```

And a CSV file appears beside the exe:

```text
log-2026-06-17-02-47-50utc.csv
```

## 📄 CSV Output

The filename intentionally matches the upstream exporter:

```text
log-{DateTime.UtcNow:yyyy-MM-dd-hh-mm-ss}utc.csv
```

So yes, it uses **UTC** and upstream's **12-hour `hh` format**.

Example CSV:

```csv
timestamp_utc,event,looter,from,is_silver,item_index,item,quantity,object_id,amount,body
2026-06-17T17:46:33.9860000Z,grabbed_loot,BlackSnowDot,,true,0,,180500,,,
```

## 🛠️ Useful Commands

```powershell
# Verify file output without launching Albion
.\AlbionLootlog.exe -self-test

# List capture adapters
.\AlbionLootlog.exe -list-devices

# Use one specific adapter
.\AlbionLootlog.exe -device "\Device\NPF_{GUID}"

# Pick a custom CSV path
.\AlbionLootlog.exe -csv my-loot.csv

# Debug capture/parser counters
.\AlbionLootlog.exe -debug
```

## 🧭 Troubleshooting

When you stop the logger with `Ctrl+C`, it prints counters:

```text
photonPackets=... photonMessages=... photonEvents=... lootEvents=...
```

Use them like this:

| Counter Result | Meaning |
| --- | --- |
| `photonPackets=0` | Wrong adapter, missing admin rights, VPN routing, or Npcap issue |
| `photonPackets>0`, `photonEvents=0` | Photon parser needs fixing |
| `photonEvents>0`, `lootEvents=0` | Event codes/loot handler changed |
| `lootEvents>0` | CSV should contain loot rows |

If adapter auto-detection is noisy, run `-list-devices` and pick your real network adapter manually.

## 🔄 Auto-Updating Protocol Codes

Albion patches can shift Photon event and operation codes. This project mirrors upstream's positional C# enums into Go:

```powershell
go run .\tools\codegen
```

The scheduled workflow does this automatically:

- ✅ Safe enum/code changes are regenerated and pushed
- ⚠️ Handler logic changes open a review PR instead of guessing

This does **not** reverse-engineer Albion by itself. It tracks the community-maintained upstream source of truth.

## 🧱 Build From Source

Requirements:

- Go 1.24+
- Npcap SDK headers/libs
- CGO-capable C compiler

Build on Windows:

```powershell
.\build.ps1
```

Run tests:

```powershell
.\build.ps1 -Test
```

Create a GitHub release by pushing a tag:

```powershell
git tag v0.1.1
git push origin v0.1.1
```

The release workflow uploads:

```text
AlbionLootlog-v0.1.1-windows-x64.exe
```

## 🛡️ Safety

AlbionLootlog is passive and read-only. It only observes local network packets.

Do not add memory reading, packet injection, overlays, or gameplay automation.

## 📜 License

GPL-3.0. This matches the upstream project this port is based on.
