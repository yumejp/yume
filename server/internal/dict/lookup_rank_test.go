package dict

import (
	"strings"
	"testing"
)

// Lookup must surface common entries (low prio from ke_pri/re_pri tags)
// ahead of rare homographs regardless of JMdict file order — はし used to
// rank 愛し (archaic) and 波斯 (Persia) around 端.
func TestLookupRanksCommonFirst(t *testing.T) {
	d := &Dict{index: map[string][]int{}}
	add := func(keb string, prio int) {
		idx := len(d.entries)
		d.entries = append(d.entries, Entry{Kanji: []string{keb}, Readings: []string{"はし"}, prio: prio})
		d.index["はし"] = append(d.index["はし"], idx)
	}
	add("愛し", 999) // rare, but earlier in file order
	add("波斯", 999)
	add("端", 10)
	add("橋", 5)

	res := d.Lookup([]string{"はし"})
	if len(res) != 1 || len(res[0].Entries) != 4 {
		t.Fatalf("unexpected result shape: %+v", res)
	}
	got := []string{res[0].Entries[0].Kanji[0], res[0].Entries[1].Kanji[0]}
	if got[0] != "橋" || got[1] != "端" {
		t.Fatalf("common entries not first: %v", got)
	}
	// stable within equal prio: 愛し (file-earlier) before 波斯
	if res[0].Entries[2].Kanji[0] != "愛し" {
		t.Fatalf("equal-prio order not stable: %v", res[0].Entries[2].Kanji)
	}
}

// ichi1/news1 on any form marks the entry common, while priority keeps the
// best nf band: 食べる is ichi1 + news2 + nf25.
func TestCommonFlagFromPriorityTags(t *testing.T) {
	d, err := Load(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
<JMdict>
<entry>
<ent_seq>1358280</ent_seq>
<k_ele><keb>食べる</keb><ke_pri>ichi1</ke_pri><ke_pri>news2</ke_pri><ke_pri>nf25</ke_pri></k_ele>
<r_ele><reb>たべる</reb><re_pri>ichi1</re_pri><re_pri>news2</re_pri><re_pri>nf25</re_pri></r_ele>
<sense><gloss>to eat</gloss></sense>
</entry>
<entry>
<ent_seq>1000001</ent_seq>
<k_ele><keb>稀語</keb><ke_pri>news2</ke_pri><ke_pri>nf40</ke_pri></k_ele>
<r_ele><reb>きご</reb></r_ele>
<sense><gloss>rare word</gloss></sense>
</entry>
</JMdict>`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res := d.Lookup([]string{"食べる", "稀語"})
	eat, rare := res[0].Entries[0], res[1].Entries[0]
	if !eat.Common || eat.Priority != 25 {
		t.Errorf("食べる common/priority = %v/%d, want true/25", eat.Common, eat.Priority)
	}
	if rare.Common || rare.Priority != 40 {
		t.Errorf("稀語 common/priority = %v/%d, want false/40", rare.Common, rare.Priority)
	}
}
