package loot

import (
	"testing"

	"github.com/BlacKSnowDot0/AlbionLootlog/internal/photon"
	"github.com/BlacKSnowDot0/AlbionLootlog/internal/protocol"
)

// recordSink captures emitted records for assertions.
type recordSink struct {
	loot   []Loot
	silver []SilverGrab
	newLot []NewLootContainer
}

func (s *recordSink) OnLoot(l Loot)                { s.loot = append(s.loot, l) }
func (s *recordSink) OnSilver(g SilverGrab)        { s.silver = append(s.silver, g) }
func (s *recordSink) OnNewLoot(n NewLootContainer) { s.newLot = append(s.newLot, n) }

func TestHandleGrabbedLoot_Item(t *testing.T) {
	sink := &recordSink{}
	c := NewController(sink)

	c.OnEvent(photon.Event{
		Code: int16(protocol.EventCodeOtherGrabbedLoot),
		Parameters: map[byte]interface{}{
			1: "PlayerVictim",
			2: "PlayerLooter",
			3: false,
			4: int32(1841),
			5: int32(2),
		},
	})

	if len(sink.loot) != 1 {
		t.Fatalf("expected 1 loot record, got %d", len(sink.loot))
	}
	l := sink.loot[0]
	if l.LootedFromName != "PlayerVictim" {
		t.Errorf("from: got %q", l.LootedFromName)
	}
	if l.LootedByName != "PlayerLooter" {
		t.Errorf("looter: got %q", l.LootedByName)
	}
	if l.IsSilver {
		t.Errorf("expected item, got silver")
	}
	if l.ItemIndex != 1841 || l.Quantity != 2 {
		t.Errorf("item/qty: got %d/%d", l.ItemIndex, l.Quantity)
	}
	if c.LootEventCount() != 1 {
		t.Errorf("loot event count: got %d", c.LootEventCount())
	}
}

func TestHandleGrabbedLoot_Silver(t *testing.T) {
	sink := &recordSink{}
	c := NewController(sink)

	c.OnEvent(photon.Event{
		Code: int16(protocol.EventCodeOtherGrabbedLoot),
		Parameters: map[byte]interface{}{
			2: "PlayerLooter",
			3: true,
			5: int32(1550115),
		},
	})

	if len(sink.loot) != 1 {
		t.Fatalf("expected 1 record, got %d", len(sink.loot))
	}
	if !sink.loot[0].IsSilver {
		t.Errorf("expected silver record")
	}
	if sink.loot[0].Quantity != 1550115 {
		t.Errorf("silver qty: got %d", sink.loot[0].Quantity)
	}
}

func TestHandleGrabbedLoot_MobResolution(t *testing.T) {
	sink := &recordSink{}
	c := NewController(sink)

	// Register object id 4242 as a mob.
	c.OnEvent(photon.Event{
		Code:       int16(protocol.EventCodeNewMob),
		Parameters: map[byte]interface{}{0: int32(4242)},
	})

	// Loot from "4242" (a mob object id) should resolve to "MOB".
	c.OnEvent(photon.Event{
		Code: int16(protocol.EventCodeOtherGrabbedLoot),
		Parameters: map[byte]interface{}{
			1: "4242",
			2: "PlayerLooter",
			4: int32(10),
			5: int32(1),
		},
	})

	if len(sink.loot) != 1 {
		t.Fatalf("expected 1 record, got %d", len(sink.loot))
	}
	if sink.loot[0].LootedFromName != "MOB" {
		t.Errorf("expected MOB resolution, got %q", sink.loot[0].LootedFromName)
	}
}

func TestTakeSilverIsNotLoggedAsLoot(t *testing.T) {
	sink := &recordSink{}
	c := NewController(sink)

	c.OnEvent(photon.Event{
		Code: int16(protocol.EventCodeTakeSilver),
		Parameters: map[byte]interface{}{
			0: int32(6436),
			1: int64(884995625105),
		},
	})

	if len(sink.loot) != 0 || len(sink.silver) != 0 {
		t.Fatalf("TakeSilver should not create loot rows, got loot=%d silver=%d", len(sink.loot), len(sink.silver))
	}
	if c.LootEventCount() != 0 {
		t.Fatalf("TakeSilver should not increment loot count, got %d", c.LootEventCount())
	}
}

func TestCoercion(t *testing.T) {
	if toInt(byte(5)) != 5 || toInt(int32(5)) != 5 || toInt(float64(5)) != 5 {
		t.Error("toInt coercion failed")
	}
	if toInt64(int32(7)) != 7 || toInt64(int64(7)) != 7 {
		t.Error("toInt64 coercion failed")
	}
	if !toBool(byte(1)) || toBool(byte(0)) {
		t.Error("toBool coercion failed")
	}
	if toString(int32(42)) != "42" || toString("x") != "x" || toString(nil) != "" {
		t.Error("toString coercion failed")
	}
}

func TestParseIntStrict(t *testing.T) {
	cases := []struct {
		in  string
		val int
		ok  bool
	}{
		{"123", 123, true},
		{"-45", -45, true},
		{"PlayerName", 0, false},
		{"12a3", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		v, ok := parseIntStrict(tc.in)
		if ok != tc.ok || (ok && v != tc.val) {
			t.Errorf("parseIntStrict(%q) = %d,%v; want %d,%v", tc.in, v, ok, tc.val, tc.ok)
		}
	}
}
