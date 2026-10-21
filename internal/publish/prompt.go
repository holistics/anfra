package publish

import "github.com/holistics/anfra/internal/cliui"

// Prompt asks a question with choices, the last one the default (the safe
// answer), and returns the choice's index; ok is false when nobody can answer it
// (not a terminal).
type Prompt func(question string, choices ...string) (choice int, ok bool)

// Confirm asks a yes-or-no question, no the default.
func Confirm(ask Prompt, question string) (yes, ok bool) {
	choice, ok := ask(question, "yes", "no")
	return ok && choice == 0, ok
}

// asking is ask with p's pending section heading written first, so a question
// asked within a section is under its heading.
func asking(p *cliui.Printer, ask Prompt) Prompt {
	return func(question string, choices ...string) (int, bool) {
		p.Open()
		return ask(question, choices...)
	}
}
