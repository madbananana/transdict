package gui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"transdict/internal/appdir"
)

// CaptureArea is 上一次截图区域: the frozen frame the rectangle came from and the
// crop in that frame's pixels. The frame index is part of it because a second
// display may have a different scale, so the rectangle cannot be reused on
// another display's image.
type CaptureArea struct {
	Frame int `json:"frame"`
	X     int `json:"x"`
	Y     int `json:"y"`
	W     int `json:"w"`
	H     int `json:"h"`
}

// lastAreaPath is a file beside config.json, not a setting: the area is
// machine state that changes with every screenshot, and it must not appear in
// an exported settings file.
func lastAreaPath() (string, error) {
	dir, err := appdir.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "last-capture.json"), nil
}

func readArea(path string) (CaptureArea, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CaptureArea{}, false
	}
	var a CaptureArea
	if err := json.Unmarshal(data, &a); err != nil || a.W <= 0 || a.H <= 0 {
		return CaptureArea{}, false
	}
	return a, true
}

func writeArea(path string, a CaptureArea) {
	data, err := json.Marshal(a)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

// LastCaptureArea hands the overlay the rectangle of 上一次截图区域, which the D
// key applies without asking for the same drag again. It answers only for the
// display the rectangle was taken on: pixel coordinates mean a different crop
// on a display with another scale.
func (s *Service) LastCaptureArea(frameIndex int) *CaptureArea {
	path, err := lastAreaPath()
	if err != nil {
		return nil
	}
	return pickArea(path, frameIndex)
}

func pickArea(path string, frameIndex int) *CaptureArea {
	a, ok := readArea(path)
	if !ok || a.Frame != frameIndex {
		return nil
	}
	return &a
}
