package publish

import (
	"fmt"

	"github.com/holistics/anfra/internal/cloudapi"
)

// ListResult is `publish --list`'s result: what would be published, and where
// (docs/designs/commands-and-api.md#cli-output).
type ListResult struct {
	// Message is the outcome, in a sentence.
	Message string `json:"message"`
	// Org is where it would be published; null when none is configured.
	Org   *OrgRef  `json:"org"`
	Files []string `json:"files" pretty:",tree"`
	// Hints are what to do next; absent when nothing is.
	Hints []string `json:"hints,omitempty"`
}

// Result is a publish's result: the revision it published, and what came of
// it.
type Result struct {
	// Message is the outcome, in a sentence.
	Message  string `json:"message"`
	Org      OrgRef `json:"org"`
	Revision string `json:"revision"`
	// Changed is false when these files were what the org had published.
	Changed bool `json:"changed"`
	// Replaced is the revision it replaced; absent when none was published.
	Replaced *cloudapi.Replaced `json:"replaced,omitempty"`
	// Files is how many files it published: publish --list lists them.
	Files int `json:"files"`
	// DataSourcesCreated are the data sources it added to the org, by name.
	DataSourcesCreated []string `json:"data_sources_created"`
	// Hints are what to do next; absent when nothing is.
	Hints []string `json:"hints,omitempty"`
}

// OrgRef is an org a publish goes to: its address, and its name when the
// login knows it.
type OrgRef struct {
	URL  string `json:"url"`
	Name string `json:"name,omitempty"`
}

// told is what a publish says beside its result: its warnings, each with a
// code, as it goes (Env.Warn), and its hints and the data sources it created,
// for its result.
type told struct {
	warn    func(code, message string)
	hints   []string
	created []string
}

func (t *told) Warn(code, format string, args ...any) {
	if t.warn != nil {
		t.warn(code, fmt.Sprintf(format, args...))
	}
}

func (t *told) Hint(format string, args ...any) {
	t.hints = append(t.hints, fmt.Sprintf(format, args...))
}
