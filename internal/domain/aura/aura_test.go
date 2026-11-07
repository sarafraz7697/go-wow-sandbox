package aura

import "testing"

func TestPeriodicAndExpiry(t *testing.T) {
	var s Set
	h := NewHolder(433, 1, 1, 18000, true, [3]*Effect{{Name: PeriodicHeal, Amount: 7, Period: 3000}})
	if _, ok := s.Add(h); !ok || h.Slot != 0 {
		t.Fatal("add")
	}
	ticks, expired := 0, 0
	for i := 0; i < 180; i++ { // 100 ms steps over 18 s
		for _, e := range s.Update(100) {
			if e.Kind == Tick {
				ticks++
			} else {
				expired++
			}
		}
	}
	if ticks != 6 || expired != 1 || len(s.Holders()) != 0 {
		t.Fatalf("ticks %d expired %d", ticks, expired)
	}
}

func TestReplaceAndSlots(t *testing.T) {
	var s Set
	a := NewHolder(1, 9, 1, -1, true, [3]*Effect{{Name: ModStat, Amount: 5, Misc: -1}})
	s.Add(a)
	b := NewHolder(1, 9, 1, -1, true, a.Effects)
	if r, _ := s.Add(b); r != a || b.Slot != 0 {
		t.Fatal("same spell and caster must replace")
	}
	n := NewHolder(2, 9, 1, 1000, false, [3]*Effect{{Name: ModStat, Amount: -3, Misc: 1}})
	s.Add(n)
	if n.Slot != MaxPositiveAuras {
		t.Fatalf("negative slot %d", n.Slot)
	}
	m := s.StatMods()
	if m.Pos[0] != 5 || m.Neg[1] != 3 {
		t.Fatalf("%+v", m)
	}
	if got := s.Update(5000); len(got) != 1 || got[0].Kind != Expired {
		t.Fatalf("%v", got)
	}
}
