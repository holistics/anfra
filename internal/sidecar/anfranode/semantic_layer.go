package anfranode

import "context"

// SLResult is the generated AML and warnings from a semantic-layer operation.
type SLResult struct {
	ModelName string   `json:"modelName"`
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Warnings  []string `json:"warnings"`
	Changed   *bool    `json:"changed,omitempty"`
}

// SLAddRequest selects a Postgres table from the repo's catalog.
type SLAddRequest struct {
	Context    CatalogContext `json:"context"`
	Table      string         `json:"table"`
	DataSource string         `json:"dataSource"`
}

// SLAdd adds to the semantic layer by generating AML from catalog metadata.
func (c *Client) SLAdd(ctx context.Context, req SLAddRequest) (SLResult, error) {
	var result SLResult
	if err := req.Context.check(); err != nil {
		return result, err
	}

	err := c.Call(ctx, "sl.add", req, &result)
	return result, err
}

// SLUpdateRequest identifies an existing table model, using dots or :: in its FQN.
type SLUpdateRequest struct {
	Context   CatalogContext `json:"context"`
	ModelName string         `json:"modelName"`
}

// SLUpdate updates the semantic layer by generating AML without writing the repo.
func (c *Client) SLUpdate(ctx context.Context, req SLUpdateRequest) (SLResult, error) {
	var result SLResult
	if err := req.Context.check(); err != nil {
		return result, err
	}

	err := c.Call(ctx, "sl.update", req, &result)
	return result, err
}
