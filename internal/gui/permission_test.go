package gui

import "testing"

// The dead end this replaces was the complaint: one sentence telling the user
// to restart, on a route that offers no way to do it. The order of the answers
// is the fix, so it is worth pinning down.
func TestMissingGrantAsksThenOffersTheDialog(t *testing.T) {
	var p permissionAsk
	if prompt, dialog := p.step(); !prompt || dialog {
		t.Fatalf("first press = prompt %v dialog %v, want the system prompt alone", prompt, dialog)
	}
	if prompt, dialog := p.step(); prompt || !dialog {
		t.Fatalf("second press = prompt %v dialog %v, want the dialog alone", prompt, dialog)
	}
	for i := 3; i <= 5; i++ {
		if prompt, dialog := p.step(); prompt || dialog {
			t.Fatalf("press %d asked again: prompt %v dialog %v", i, prompt, dialog)
		}
	}
}

// 屏幕录制 and 辅助功能 are separate checkboxes, so exhausting one prompt must
// not silence the other.
func TestEachGrantKeepsItsOwnAsking(t *testing.T) {
	askScreen.step()
	if prompt, _ := askTrust.step(); !prompt {
		t.Fatal("辅助功能 lost its system prompt to 屏幕录制")
	}
}
