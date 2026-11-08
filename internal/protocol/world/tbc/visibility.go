package tbc

import (
	"math"
	"time"

	"wowserver/internal/protocol/world/tbc/codec"
)

// Visibility streaming for creatures and gameobjects.
//
// The reference keeps every Player's "client visibility" up to date as the
// player moves (Player::UpdateVisibilityOf / Map::UpdateObjectVisibility):
// objects entering the visibility distance are created for the client with
// SMSG_UPDATE_OBJECT, objects leaving it are removed with
// SMSG_DESTROY_OBJECT. Previously only the login snapshot was sent, so
// walking into unexplored areas showed an empty world.
const (
	visibleRadius     = 90.0 // DEFAULT_VISIBILITY_DISTANCE: objects appear inside this
	visibleKeepRadius = 95.0 // objects are only destroyed beyond this (no flicker at the edge)
	visibleMoveStep   = 8.0  // refresh after the player moved this far
	visibleTickEvery  = 2 * time.Second
)

// visibleSet is the per-session list of world objects the client knows.
type visibleSet struct {
	mapID  uint32
	x, y   float32
	next   time.Time
	known  map[uint64]struct{}
	inited bool
}

// spawnNearbyObjects is the world-enter snapshot: it resets the set and
// sends everything in range.
func (h *Handler) spawnNearbyObjects(s *Session) {
	s.vis = visibleSet{}
	h.refreshVisibility(s)
}

// maybeRefreshVisibility is called after player movement.
func (h *Handler) maybeRefreshVisibility(s *Session) {
	if s.player == nil || !s.vis.inited {
		return
	}
	dx, dy := s.player.X-s.vis.x, s.player.Y-s.vis.y
	if s.player.Map != s.vis.mapID || dx*dx+dy*dy >= visibleMoveStep*visibleMoveStep {
		h.refreshVisibility(s)
	}
}

// refreshVisibility creates the objects that came into range and destroys
// those that left it.
func (h *Handler) refreshVisibility(s *Session) {
	if s.player == nil || h.Data == nil {
		return
	}
	v := &s.vis
	now := time.Now()
	if !v.inited || v.mapID != s.player.Map {
		// New map (or first call): the client dropped its objects itself.
		v.known = make(map[uint64]struct{})
	}
	v.inited, v.mapID, v.x, v.y = true, s.player.Map, s.player.X, s.player.Y
	v.next = now.Add(visibleTickEvery)

	px, py, pz := s.player.X, s.player.Y, s.player.Z
	mapID := s.player.Map
	clock := uint32(now.UnixMilli())
	inRange := make(map[uint64]struct{}, len(v.known)+16)
	created := 0

	// Creatures: candidates by spawn point widened by the wander distance,
	// then judged at their live position.
	creatureRadius := float32(visibleKeepRadius) + h.Data.MaxCreatureWander()
	for _, cs := range h.Data.CreatureSpawnsNear(mapID, px, py, pz, creatureRadius) {
		x, y, z := cs.Data.PosX, cs.Data.PosY, cs.Data.PosZ
		o := cs.Data.Orientation
		live := false
		if lx, ly, lz, lo, ok := h.currentCreaturePosition(cs.GUID, now); ok {
			x, y, z, o, live = lx, ly, lz, lo, true
		}
		_ = live
		dx, dy := float64(x-px), float64(y-py)
		d := math.Sqrt(dx*dx + dy*dy)
		guid := codec.CreatureGUID(cs.Data.ID, cs.GUID)
		_, known := v.known[guid]
		if d > visibleKeepRadius || (!known && d > visibleRadius) {
			continue
		}
		lifeHealth, lifeFlags, lifeSet, lifeVisible := h.creatureVisibleState(cs.GUID)
		if !lifeVisible {
			continue // corpse already removed, respawn pending
		}
		inRange[guid] = struct{}{}
		if known {
			continue
		}
		resolved, ok := h.Data.ResolveCreatureSpawn(cs)
		if !ok {
			delete(inRange, guid)
			continue
		}
		if lifeSet {
			resolved.Health = lifeHealth
			if lifeFlags != 0 {
				resolved.UnitFlags = lifeFlags
			}
		}
		resolved.X, resolved.Y, resolved.Z, resolved.Orientation = x, y, z, o
		_ = s.send(codec.CreateObjectForCreature(resolved, clock))
		created++
	}

	// Gameobjects are static.
	for _, gs := range h.Data.GameObjectSpawnsNear(mapID, px, py, pz, visibleKeepRadius) {
		dx, dy := float64(gs.Data.PosX-px), float64(gs.Data.PosY-py)
		d := math.Sqrt(dx*dx + dy*dy)
		guid := codec.GameObjectGUID(gs.Data.ID, gs.GUID)
		_, known := v.known[guid]
		if d > visibleKeepRadius || (!known && d > visibleRadius) {
			continue
		}
		inRange[guid] = struct{}{}
		if known {
			continue
		}
		tmpl, ok := h.Data.ResolveGameObjectSpawn(gs)
		if !ok {
			delete(inRange, guid)
			continue
		}
		_ = s.send(codec.CreateObjectForGameObject(gs.GUID, gs, tmpl))
		created++
	}

	destroyed := 0
	for guid := range v.known {
		if _, ok := inRange[guid]; !ok {
			_ = s.send(codec.DestroyObjectGUID(guid))
			destroyed++
		}
	}
	v.known = inRange
	_, _ = created, destroyed
}

// visibilityTick refreshes periodically so wandering creatures that cross the
// range edge while the player stands still are created/destroyed too.
func (h *Handler) visibilityTick(s *Session, now time.Time) {
	if s.player != nil && s.vis.inited && !now.Before(s.vis.next) {
		h.refreshVisibility(s)
	}
}
