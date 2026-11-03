package gamedata

import "fmt"

type Enchantment struct {
	ID, Visual                  uint32
	Effects, Amounts, Arguments [3]uint32
	Condition                   uint32
}
type Creature struct{ ID, Family uint32 }

func (c *Catalog) Enchantment(id uint32) (Enchantment, bool) {
	v, ok := c.enchantments[id]
	return v, ok
}
func (c *Catalog) Creature(id uint32) (Creature, bool) { v, ok := c.creatures[id]; return v, ok }
func (c *Catalog) loadVisuals(client ClientData, db Rows) error {
	c.enchantments = make(map[uint32]Enchantment, len(client.Enchantments))
	c.creatures = make(map[uint32]Creature, len(db.Creatures))
	for _, v := range client.Enchantments {
		if _, ok := c.enchantments[v.ID]; ok {
			return fmt.Errorf("duplicate enchantment %d", v.ID)
		}
		c.enchantments[v.ID] = v
	}
	for _, v := range db.Creatures {
		if v.ID == 0 {
			continue
		}
		if _, ok := c.creatures[v.ID]; ok {
			return fmt.Errorf("duplicate creature %d", v.ID)
		}
		c.creatures[v.ID] = v
	}
	return nil
}
