// Command lootlogger is a headless Albion Online loot logger. It captures
// Photon network traffic, decodes loot-related events, and writes them to the
// console and a timestamped CSV file. No UI: launch it and it logs.
//
// This is a Go port of the loot-logger feature of
// Triky313/AlbionOnline-StatisticsAnalysis (GPL-3.0). This program is likewise
// licensed under GPL-3.0; see the LICENSE file.
//
// Requirements:
//   - Npcap installed (https://npcap.com). Run as Administrator on Windows if
//     not using Npcap's "WinPcap API-compatible mode" / admin-free capture.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BlacKSnowDot0/AlbionLootlog/internal/capture"
	"github.com/BlacKSnowDot0/AlbionLootlog/internal/loot"
	"github.com/BlacKSnowDot0/AlbionLootlog/internal/photon"
)

func main() {
	var (
		device      = flag.String("device", "", "pcap device name to capture on (empty = all eligible devices)")
		bpf         = flag.String("filter", capture.DefaultBPFFilter, "BPF capture filter")
		csvPath     = flag.String("csv", defaultCSVPath(time.Now().UTC()), "append loot records as CSV rows to this file (empty = disabled)")
		listDevices = flag.Bool("list-devices", false, "list capturable network devices and exit")
		selfTest    = flag.Bool("self-test", false, "write one sample loot record to the configured CSV file and exit")
		debug       = flag.Bool("debug", false, "enable debug logging")
		healthAfter = flag.Duration("health-warn-after", 5*time.Minute, "warn if no loot events are seen within this duration (0 = disabled)")
	)
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if *listDevices {
		if err := printDevices(); err != nil {
			logger.Error("list devices failed", "err", err)
			os.Exit(1)
		}
		return
	}

	// CSV output file. By default this creates log-<UTC timestamp>utc.csv so
	// double-click or plain command-line launches still leave an artifact on disk.
	var csvOut *os.File
	if *csvPath != "" {
		f, err := os.OpenFile(*csvPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			logger.Error("open csv output failed", "path", *csvPath, "err", err)
			os.Exit(1)
		}
		defer f.Close()
		csvOut = f
		logger.Info("writing CSV", "path", *csvPath)
	}

	// Build the output sink. The self-test path gives us a deterministic way to
	// verify file writing without needing live Albion traffic.
	var sink *loot.LogSink
	if csvOut != nil {
		sink = loot.NewLogSink(logger, csvOut, nil)
	} else {
		sink = loot.NewLogSink(logger, nil, nil)
	}
	if *selfTest {
		sink.OnLoot(loot.Loot{
			Timestamp:      time.Now(),
			LootedFromName: "SELF_TEST_BODY",
			LootedByName:   "SELF_TEST_PLAYER",
			ItemIndex:      1841,
			Quantity:       1,
		})
		logger.Info("self-test complete", "csv", *csvPath)
		return
	}

	// Build the pipeline: capture -> photon parser -> loot controller -> sink.
	controller := loot.NewController(sink)
	parser := photon.NewParser(controller)

	cap := capture.New(capture.Options{
		Device:    *device,
		BPFFilter: *bpf,
		Logger:    logger,
	}, parser)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Health monitor: warn (once) if no loot events appear, which usually means
	// the protocol codes shifted after a game patch and need re-syncing.
	if *healthAfter > 0 {
		go healthMonitor(ctx, logger, controller, parser, *healthAfter)
	}

	logger.Info("albion loot logger starting", "device", deviceLabel(*device))
	if err := cap.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("capture failed", "err", err)
		os.Exit(1)
	}
	stats := parser.Stats()
	logger.Info("albion loot logger stopped",
		"photonPackets", stats.Packets,
		"photonMessages", stats.Messages,
		"photonEvents", stats.Events,
		"photonRequests", stats.Requests,
		"photonResponses", stats.Responses,
		"lootEvents", controller.LootEventCount(),
	)
}

func deviceLabel(d string) string {
	if d == "" {
		return "auto (all eligible)"
	}
	return d
}

func defaultCSVPath(now time.Time) string {
	// Match upstream StatisticsAnalysisTool export naming exactly:
	// $"log-{DateTime.UtcNow:yyyy-MM-dd-hh-mm-ss}utc.csv"
	// Note the 12-hour clock (hh), not 24-hour (HH).
	return "log-" + now.Format("2006-01-02-03-04-05") + "utc.csv"
}

func printDevices() error {
	devs, err := capture.ListDevices()
	if err != nil {
		return err
	}
	if len(devs) == 0 {
		fmt.Println("No capturable devices found. Is Npcap installed?")
		return nil
	}
	for _, d := range devs {
		desc := d.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("%s\n    %s\n", d.Name, desc)
		for _, a := range d.Addresses {
			fmt.Printf("    addr: %s\n", a.IP)
		}
	}
	return nil
}

// healthMonitor warns once if no loot events are observed within d. A
// persistent zero count after a game patch is the signal to re-run the codegen
// sync (or wait for the auto-update workflow's PR).
func healthMonitor(ctx context.Context, logger *slog.Logger, c *loot.Controller, p *photon.Parser, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		if c.LootEventCount() == 0 {
			stats := p.Stats()
			logger.Warn("no loot events seen yet",
				"after", d.String(),
				"photonPackets", stats.Packets,
				"photonMessages", stats.Messages,
				"photonEvents", stats.Events,
				"photonRequests", stats.Requests,
				"photonResponses", stats.Responses,
				"hint", "if you have been looting, the Photon protocol codes may have shifted after a game patch; re-sync constants (go run ./tools/codegen) or update from upstream",
			)
		}
	}
}
