package querycache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/holistics/anfra/shared/jsonkit"
)

// KeyInput is what a query's key is made of: anfra-node's key, and what the
// compiler doesn't see. Each field is either something that decides access,
// which the key guarantees, or something that changes the rows under the same
// access, which it holds as best it can. What changes neither (comments,
// timeouts, SSL, pooling, the result's maximums) is not here
// (docs/designs/query-cache.md#keys).
type KeyInput struct {
	// CompilerKey is anfra-node's key for an AQL query (v1:…): the compiled
	// query, data restrictions and user attributes resolved into it. Its
	// format version stands for anfra-node's version.
	CompilerKey string
	// SQL is a SQL query's text, which has no compiler key. Exactly one of
	// CompilerKey and SQL is set.
	SQL string

	// Tenant is whom the query runs for: tenants never share an entry.
	Tenant string
	// DataSource is the data source's identity: who the warehouse sees
	// (datasource.DataSource.Identity).
	DataSource string
	// DBType is the data source's type, the SQL dialect.
	DBType string
	// TruncateRows is the most rows the query answers, as canal-query stores
	// the rows it streamed (negative: all of them).
	TruncateRows int
}

// keyFormat prefixes every key. Bump it when what goes into the hash changes,
// so no key made before means something else now.
const keyFormat = "k1:"

// Why NewKey makes no key. Each is a mistake of whoever built the input, not
// the person running the query: the query runs uncached, and the log says
// which.
var (
	ErrNoQuery              = errors.New("the query cache's key needs a compiler key or a SQL text, and has neither")
	ErrTwoQueries           = errors.New("the query cache's key needs a compiler key or a SQL text, and has both")
	ErrNoTenant             = errors.New("the query cache's key needs the tenant the query runs for, and has none")
	ErrNoDataSourceIdentity = errors.New("the query cache's key needs the data source's identity, and the host gave none")
)

// NewKey is the key of the query in: the hash of every field, so that two
// inputs differing in any one have different keys.
func NewKey(in KeyInput) (Key, error) {
	switch {
	case in.CompilerKey == "" && in.SQL == "":
		return "", ErrNoQuery
	case in.CompilerKey != "" && in.SQL != "":
		return "", ErrTwoQueries
	case in.Tenant == "":
		return "", ErrNoTenant
	case in.DataSource == "":
		return "", ErrNoDataSourceIdentity
	}
	// Named fields, so a SQL text and a compiler key never hash alike.
	b, err := jsonkit.Marshal(struct {
		CompilerKey  string `json:"compiler_key,omitempty"`
		SQL          string `json:"sql,omitempty"`
		Tenant       string `json:"tenant"`
		DataSource   string `json:"data_source"`
		DBType       string `json:"dbtype"`
		TruncateRows int    `json:"truncate_rows"`
	}{in.CompilerKey, in.SQL, in.Tenant, in.DataSource, in.DBType, in.TruncateRows})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return Key(keyFormat + hex.EncodeToString(sum[:])), nil
}
