package publish

import (
	"fmt"

	"github.com/holistics/anfra/internal/cloudapi"
)

// CheckResult is `publish check`'s result: what would be published, and where
// (docs/designs/commands-and-api.md#cli-output).
type CheckResult struct {
	Message string   `json:"message" doc:"the outcome, in a sentence"`
	Org     *OrgRef  `json:"org" doc:"where it would be published. Null when no org is configured."`
	Files   []string `json:"files" doc:"the files it would send" pretty:",tree"`
	Hints   []string `json:"hints,omitempty" doc:"what to do next, if any"`
}

// Result is a publish's result: the revision it published, and what came of
// it.
type Result struct {
	Message            string             `json:"message" doc:"the outcome, in a sentence"`
	Org                OrgRef             `json:"org" doc:"where it was published"`
	Revision           string             `json:"revision" doc:"the revision published"`
	Changed            bool               `json:"changed" doc:"false when these files were what the org had published"`
	Replaced           *cloudapi.Replaced `json:"replaced,omitempty" doc:"the revision it replaced"`
	Files              int                `json:"files" doc:"how many files it published"`
	DataSourcesCreated []string           `json:"data_sources_created" doc:"the data sources it added to the org"`
	Hints              []string           `json:"hints,omitempty" doc:"what to do next, if any"`
}

// OrgRef is an org a publish goes to: its address, and its name when the
// login knows it.
type OrgRef struct {
	URL  string `json:"url" doc:"the org's address"`
	Name string `json:"name,omitempty" doc:"the org's name, from its saved login. Absent when it isn't known."`
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
