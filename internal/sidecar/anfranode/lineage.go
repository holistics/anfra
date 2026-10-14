package anfranode

import "context"

// CatalogLineageRequest mirrors anfra-node's catalog.lineage params: the
// catalog's context, the fields to trace, and the page of what each one needs.
type CatalogLineageRequest struct {
	Context CatalogContext  `json:"context"`
	Targets []LineageTarget `json:"targets"`
	Limit   int             `json:"limit,omitzero"`
	Offset  int             `json:"offset,omitzero"`
}

// LineageTarget is one field to trace: its parent, and its name in it.
type LineageTarget struct {
	Parent string `json:"parent" minLength:"1" doc:"the fqn of what holds the field: a model, or a dataset"`
	Child  string `json:"child" minLength:"1" doc:"the field's name in its parent: a dimension, a measure or a metric"`
}

// CatalogLineageResult is catalog.lineage's answer: one per target, in the
// order of the request's.
type CatalogLineageResult struct {
	Results []TargetDiagram `json:"results"`
}

// TargetDiagram is one target's lineage, or why it has none.
type TargetDiagram struct {
	Target  LineageTarget `json:"target"`
	Query   string        `json:"query"`
	Diagram *Diagram      `json:"diagram,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// CatalogLineage traces fields through the catalog of the request's context.
func (c *Client) CatalogLineage(ctx context.Context, req CatalogLineageRequest) (CatalogLineageResult, error) {
	var res CatalogLineageResult
	if err := req.Context.check(); err != nil {
		return res, err
	}
	err := c.Call(ctx, "catalog.lineage", req, &res)
	return res, err
}

// Diagram is a field's lineage, laid out to be drawn: cards (the entities drawn
// at the top level, each with its children nested in sections), in layers from
// upstream to downstream, and the links between them. The core API passes it
// through as anfra-node formats it.
type Diagram struct {
	Version int            `json:"version" enum:"2"`
	Query   string         `json:"query" doc:"the catalog search the lineage is"`
	Total   int            `json:"total" doc:"how many entities the search matched, over every page"`
	Matched []string       `json:"matched" doc:"the ids of this page's matched entities, in search order"`
	Schema  DiagramSchema  `json:"schema"`
	Layers  []DiagramLayer `json:"layers" doc:"the cards in layers, upstream first"`
	Cards   []DiagramCard  `json:"cards" doc:"every card, in layer order"`
}

// DiagramSchema is which entity types the diagram has, and how they connect,
// read from the diagram itself.
type DiagramSchema struct {
	Edges map[string]string            `json:"edges" doc:"edge type → what it means"`
	Types map[string]DiagramSchemaType `json:"types" doc:"entity type → how it connects"`
}

// DiagramSchemaType is how the entities of one type connect.
type DiagramSchemaType struct {
	TopLevel bool                `json:"top_level" doc:"no entity of the type is a child in a section"`
	Edges    map[string][]string `json:"edges" doc:"edge type → the entity types it links to"`
}

// DiagramLayer is one layer of cards.
type DiagramLayer struct {
	Index   int      `json:"index"`
	Types   []string `json:"types" doc:"the entity types of its cards"`
	CardIDs []string `json:"card_ids"`
}

// DiagramNode is an entity of the lineage, with its children.
type DiagramNode struct {
	ID       string           `json:"id" doc:"the catalog's id of the entity"`
	Type     string           `json:"type" doc:"the catalog's entity type, such as aml.model or aml.metric"`
	Label    string           `json:"label"`
	Ref      string           `json:"ref,omitempty" doc:"the entity this node is a copy of"`
	Tags     []string         `json:"tags,omitempty"`
	Props    map[string]any   `json:"props,omitempty" doc:"the entity's properties"`
	Links    []DiagramLink    `json:"links,omitempty" doc:"the node's links; a link is on its edge's from entity"`
	Sections []DiagramSection `json:"sections,omitempty" doc:"the node's children, grouped by type"`
}

// DiagramSection is a node's children of one type.
type DiagramSection struct {
	Type  string        `json:"type"`
	Nodes []DiagramNode `json:"nodes"`
}

// DiagramCard is a node drawn at the top level, in its layer.
type DiagramCard struct {
	DiagramNode
	Layer    int  `json:"layer"`
	Detached bool `json:"detached,omitzero" doc:"its type is not a top-level one, but its container is not in the lineage"`
}

// DiagramLink is an edge from a node.
type DiagramLink struct {
	Type  string         `json:"type" doc:"the edge type, such as refers or includes"`
	To    string         `json:"to" doc:"the linked entity's id"`
	Props map[string]any `json:"props,omitempty" doc:"the edge's properties, when it has some"`
}
