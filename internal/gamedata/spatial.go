package gamedata

import "math"

// spatialCell is the side of one index cell in yards. The index replaces the
// linear scan over every spawn that CreatureSpawnsNear / GameObjectSpawnsNear
// used to do, which cannot be run on every player movement update.
const spatialCell = 128.0

type spatialKey struct {
	mapID  uint32
	cx, cy int32
}

type spatialIndex struct {
	creatures map[spatialKey][]int32
	objects   map[spatialKey][]int32
	// maxWander is the largest creature.spawndist: a wandering creature may
	// be up to that far from its spawn point.
	maxWander float32
}

func keyOf(mapID uint32, x, y float32) spatialKey {
	return spatialKey{mapID, int32(math.Floor(float64(x) / spatialCell)), int32(math.Floor(float64(y) / spatialCell))}
}

func (c *Catalog) buildSpatial() {
	idx := &spatialIndex{creatures: map[spatialKey][]int32{}, objects: map[spatialKey][]int32{}}
	for i, s := range c.creatureSpawns {
		k := keyOf(s.Data.MapID, s.Data.PosX, s.Data.PosY)
		idx.creatures[k] = append(idx.creatures[k], int32(i))
		if s.Data.MovementType == 1 && s.Data.SpawnDist > idx.maxWander {
			idx.maxWander = s.Data.SpawnDist
		}
	}
	for i, s := range c.goSpawns {
		k := keyOf(s.Data.MapID, s.Data.PosX, s.Data.PosY)
		idx.objects[k] = append(idx.objects[k], int32(i))
	}
	c.spatial = idx
}

// MaxCreatureWander returns the largest wander distance of any spawn, so
// callers can widen a range query for creatures that left their spawn point.
func (c *Catalog) MaxCreatureWander() float32 {
	if c.spatial == nil {
		return 0
	}
	return c.spatial.maxWander
}

func cellRange(x, y, radius float32) (x0, x1, y0, y1 int32) {
	x0 = int32(math.Floor(float64(x-radius) / spatialCell))
	x1 = int32(math.Floor(float64(x+radius) / spatialCell))
	y0 = int32(math.Floor(float64(y-radius) / spatialCell))
	y1 = int32(math.Floor(float64(y+radius) / spatialCell))
	return
}
