package character

// Stats is the calculated login snapshot, independent of wire field indices.
type Stats struct {
	Attributes                     [5]uint32
	Positive, Negative             [5]uint32
	MaxHealth, MaxMana             uint32
	Resistances                    [7]uint32
	AttackPower, RangedAttackPower int32
	AttackTime                     [3]uint32
	DamageMin, DamageMax           [3]float32
	ShieldBlock                    uint32
	Limitations                    []string
}
