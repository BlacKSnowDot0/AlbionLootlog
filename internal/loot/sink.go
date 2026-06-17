package loot

import (
	"encoding/csv"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// LogSink writes loot records as human-readable console lines and optionally as
// CSV rows to a separate writer. It is safe for concurrent use.
type LogSink struct {
	logger *slog.Logger

	mu       sync.Mutex
	csv      *csv.Writer // optional; one loot-related record per row
	resolver ItemResolver
}

// ItemResolver maps a numeric item index to a human-readable item id/name.
// A nil resolver logs the raw index. This is where ao-bin-dumps item data would
// be wired in (fully automatable via the data-sync workflow).
type ItemResolver interface {
	Resolve(itemIndex int) (name string, ok bool)
}

// NewLogSink returns a LogSink. csvOut may be nil to disable file output.
func NewLogSink(logger *slog.Logger, csvOut io.Writer, resolver ItemResolver) *LogSink {
	if logger == nil {
		logger = slog.Default()
	}
	s := &LogSink{logger: logger, resolver: resolver}
	if csvOut != nil {
		s.csv = csv.NewWriter(csvOut)
		_ = s.csv.Write([]string{
			"timestamp_utc",
			"event",
			"looter",
			"from",
			"is_silver",
			"item_index",
			"item",
			"quantity",
			"object_id",
			"amount",
			"body",
		})
		s.csv.Flush()
	}
	return s
}

func (s *LogSink) itemName(idx int) string {
	if s.resolver != nil {
		if name, ok := s.resolver.Resolve(idx); ok {
			return name
		}
	}
	return ""
}

// OnLoot logs a grabbed-loot record.
func (s *LogSink) OnLoot(l Loot) {
	name := s.itemName(l.ItemIndex)
	if l.IsSilver {
		s.logger.Info("loot",
			"type", "silver",
			"looter", l.LootedByName,
			"from", l.LootedFromName,
			"quantity", l.Quantity,
		)
	} else {
		attrs := []any{
			"type", "item",
			"looter", l.LootedByName,
			"from", l.LootedFromName,
			"itemIndex", l.ItemIndex,
			"quantity", l.Quantity,
		}
		if name != "" {
			attrs = append(attrs, "item", name)
		}
		s.logger.Info("loot", attrs...)
	}
	s.writeCSV([]string{
		l.Timestamp.UTC().Format(time.RFC3339Nano),
		"grabbed_loot",
		l.LootedByName,
		l.LootedFromName,
		strconv.FormatBool(l.IsSilver),
		strconv.Itoa(l.ItemIndex),
		name,
		strconv.Itoa(l.Quantity),
		"",
		"",
		"",
	})
}

// OnSilver logs a silver pickup.
func (s *LogSink) OnSilver(g SilverGrab) {
	s.logger.Info("silver", "objectId", g.ObjectID, "amount", g.Amount)
	s.writeCSV([]string{
		g.Timestamp.UTC().Format(time.RFC3339Nano),
		"take_silver",
		"",
		"",
		"true",
		"",
		"",
		"",
		strconv.Itoa(g.ObjectID),
		strconv.FormatInt(g.Amount, 10),
		"",
	})
}

// OnNewLoot logs a new lootable container appearing.
func (s *LogSink) OnNewLoot(n NewLootContainer) {
	s.logger.Debug("new_loot", "objectId", n.ObjectID, "body", n.BodyName)
	s.writeCSV([]string{
		n.Timestamp.UTC().Format(time.RFC3339Nano),
		"new_loot",
		"",
		"",
		"",
		"",
		"",
		"",
		strconv.Itoa(n.ObjectID),
		"",
		n.BodyName,
	})
}

func (s *LogSink) writeCSV(row []string) {
	if s.csv == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.csv.Write(row)
	s.csv.Flush()
}
