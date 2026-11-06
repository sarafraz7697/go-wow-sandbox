package gamedata

import (
	"math/rand"
	"sort"
	"testing"

	"wowserver/internal/entity"
)

// The grid index must return exactly what a brute-force scan returns.
func TestSpatialIndexMatchesLinearScan(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	c := &Catalog{}
	for i := 0; i < 3000; i++ {
		d := entity.CreatureData{ID: 1, MapID: uint32(r.Intn(2)), PosX: float32(r.Intn(4000) - 2000), PosY: float32(r.Intn(4000) - 2000), PosZ: float32(r.Intn(200)), MovementType: 1, SpawnDist: 5}
		c.creatureSpawns = append(c.creatureSpawns, CreatureSpawnRef{GUID: uint32(i + 1), Data: d})
		g := entity.GameObjectData{ID: 2, MapID: d.MapID, PosX: d.PosY, PosY: d.PosX, PosZ: d.PosZ}
		c.goSpawns = append(c.goSpawns, GameObjectSpawnRef{GUID: uint32(i + 1), Data: g})
	}
	brute := func(mapID uint32, x, y, z, rad float32) (cs, gs []uint32) {
		for _, s := range c.creatureSpawns {
			dx, dy, dz := s.Data.PosX-x, s.Data.PosY-y, s.Data.PosZ-z
			if s.Data.MapID == mapID && dx*dx+dy*dy+dz*dz <= rad*rad {
				cs = append(cs, s.GUID)
			}
		}
		for _, s := range c.goSpawns {
			dx, dy, dz := s.Data.PosX-x, s.Data.PosY-y, s.Data.PosZ-z
			if s.Data.MapID == mapID && dx*dx+dy*dy+dz*dz <= rad*rad {
				gs = append(gs, s.GUID)
			}
		}
		return
	}
	c.buildSpatial()
	if c.MaxCreatureWander() != 5 {
		t.Fatalf("max wander = %v", c.MaxCreatureWander())
	}
	for i := 0; i < 50; i++ {
		x, y, z := float32(r.Intn(4000)-2000), float32(r.Intn(4000)-2000), float32(r.Intn(200))
		rad := float32(30 + r.Intn(300))
		wantC, wantG := brute(1, x, y, z, rad)
		var gotC, gotG []uint32
		for _, s := range c.CreatureSpawnsNear(1, x, y, z, rad) {
			gotC = append(gotC, s.GUID)
		}
		for _, s := range c.GameObjectSpawnsNear(1, x, y, z, rad) {
			gotG = append(gotG, s.GUID)
		}
		for _, p := range [][]uint32{wantC, gotC, wantG, gotG} {
			sort.Slice(p, func(a, b int) bool { return p[a] < p[b] })
		}
		if len(wantC) != len(gotC) || len(wantG) != len(gotG) {
			t.Fatalf("mismatch at (%v,%v) r=%v: creatures %d/%d objects %d/%d", x, y, rad, len(gotC), len(wantC), len(gotG), len(wantG))
		}
		for j := range wantC {
			if wantC[j] != gotC[j] {
				t.Fatal("creature set differs")
			}
		}
	}
}
