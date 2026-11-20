package anfranode

import "context"

// ModelFile is generated AML. Only the CLI writes it to the local repo.
type ModelFile struct {
	ModelName string   `json:"modelName"`
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Warnings  []string `json:"warnings"`
	Changed   *bool    `json:"changed,omitempty"`
}

// AddTableRequest selects a Postgres table from the repo's catalog.
type AddTableRequest struct {
	Context    CatalogContext `json:"context"`
	Table      string         `json:"table"`
	DataSource string         `json:"dataSource"`
}

// AddTable generates a table model from catalog metadata.
func (c *Client) AddTable(ctx context.Context, req AddTableRequest) (ModelFile, error) {
	var result ModelFile
	if err := req.Context.check(); err != nil {
		return result, err
	}
	err := c.Call(ctx, "sl.add", req, &result)
	return result, err
}

// UpdateModelRequest identifies an existing table model, using dots or :: in its FQN.
type UpdateModelRequest struct {
	Context   CatalogContext `json:"context"`
	ModelName string         `json:"modelName"`
}

// UpdateModel generates updated AML without writing the repo.
func (c *Client) UpdateModel(ctx context.Context, req UpdateModelRequest) (ModelFile, error) {
	var result ModelFile
	if err := req.Context.check(); err != nil {
		return result, err
	}
	err := c.Call(ctx, "sl.update", req, &result)
	return result, err
}
