package worldload

import (
	"context"

	"wowserver/internal/entity"
)

// gameObjectTemplateLoader ports ObjectMgr::LoadGameobjectInfo and the
// SQLHashStorage format for sGOStorage: gameobject_template entry, type,
// displayId, name, IconName, castBarCaption, faction, flags, size and the
// full data0..data23 block. The deprecated mingold/maxgold columns are
// not part of the reference struct and are left unread.
func init() {
	Register(Loader{
		Name:       "gameobject_template",
		Order:      OrderGameObjectTemplate,
		CountQuery: "SELECT COUNT(*) FROM `gameobject_template`",
		Load:       loadGameObjectTemplates,
	})
}

func loadGameObjectTemplates(ctx context.Context, q Queryer, dest *Dataset, t *Table) error {
	query := "SELECT `entry`,`type`,`displayId`,`name`,`IconName`,`castBarCaption`,`faction`,`flags`,`size`"
	for i := 0; i < entity.MaxGOData; i++ {
		query += ",`data" + itoa(i) + "`"
	}
	query += " FROM `gameobject_template` ORDER BY `entry`"

	return scanTable(ctx, q, t, query, 2000, func(r scanner) error {
		var v entity.GameObjectInfo
		dest2 := []any{
			u32(&v.ID), u32(&v.Type), u32(&v.DisplayID), str(&v.Name), str(&v.IconName), str(&v.CastBarCaption), u32(&v.Faction), u32(&v.Flags), f32(&v.Size),
		}
		for i := range v.Data {
			dest2 = append(dest2, u32(&v.Data[i]))
		}
		if err := r.Scan(dest2...); err != nil {
			return err
		}
		// Reference validation (LoadGameobjectInfo): too small a size is
		// raised to the default object scale; the entry is kept.
		if v.ID == 0 {
			t.Skipf("gameobject_template entry 0 is not a valid template")
			return nil
		}
		if v.Size <= 0 {
			t.Skipf("Gameobject (Entry: %d GoType: %d) have too small size=%f", v.ID, v.Type, v.Size)
			v.Size = 1
		}
		if _, dup := dest.GameObjectTemplates[v.ID]; dup {
			t.Skipf("gameobject_template has duplicate entry %d", v.ID)
			return nil
		}
		dest.GameObjectTemplates[v.ID] = v
		dest.GameObjectTemplateOrder = append(dest.GameObjectTemplateOrder, v.ID)
		t.Stored(1)
		return nil
	})
}

// itoa avoids importing strconv in every loader file for data column
// names.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
