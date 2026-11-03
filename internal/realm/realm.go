package realm

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
