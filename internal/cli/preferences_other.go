//go:build !linux

package cli

import "context"

type unsupportedPreferences struct{}

func newPreferences() preferenceStore                { return unsupportedPreferences{} }
func (unsupportedPreferences) Read() (string, error) { return "", nil }
func (unsupportedPreferences) Save(context.Context, string, bool) (string, error) {
	return "", cliError("preference_readonly")
}
func preferenceReadCode(error) string { return "preference_invalid" }
