package srs

import "testing"

// The dictionary "already in deck" check must catch cards saved WITHOUT a
// jmdict_id (imports, mined words) — an id-only check green-lights saving a
// duplicate of a card the learner has been reviewing for weeks.
func TestCardStatusByLemma(t *testing.T) {
	s := openTestStore(t)
	r, err := s.SaveCard(SaveCardReq{Lemma: "操作", Glosses: [][]string{{"operation"}}})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	rows, err := s.CardStatusByLemma([]string{"操作", "未保存"})
	if err != nil {
		t.Fatalf("lemma status: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (input order preserved)", len(rows))
	}
	if rows[0].Lemma != "操作" || rows[0].CardID == nil || *rows[0].CardID != r.ID {
		t.Errorf("操作 = %+v, want card %d", rows[0], r.ID)
	}
	if rows[1].CardID != nil {
		t.Errorf("未保存 should have no card, got %+v", rows[1])
	}
	empty, err := s.CardStatusByLemma(nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty input: got %v rows, err %v", len(empty), err)
	}
}
