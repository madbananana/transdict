//go:build !darwin

package dict

// The apple kind is the macOS Dictionary Services engine, which has no
// equivalent on Windows or Linux. Nothing is registered here on purpose: a
// provider that only ever errors would show up in the settings list as a
// service the user can enable and that then fails every lookup.
