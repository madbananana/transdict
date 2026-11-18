//go:build !darwin

package gui

// liveOverlay stays off where the transparent sheet has not been verified: the
// frozen-frame flow works there, and a see-through overlay that paints opaque
// would hide the very screen the user is selecting from.
const liveOverlay = false
