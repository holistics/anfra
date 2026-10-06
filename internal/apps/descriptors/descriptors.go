// Package descriptors builds the Anfra SDK's Dataset descriptors from anfra's catalog.
package descriptors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/holistics/anfra/internal/apps/anfra"
)

// Field is one model field, as the SDK's DatasetDescriptor has it.
type Field struct {
	Name            string `json:"name"`
	Label           string `json:"label"`
	Type            string `json:"type"`
	IsCustomMeasure bool   `json:"is_custom_measure"`
}

// Model is one model a Dataset joins.
type Model struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Label  string  `json:"label"`
	Fields []Field `json:"fields"`
}

// Metric is one dataset-level metric.
type Metric struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

// Dataset is the SDK's DatasetDescriptor.
type Dataset struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Label      string   `json:"label"`
	DataModels []Model  `json:"data_models"`
	Metrics    []Metric `json:"metrics"`
}

type entity struct {
	Properties map[string]any `json:"properties"`
}

func (e entity) str(key string) string {
	if s, ok := e.Properties[key].(string); ok {
		return s
	}
	return ""
}

func search(ctx context.Context, a *anfra.Anfra, query string) ([]entity, error) {
	_, data, err := a.Call(ctx, "search", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	var result struct {
		Total    int      `json:"total"`
		Entities []entity `json:"entities"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("anfra search %q: %w", query, err)
	}
	// Every entity of the type is needed; a truncated answer would silently drop fields.
	if result.Total > len(result.Entities) {
		return nil, fmt.Errorf("anfra search %q returned %d of %d entities", query, len(result.Entities), result.Total)
	}
	return result.Entities, nil
}

func or(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// Build reads every Dataset from anfra's catalog: the dataset, the models it joins, each model's
// dimensions and measures with their types, and its metrics. Deliberately one function, so a
// dedicated catalog tool can replace it later (issue 01).
func Build(ctx context.Context, a *anfra.Anfra) (map[string]Dataset, error) {
	if _, _, err := a.Call(ctx, "ingest", nil); err != nil {
		return nil, err
	}
	kinds := []string{"aml.dataset", "aml.dataset_model_alias", "aml.model", "aml.dimension", "aml.measure", "aml.metric"}
	found := map[string][]entity{}
	for _, kind := range kinds {
		entities, err := search(ctx, a, "type:"+kind)
		if err != nil {
			return nil, err
		}
		found[kind] = entities
	}

	modelLabel := map[string]string{}
	for _, m := range found["aml.model"] {
		modelLabel[m.str("fqn")] = m.str("label")
	}
	fieldsOf := func(modelFqn string) []Field {
		fields := []Field{}
		for _, kind := range []string{"aml.dimension", "aml.measure"} {
			for _, f := range found[kind] {
				if f.str("parent_fqn") != modelFqn {
					continue
				}
				fields = append(fields, Field{
					Name:            f.str("name"),
					Label:           or(f.str("label"), f.str("name")),
					Type:            or(f.str("data_type"), "unknown"),
					IsCustomMeasure: kind == "aml.measure",
				})
			}
		}
		return fields
	}

	out := map[string]Dataset{}
	for _, d := range found["aml.dataset"] {
		name := d.str("name")
		fqn := or(d.str("fqn"), name)
		ds := Dataset{ID: fqn, Name: name, Label: or(d.str("label"), name), DataModels: []Model{}, Metrics: []Metric{}}
		for _, alias := range found["aml.dataset_model_alias"] {
			if alias.str("dataset_fqn") != fqn {
				continue
			}
			modelFqn := alias.str("model_fqn")
			ds.DataModels = append(ds.DataModels, Model{
				ID: modelFqn, Name: modelFqn, Label: or(modelLabel[modelFqn], modelFqn), Fields: fieldsOf(modelFqn),
			})
		}
		for _, m := range found["aml.metric"] {
			if m.str("dataset_name") != name {
				continue
			}
			ds.Metrics = append(ds.Metrics, Metric{
				Name: m.str("name"), Label: or(m.str("label"), m.str("name")), Type: or(m.str("data_type"), "number"),
			})
		}
		out[name] = ds
	}
	return out, nil
}

// FieldType is the type of model.field in a Dataset, or "" when the Dataset doesn't define it.
func FieldType(datasets map[string]Dataset, dataset, model, field string) string {
	for _, m := range datasets[dataset].DataModels {
		if m.Name != model {
			continue
		}
		for _, f := range m.Fields {
			if f.Name == field {
				return f.Type
			}
		}
	}
	return ""
}
