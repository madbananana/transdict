package selection

import "testing"

func TestTrackerWaitsForTheSelectionToSettle(t *testing.T) {
	var tr tracker
	// A slow drag over "DSP Studio" reads these partial highlights on the way.
	for _, partial := range []string{"D", "DS", "DSP", "DSP S", "DSP Studio"} {
		if tr.observe(partial) {
			t.Fatalf("%q 只是拖拽途中的中间状态，不应触发查询", partial)
		}
	}
	if !tr.observe("DSP Studio") {
		t.Fatal("选区停下来后应当触发一次查询")
	}
	if tr.observe("DSP Studio") {
		t.Fatal("同一次选区不应重复查询")
	}
}

func TestTrackerAsksAgainAfterTheHighlightClears(t *testing.T) {
	var tr tracker
	tr.observe("敏捷的棕色狐狸")
	if !tr.observe("敏捷的棕色狐狸") {
		t.Fatal("稳定的选区应当触发")
	}
	tr.observe("")
	tr.observe("敏捷的棕色狐狸")
	if !tr.observe("敏捷的棕色狐狸") {
		t.Fatal("清空后重新选中同样的文字应当再问一次")
	}
}

func TestTrackerReportsAnExtendedSelection(t *testing.T) {
	var tr tracker
	tr.observe("quick brown")
	tr.observe("quick brown")
	// The selection grew; the first look at the longer text only records it.
	if tr.observe("quick brown fox") {
		t.Fatal("选区刚变化时不应触发")
	}
	if !tr.observe("quick brown fox") {
		t.Fatal("延长后的选区稳定下来应当触发")
	}
}
