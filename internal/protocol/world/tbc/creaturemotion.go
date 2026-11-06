package tbc

import (
	"math"
	"math/rand"
	"sync"
	"time"

	domainworld "wowserver/internal/domain/world"
	"wowserver/internal/gamedata"
	"wowserver/internal/protocol/world/tbc/codec"
)

// Random creature movement, ported from RandomMovementGenerator.cpp
// (MovementType 1 = RANDOM_MOTION_TYPE) and MoveSplineInit::Launch.
//
// Constants come straight from the reference generator:
//   - chance of no pause between hops: 30%
//   - rest after a hop: urand(3000, 10000) ms
//   - retry delay when no point could be picked: 50 ms
//   - wander radius: creature.spawndist (GetRespawnRadius), min 0.1
//   - hop point: uniform in the disc around the SPAWN point
//     (angle = frand(0, 2pi), dist = radius * sqrt(frand(0, 1)))
//   - hops are walked at the creature's walk speed (MOVE_WALK)
//
// The reference snaps the hop point onto the navmesh/terrain; this server has
// no map data, so Z stays at the spawn Z.
const (
	motionChanceNoBreak = 30
	motionRestMin       = 3000
	motionRestMax       = 10000
	motionRetryDelay    = 50
	motionMinRadius     = 0.1
	motionCellSize      = 128.0
	// motionActiveRadius: only creatures near a player are simulated, like
	// the reference only updates creatures in active grids.
	motionActiveRadius = 140.0
	motionBroadcast    = 90.0 // DEFAULT_VISIBILITY_DISTANCE
)

type motionKey struct {
	mapID  uint32
	cx, cy int32
}

type creatureMover struct {
	spawn   gamedata.CreatureSpawnRef
	entry   uint32
	cx, cy  float32 // spawn centre
	cz      float32
	radius  float32
	walk    float32
	x, y, z float32 // position at the start of the current leg / standing position
	o       float32
	dx, dy  float32 // leg destination
	moving  bool
	legFrom time.Time
	legDur  time.Duration
	restDur time.Duration
	restEnd time.Time
}

type creatureMotion struct {
	mu     sync.Mutex
	init   bool
	cells  map[motionKey][]*creatureMover
	byGUID map[uint32]*creatureMover
	rng    *rand.Rand
}

func (m *creatureMotion) build(data *gamedata.Catalog) {
	m.init = true
	m.cells = make(map[motionKey][]*creatureMover)
	m.byGUID = make(map[uint32]*creatureMover)
	m.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	if data == nil {
		return
	}
	data.EachCreatureSpawn(func(s gamedata.CreatureSpawnRef) {
		if s.Data.MovementType != 1 || s.Data.SpawnDist <= 0 {
			return
		}
		r, ok := data.ResolveCreatureSpawn(s)
		if !ok {
			return
		}
		mv := &creatureMover{
			spawn: s, entry: r.Entry,
			cx: s.Data.PosX, cy: s.Data.PosY, cz: s.Data.PosZ,
			radius: s.Data.SpawnDist,
			walk:   r.SpeedWalk,
			x:      s.Data.PosX, y: s.Data.PosY, z: s.Data.PosZ, o: s.Data.Orientation,
		}
		if mv.radius < motionMinRadius {
			mv.radius = motionMinRadius
		}
		if mv.walk <= 0 {
			mv.walk = 2.5
		}
		k := cellOf(s.Data.MapID, mv.cx, mv.cy)
		m.cells[k] = append(m.cells[k], mv)
		m.byGUID[s.GUID] = mv
	})
}

func cellOf(mapID uint32, x, y float32) motionKey {
	return motionKey{mapID, int32(math.Floor(float64(x) / motionCellSize)), int32(math.Floor(float64(y) / motionCellSize))}
}

// pos returns the current interpolated position (linear spline).
func (c *creatureMover) pos(now time.Time) (x, y, z, o float32) {
	if !c.moving {
		return c.x, c.y, c.z, c.o
	}
	t := float32(now.Sub(c.legFrom)) / float32(c.legDur)
	if t > 1 {
		t = 1
	}
	return c.x + (c.dx-c.x)*t, c.y + (c.dy-c.y)*t, c.z, c.o
}

// finish settles a completed leg: the creature stands at the destination and
// the rest chosen at launch starts counting ("the timer only runs while the
// creature is standing").
func (c *creatureMover) finish(now time.Time) {
	if c.moving && now.Sub(c.legFrom) >= c.legDur {
		c.x, c.y = c.dx, c.dy
		c.moving = false
		c.restEnd = c.legFrom.Add(c.legDur).Add(c.restDur)
	}
}

// Tick advances every creature near (mapID, x, y) and returns the packets to
// broadcast as (creature position, packet) pairs.
func (h *Handler) tickCreatureMotion(mapID uint32, px, py float32, now time.Time) {
	if h.Data == nil || h.Broadcast == nil {
		return
	}
	m := &h.motion
	m.mu.Lock()
	if !m.init {
		m.build(h.Data)
	}
	var sends []func()
	r := int32(math.Ceil(motionActiveRadius / motionCellSize))
	base := cellOf(mapID, px, py)
	for ix := base.cx - r; ix <= base.cx+r; ix++ {
		for iy := base.cy - r; iy <= base.cy+r; iy++ {
			for _, c := range m.cells[motionKey{mapID, ix, iy}] {
				dx, dy := c.cx-px, c.cy-py
				if dx*dx+dy*dy > (motionActiveRadius+c.radius)*(motionActiveRadius+c.radius) {
					continue
				}
				c.finish(now)
				if c.moving || now.Before(c.restEnd) {
					continue
				}
				// Pick a hop (RandomPoint on the spawn disc).
				ang := m.rng.Float64() * 2 * math.Pi
				dist := float64(c.radius) * math.Sqrt(m.rng.Float64())
				nx := c.cx + float32(dist*math.Cos(ang))
				ny := c.cy + float32(dist*math.Sin(ang))
				ddx, ddy := float64(nx-c.x), float64(ny-c.y)
				length := math.Sqrt(ddx*ddx + ddy*ddy)
				// MoveSplineInit: duration = minimal_duration(1) + len * 1000 / velocity.
				durMs := uint32(1 + int32(float32(length)*(1000/c.walk)))
				// The rest that follows THIS hop is decided now.
				c.restDur = time.Duration(motionRetryDelay) * time.Millisecond
				if m.rng.Intn(100) >= motionChanceNoBreak {
					c.restDur = time.Duration(motionRestMin+m.rng.Intn(motionRestMax-motionRestMin+1)) * time.Millisecond
				}
				c.o = float32(math.Atan2(ddy, ddx))
				if c.o < 0 {
					c.o += 2 * math.Pi
				}
				c.dx, c.dy = nx, ny
				c.legFrom, c.legDur, c.moving = now, time.Duration(durMs)*time.Millisecond, true
				pkt := codec.MonsterMoveLinear(codec.CreatureGUID(c.entry, c.spawn.GUID),
					c.x, c.y, c.z, nx, ny, c.z, durMs)
				sx, sy, sz, mp := c.x, c.y, c.z, c.spawn.Data.MapID
				sends = append(sends, func() {
					h.Broadcast.BroadcastArea(mp, sx, sy, sz, motionBroadcast, domainworld.Packet{Opcode: pkt.Opcode, Payload: pkt.Payload})
				})
			}
		}
	}
	m.mu.Unlock()
	for _, f := range sends {
		f()
	}
}

// currentCreaturePosition returns the live position of a spawn.
func (h *Handler) currentCreaturePosition(spawnGUID uint32, now time.Time) (x, y, z, o float32, moved bool) {
	m := &h.motion
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.init {
		m.build(h.Data)
	}
	c, ok := m.byGUID[spawnGUID]
	if !ok {
		return 0, 0, 0, 0, false
	}
	c.finish(now)
	x, y, z, o = c.pos(now)
	return x, y, z, o, true
}

// stopCreatureMoving ports Creature::StopMoving -> MoveSplineInit::Stop: the
// creature halts where it is and SMSG_MONSTER_MOVE (MonsterMoveStop) goes out.
func (h *Handler) stopCreatureMoving(spawnGUID uint32) {
	m := &h.motion
	now := time.Now()
	m.mu.Lock()
	if !m.init {
		m.build(h.Data)
	}
	c, ok := m.byGUID[spawnGUID]
	if !ok {
		m.mu.Unlock()
		return
	}
	c.finish(now)
	if !c.moving {
		m.mu.Unlock()
		return
	}
	c.x, c.y, c.z, _ = c.pos(now)
	c.moving = false
	c.restEnd = now.Add(c.restDur)
	pkt := codec.MonsterMoveStop(codec.CreatureGUID(c.entry, spawnGUID), c.x, c.y, c.z)
	mp, sx, sy, sz := c.spawn.Data.MapID, c.x, c.y, c.z
	m.mu.Unlock()
	if h.Broadcast != nil {
		h.Broadcast.BroadcastArea(mp, sx, sy, sz, motionBroadcast, domainworld.Packet{Opcode: pkt.Opcode, Payload: pkt.Payload})
	}
}

// creaturePosition returns the creature's current position and whether it
// is alive (Creature::IsAlive).
func (h *Handler) creaturePosition(spawn gamedata.ResolvedCreatureSpawn) (x, y, z float32, alive bool) {
	if cx, cy, cz, _, ok := h.currentCreaturePosition(spawn.GUID, time.Now()); ok {
		return cx, cy, cz, true
	}
	return spawn.X, spawn.Y, spawn.Z, true
}
