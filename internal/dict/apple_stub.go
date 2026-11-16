//go:build !darwin

package dict

// The apple kind is the macOS Dictionary Services engine, which has no
// equivalent on Windows or Linux. Nothing is registered here on purpose: a
// provider that only ever errors would show up in the settings list as a
// service the user can enable and that then fails every lookup.

// IsEnglishWord has no system dictionary to ask off macOS, so a word is never
// known and 查询时自动分词 splits it when it looks like an identifier.
func IsEnglishWord(string) bool { return false }
