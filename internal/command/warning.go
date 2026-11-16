package command

import "github.com/holistics/anfra/internal/repo"

// AnswerWarning is a problem an answer reports beside its result, which didn't
// fail the request (docs/designs/errors.md): a code a program branches on, and
// a message.
type AnswerWarning struct {
	Code    string
	Message string
}

// Warner is an answer that reports warnings beside its result, such as a
// query's that ran without the cache it asked for. The CLI prints them on
// stderr when it doesn't show the answer as YAML or JSON (in the pretty
// format, or an export's file, downloaded), where they'd go unseen.
type Warner interface {
	Warnings() []AnswerWarning
}

// LogHint is what a failure's message ends with, so its reader can see why:
// for a local repo, whose log is on the reader's machine, the command that
// shows it; for a managed repo, whose host keeps its logs, nothing.
func LogHint(r repo.Repo) string {
	if _, ok := r.Local(); ok {
		return "Run `anfra logs` to see what failed."
	}
	return ""
}

// WithHint is message, then hint, when there is one.
func WithHint(message, hint string) string {
	if hint == "" {
		return message
	}
	return message + " " + hint
}
