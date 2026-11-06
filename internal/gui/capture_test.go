package gui

import (
	"testing"

	"transdict/internal/capture"
)

// The crop kept for 遮挡后重识别 outlives the session it came from as soon as the
// user starts another capture. Pixelating a crop nobody asked for would then
// overwrite the new selection's image, so the stale request has to be refused.
func TestMaskWithoutACropIsRefused(t *testing.T) {
	err := (&Service{}).MaskAndRecognize([]capture.MaskRect{{X: 0, Y: 0, W: 4, H: 4}})
	if err == nil {
		t.Fatal("没有选区截图时应拒绝重识别")
	}
	if err.Error() != "截图已过期，请重新截取" {
		t.Errorf("得到 %q", err.Error())
	}
}
