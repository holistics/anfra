package sl

import (
	"fmt"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
)

// Result is the generated AML and warnings from a semantic-layer operation.
// The core never writes it to the repo.
type Result struct {
	Message     string   `json:"message" doc:"what was generated or whether the model is already up to date"`
	ModelName   string   `json:"model_name" doc:"the model's fully qualified name"`
	Path        string   `json:"path" doc:"the AML file's path relative to the repo"`
	WarningList []string `json:"warnings" pretty:"-" doc:"source metadata or fields that need attention"`
	Changed     *bool    `json:"changed,omitempty" doc:"whether the content differs from the existing file. Present for sl.update."`
	Content     string   `json:"content" pretty:"-" doc:"the complete AML file content, validated in memory"`
	Hints       []string `json:"hints,omitempty" doc:"what to do next, if any"`
}

func newResult(file anfranode.SLResult, update bool) Result {
	message := fmt.Sprintf("Generated AML for model %q.", file.ModelName)
	if update {
		message = fmt.Sprintf("Generated updated AML for model %q.", file.ModelName)
		if file.Changed != nil && !*file.Changed {
			message = fmt.Sprintf("Model %q is up to date.", file.ModelName)
		}
	}
	warnings := file.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return Result{Message: message, ModelName: file.ModelName, Path: file.Path, WarningList: warnings, Changed: file.Changed, Content: file.Content}
}

// Warnings lets pretty output report conversion warnings on stderr.
func (r Result) Warnings() []command.AnswerWarning {
	warnings := make([]command.AnswerWarning, len(r.WarningList))
	for i, message := range r.WarningList {
		warnings[i] = command.AnswerWarning{Code: "sl_warning", Message: message}
	}
	return warnings
}
