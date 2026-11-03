package realm

// DOMAIN_WORLD_SPLIT_PATCH: domain model (no protocol, no SQL)

type Realm struct {
	ID                    uint32
	Name, Host            string
	Port                  uint16
	Type, Flags, Category uint8
	Locked                bool
	Population            float32
	Characters            uint8
	Build                 uint16
}
