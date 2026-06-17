package loot

import "time"

// Loot is a single looted item or silver pickup, mirroring the upstream Loot
// model fields relevant to logging.
type Loot struct {
	Timestamp     time.Time
	LootedFromName string // body/container the loot came from ("MOB" for mobs)
	LootedByName   string // player who grabbed the loot
	IsSilver       bool
	ItemIndex      int // numeric item id (resolve to a name via item data dump)
	Quantity       int
}

// SilverGrab is a TakeSilver pickup.
type SilverGrab struct {
	Timestamp time.Time
	ObjectID  int
	Amount    int64
}

// NewLootContainer is a NewLoot event: a lootable body/container appearing in
// view, with the looter's name.
type NewLootContainer struct {
	Timestamp time.Time
	ObjectID  int
	BodyName  string
}
