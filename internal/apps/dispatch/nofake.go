//go:build !apps_e2e

package dispatch

// Fake is the canned-answer Caller of an e2e build (-tags apps_e2e); nil in any other build.
func Fake() Caller { return nil }
