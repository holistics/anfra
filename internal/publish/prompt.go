package publish

import "github.com/holistics/anfra/internal/cliui"

// Prompt asks a question with choices, the last one the default (the safe
// answer), and returns the choice's index; ok is false when nobody can answer it
// (not a terminal).
type Prompt func(question string, choices ...string) (choice int, ok bool)

// Input asks for a line of text, and returns it trimmed: "" when nothing was
// typed, or the question was interrupted; ok is false when nobody can answer it.
type Input func(question string) (answer string, ok bool)

// Confirm asks a yes-or-no question, no the default.
func Confirm(ask Prompt, question string) (yes, ok bool) {
	choice, ok := ask(question, "yes", "no")
	return ok && choice == 0, ok
}

// asking is ask with p's pending section heading written first, so a question
// asked within a section is under its heading. inputting is the same for Input.
func asking(p *cliui.Printer, ask Prompt) Prompt {
	return func(question string, choices ...string) (int, bool) {
		p.Open()
		return ask(question, choices...)
	}
}

func inputting(p *cliui.Printer, input Input) Input {
	return func(question string) (string, bool) {
		p.Open()
		return input(question)
	}
}
