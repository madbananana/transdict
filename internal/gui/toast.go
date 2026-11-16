package gui

// Toast reports a short outcome at the pointer. A card action such as 添加到
// Anki answers where the click happened, as EasyDict does, instead of growing a
// status line inside the window.
func (s *Service) Toast(text string) {
	ShowToastAtMouse(text)
}
