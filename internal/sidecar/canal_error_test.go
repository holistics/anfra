package sidecar

import "testing"

// canal-query's error object is read into a typed error, whatever it says.
func TestCanalError(t *testing.T) {
	e := canalError(map[string]any{"message": "run query: boom", "scope": "User", "type": "GenericError"})
	if e.Message != "run query: boom" || e.Scope != "User" || e.Type != "GenericError" {
		t.Errorf("read %+v", e)
	}
	if e := canalError(map[string]any{"code": 7}); e.Message == "" {
		t.Error("an error object with no message reads as an empty message")
	}
}
