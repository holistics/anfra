package anfranode

import "context"

// CatalogEntity is a semantic catalog record, with its source's properties.
type CatalogEntity struct {
	EntityID    string         `json:"entity_id" doc:"the entity's stable catalog identity"`
	Type        string         `json:"type" doc:"the catalog entity type"`
	Source      string         `json:"source" doc:"the source that supplied the entity"`
	DisplayName *string        `json:"display_name" nullable:"true" doc:"the entity's display name, when available"`
	Properties  map[string]any `json:"properties" doc:"the entity's source metadata"`
}

// AddTableRequest selects a Postgres table from the repo's catalog.
type AddTableRequest struct {
	Context    CatalogContext `json:"context"`
	Table      string         `json:"table"`
	DataSource string         `json:"dataSource"`
}

// AddTable returns the table's catalog entity. AML generation comes later.
func (c *Client) AddTable(ctx context.Context, req AddTableRequest) (CatalogEntity, error) {
	var result CatalogEntity
	if err := req.Context.check(); err != nil {
		return result, err
	}
	err := c.Call(ctx, "sl.add", req, &result)
	return result, err
}
