package srs

import "testing"

func TestClozeTargetPrefersSurfaceThenLemmaThenKanji(t *testing.T) {
	surf := "解け"
	if got := clozeTargetFor("問題が解けた。", &surf, "解ける", []string{"解ける"}); got != "解け" {
		t.Errorf("surface first: %q", got)
	}
	if got := clozeTargetFor("問題が解ける。", nil, "解ける", []string{"とける"}); got != "解ける" {
		t.Errorf("lemma second: %q", got)
	}
	if got := clozeTargetFor("こだわりが強い。", nil, "拘り", []string{"拘り", "こだわり"}); got != "こだわり" {
		t.Errorf("kanji spellings last: %q", got)
	}
	if got := clozeTargetFor("関係ない文。", nil, "解ける", nil); got != "" {
		t.Errorf("no match: %q", got)
	}
}

func TestErrCodeTravelsWithClientErrors(t *testing.T) {
	err := clientCodef(ErrCodeReviewedAtRejected, "reviewed_at precedes the card's last review")
	if !IsClientErr(err) || ErrCode(err) != ErrCodeReviewedAtRejected {
		t.Errorf("code = %q client=%v", ErrCode(err), IsClientErr(err))
	}
	if ErrCode(clientErrf("plain")) != "" {
		t.Error("plain client errors carry no code")
	}
}
