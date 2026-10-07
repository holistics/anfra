package anfranode

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// ShowRequest mirrors the sidecar's aml.show params: what to show, as the
// semantic catalog identifies it, an entity type and its fqn; both empty for
// the repo. DataSources are what compiling a dataset resolves its dialect from.
type ShowRequest struct {
	RepoPath string `json:"repoPath"`
	// RepoID is the compile-cache identity; see CompileToSQLRequest.RepoID.
	RepoID      string                       `json:"repoId,omitempty"`
	DataSources map[string]CompileDataSource `json:"dataSources"`
	Type        string                       `json:"type,omitempty"`
	Fqn         string                       `json:"fqn,omitempty"`
}

// ShowResult is aml.show's answer, which the core API passes through: the
// object shown, and the repo's files that do not compile.
type ShowResult struct {
	Object      ShowObject     `json:"object"`
	Diagnostics []CompileError `json:"diagnostics" doc:"the repo's files that do not compile; empty, what is shown is complete"`
}

// ShowObject is one object of the repo, tagged by its kind: the repo, or a
// dataset. Exactly one of its fields is set. More kinds come (a model, a
// field, a metric), so a client must accept a kind it does not know.
type ShowObject struct {
	Repo    *ShowRepo
	Dataset *ShowDataset
}

func (o ShowObject) MarshalJSON() ([]byte, error) {
	switch {
	case o.Repo != nil:
		return json.Marshal(o.Repo)
	case o.Dataset != nil:
		return json.Marshal(o.Dataset)
	}
	return nil, fmt.Errorf("anfranode: a ShowObject with nothing set")
}

func (o *ShowObject) UnmarshalJSON(b []byte) error {
	var tag struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(b, &tag); err != nil {
		return err
	}
	switch tag.Kind {
	case "repo":
		o.Repo = &ShowRepo{}
		return json.Unmarshal(b, o.Repo)
	case "dataset":
		o.Dataset = &ShowDataset{}
		return json.Unmarshal(b, o.Dataset)
	}
	return fmt.Errorf("anfranode: aml.show answered an object of kind %q, which this anfra does not know", tag.Kind)
}

// Schema publishes the union, discriminated by kind.
func (ShowObject) Schema(r huma.Registry) *huma.Schema {
	repo := r.Schema(reflect.TypeFor[ShowRepo](), true, "")
	dataset := r.Schema(reflect.TypeFor[ShowDataset](), true, "")
	return &huma.Schema{
		Description: "the object shown, by kind; a client must accept a kind it does not know",
		OneOf:       []*huma.Schema{repo, dataset},
		Discriminator: &huma.Discriminator{PropertyName: "kind", Mapping: map[string]string{
			"repo": repo.Ref, "dataset": dataset.Ref,
		}},
	}
}

// ShowRepo is the repo: the datasets it offers, in outline.
type ShowRepo struct {
	Kind     string        `json:"kind" enum:"repo"`
	Datasets []ShowDataset `json:"datasets" doc:"the repo's datasets, in outline"`
}

// ShowDataset is a dataset: in outline, who it is; in full, also its models
// and metrics. Its interface, not its implementation: what a caller needs to
// write a query against it or read a query's result, nothing of how it is
// built.
type ShowDataset struct {
	Kind        string      `json:"kind" enum:"dataset"`
	Fqn         string      `json:"fqn" doc:"the dataset's fully qualified name, as AQL and the catalog name it"`
	Name        string      `json:"name"`
	Label       string      `json:"label,omitempty"`
	Description string      `json:"description,omitempty"`
	Models      []ShowModel `json:"models,omitempty" doc:"the models the dataset uses; absent in outline"`
	Metrics     []ShowField `json:"metrics,omitempty" doc:"the dataset's metrics; absent in outline"`
}

// ShowModel is a model as a dataset uses it, with the fields AQL can
// reference on it.
type ShowModel struct {
	Fqn         string      `json:"fqn" doc:"the model's fully qualified name: what AQL writes before a field"`
	Name        string      `json:"name"`
	Label       string      `json:"label,omitempty"`
	Description string      `json:"description,omitempty"`
	Fields      []ShowField `json:"fields"`
}

// ShowField is a field or a metric.
type ShowField struct {
	Fqn              string `json:"fqn" doc:"the catalog's identity: <model fqn>.<name>, or <dataset fqn>.<name> for a dataset-level field or a metric"`
	Name             string `json:"name" doc:"AQL writes <model fqn>.<name>; a metric, its name alone"`
	Label            string `json:"label,omitempty"`
	Description      string `json:"description,omitempty"`
	Role             string `json:"role" enum:"dimension,measure,param,metric"`
	Type             string `json:"type" doc:"the compiled data type, such as number, text, date, datetime, truefalse"`
	Aggregation      string `json:"aggregation,omitempty" doc:"a measure's aggregation, such as sum or count distinct; custom for a custom one"`
	Hidden           bool   `json:"hidden"`
	Format           any    `json:"format,omitempty" doc:"how a value displays, as the AML declares it"`
	Definition       string `json:"definition,omitempty" doc:"the definition of a field or metric written in AQL"`
	DefinedInDataset bool   `json:"definedInDataset,omitempty" doc:"a dimension the dataset declares on one of its models"`
}

// ShowAML shows an object of the repo, read from its compiled AML.
func (c *Client) ShowAML(ctx context.Context, req ShowRequest) (ShowResult, error) {
	var res ShowResult
	if err := checkRepoID(req.RepoID); err != nil {
		return res, err
	}
	err := c.Call(ctx, "aml.show", req, &res)
	return res, err
}
