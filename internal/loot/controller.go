package loot

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/BlacKSnowDot0/AlbionLootlog/internal/photon"
	"github.com/BlacKSnowDot0/AlbionLootlog/internal/protocol"
)

// Sink receives finished loot records for output (file, stdout, etc.).
type Sink interface {
	OnLoot(Loot)
	OnSilver(SilverGrab)
	OnNewLoot(NewLootContainer)
}

// Controller implements photon.Handler, routing decoded events to loot logic.
// It mirrors the relevant handlers wired in upstream NetworkManager.Build().
type Controller struct {
	sink Sink

	// mobNames tracks object ids known to be mobs, populated from NewMob
	// events. Upstream resolves looted-from names through MobController.IsMob;
	// we approximate by remembering mob object ids seen in the same session.
	mu       sync.RWMutex
	mobNames map[int]struct{}

	// lootEventCount is incremented on every loot-relevant event and used by
	// the health monitor to detect a likely protocol break ("no loot seen").
	lootEventCount atomic.Uint64
}

// NewController returns a Controller emitting records to sink.
func NewController(sink Sink) *Controller {
	return &Controller{
		sink:     sink,
		mobNames: make(map[int]struct{}),
	}
}

// LootEventCount returns the number of loot-relevant events processed so far.
func (c *Controller) LootEventCount() uint64 { return c.lootEventCount.Load() }

// OnEvent dispatches a decoded Photon event by its code.
func (c *Controller) OnEvent(e photon.Event) {
	switch protocol.EventCode(e.Code) {
	case protocol.EventCodeOtherGrabbedLoot:
		c.handleGrabbedLoot(e.Parameters)
	case protocol.EventCodeNewLoot:
		c.handleNewLoot(e.Parameters)
	case protocol.EventCodeNewMob:
		c.handleNewMob(e.Parameters)
	}
}

// OnRequest / OnResponse are unused for loot logging but satisfy the interface.
func (c *Controller) OnRequest(photon.OperationRequest)   {}
func (c *Controller) OnResponse(photon.OperationResponse) {}

// handleGrabbedLoot ports upstream GrabbedLootEvent. Field layout:
//
//	1: lootedFromName (player name, or a mob id resolved to "MOB")
//	2: looterByName
//	3: isSilver (bool)
//	4: itemIndex (int)
//	5: quantity (int)
func (c *Controller) handleGrabbedLoot(p map[byte]interface{}) {
	c.lootEventCount.Add(1)

	var lootedFrom string
	if v, ok := p[1]; ok {
		name := toString(v)
		if c.isMobName(name) {
			lootedFrom = "MOB"
		} else {
			lootedFrom = name
		}
	}

	rec := Loot{
		Timestamp:      time.Now(),
		LootedFromName: lootedFrom,
		LootedByName:   toString(p[2]),
		IsSilver:       toBool(p[3]),
		ItemIndex:      toInt(p[4]),
		Quantity:       toInt(p[5]),
	}
	c.sink.OnLoot(rec)
}

// handleNewLoot ports the NewLoot event: a lootable body appears.
//
//	0: objectId, 3: bodyName (looter/owner name)
func (c *Controller) handleNewLoot(p map[byte]interface{}) {
	c.lootEventCount.Add(1)
	c.sink.OnNewLoot(NewLootContainer{
		Timestamp: time.Now(),
		ObjectID:  toInt(p[0]),
		BodyName:  toString(p[3]),
	})
}

// handleNewMob records an object id as a mob so later loot can be attributed to
// "MOB" rather than a player name. NewMob field 0 is the object id.
func (c *Controller) handleNewMob(p map[byte]interface{}) {
	id := toInt(p[0])
	c.mu.Lock()
	c.mobNames[id] = struct{}{}
	c.mu.Unlock()
}

// isMobName reports whether the given looted-from name is a known mob. Upstream
// uses a dedicated MobController; here we treat any name that parses to a known
// mob object id as a mob. Non-numeric names are players.
func (c *Controller) isMobName(name string) bool {
	id, ok := parseIntStrict(name)
	if !ok {
		return false
	}
	c.mu.RLock()
	_, isMob := c.mobNames[id]
	c.mu.RUnlock()
	return isMob
}

// parseIntStrict parses a base-10 int, returning ok=false for non-numeric
// strings (i.e. player names).
func parseIntStrict(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	neg := false
	for i, r := range s {
		if i == 0 && r == '-' {
			neg = true
			continue
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}
