package worldload

import (
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"time"
)

// scanner is implemented by *sql.Rows and *sql.Row.
type scanner interface {
	Scan(dest ...any) error
}

// The MaNGOS Field::GetXXX accessors silently return zero for NULL
// columns. The MySQL driver refuses to scan NULL into a Go int or string,
// so the loaders scan through these small pointer-wrappers that default
// to the destination's zero value — the same behavior as the reference.
//
// Usage inside a scan callback:
//
//	rows.Scan(u32(&v.Entry), str(&v.Name), i32(&v.Expansion), f32(&v.Scale))
type (
	pu32  struct{ p *uint32 }
	pi32  struct{ p *int32 }
	pf32  struct{ p *float32 }
	pstr  struct{ p *string }
	pu8   struct{ p *uint8 }
	pbool struct{ p *bool }
)

// u32 scans a nullable unsigned column (NULL and negatives become 0).
func u32(p *uint32) *pu32 { return &pu32{p} }

// i32 scans a nullable signed column.
func i32(p *int32) *pi32 { return &pi32{p} }

// f32 scans a nullable float column.
func f32(p *float32) *pf32 { return &pf32{p} }

// str scans a nullable string column (NULL becomes "").
func str(p *string) *pstr { return &pstr{p} }

// u8 scans a nullable small unsigned column.
func u8(p *uint8) *pu8 { return &pu8{p} }

// boolean scans a tinyint column (any non-zero value is true, like
// MaNGOS Field::GetBool).
func boolean(p *bool) *pbool { return &pbool{p} }

func (n *pu32) Scan(src any) error {
	v, err := toInt(src)
	if err != nil {
		return err
	}
	if v < 0 {
		v = 0
	}
	*n.p = uint32(v)
	return nil
}

func (n *pi32) Scan(src any) error {
	v, err := toInt(src)
	if err != nil {
		return err
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		return fmt.Errorf("int32 out of range: %d", v)
	}
	*n.p = int32(v)
	return nil
}

func (n *pf32) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*n.p = 0
	case float64:
		*n.p = float32(v)
	case int64:
		*n.p = float32(v)
	case []byte:
		f, err := strconv.ParseFloat(string(v), 32)
		if err != nil {
			return err
		}
		*n.p = float32(f)
	default:
		return fmt.Errorf("float scan: unsupported type %T", src)
	}
	return nil
}

func (n *pstr) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*n.p = ""
	case string:
		*n.p = v
	case []byte:
		*n.p = string(v)
	case time.Time:
		// Drivers configured with parseTime (and the SQLite test driver)
		// hand DATETIME columns back as time.Time; render them like the
		// MySQL textual format the reference reads.
		*n.p = v.Format("2006-01-02 15:04:05")
	default:
		return fmt.Errorf("string scan: unsupported type %T", src)
	}
	return nil
}

func (n *pu8) Scan(src any) error {
	v, err := toInt(src)
	if err != nil {
		return err
	}
	if v < 0 || v > 255 {
		v = 0
	}
	*n.p = uint8(v)
	return nil
}

func (n *pbool) Scan(src any) error {
	v, err := toInt(src)
	if err != nil {
		return err
	}
	*n.p = v != 0
	return nil
}

// nullI32Raw and nullBoolRaw are value-holding nullable wrappers for
// loaders that need to test the scanned value (event/pool ids, death
// state) instead of writing it straight into the entity.
type nullI32Raw struct{ v int32 }

func (n *nullI32Raw) Scan(src any) error {
	v, err := toInt(src)
	if err != nil {
		return err
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		return fmt.Errorf("int32 out of range: %d", v)
	}
	n.v = int32(v)
	return nil
}

type nullBoolRaw struct{ v bool }

func (n *nullBoolRaw) Scan(src any) error {
	v, err := toInt(src)
	if err != nil {
		return err
	}
	n.v = v != 0
	return nil
}

// toInt normalizes every numeric shape the MySQL driver produces
// (int64, uint64, float64, []byte, string) into an int64.
func toInt(src any) (int64, error) {
	switch v := src.(type) {
	case nil:
		return 0, nil
	case int64:
		return v, nil
	case uint64:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("numeric scan: unsupported type %T", src)
	}
}

// Compile-time interface checks for the wrapper types.
var (
	_ sql.Scanner = (*pu32)(nil)
	_ sql.Scanner = (*pi32)(nil)
	_ sql.Scanner = (*pf32)(nil)
	_ sql.Scanner = (*pstr)(nil)
	_ sql.Scanner = (*pu8)(nil)
	_ sql.Scanner = (*pbool)(nil)
)
