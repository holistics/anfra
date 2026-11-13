// Package anfracanal is the anfra-canal binary: canal's services in one, each
// run with its name as the first argument (anfra-canal query). anfra runs it
// once per service, as that service's sidecar. It knows nothing of the
// services' settings, sockets or clients; their own packages do.
package anfracanal

import (
	"fmt"
	"os"
	"sync"

	"github.com/holistics/anfra/internal/sidecar"
)

// Name is the binary's name, in its errors and its unpacked file's.
const Name = "anfra-canal"

// BinEnv names an anfra-canal binary, for a build from source.
const BinEnv = "ANFRA_CANAL_BIN"

// Service is one of canal's services in anfra-canal.
type Service struct {
	// Mode is its argument to anfra-canal: "query".
	Mode string
	// Name is its name as a binary of its own: "canal-query".
	Name string
	// BinEnv names a binary of that service alone, for a build from source.
	BinEnv string
}

// Query is canal-query, the SQL execution engine.
var Query = Service{Mode: "query", Name: "canal-query", BinEnv: "ANFRA_CANAL_QUERY_BIN"}

// Lake is canal-lake, which stores query results as Parquet files.
var Lake = Service{Mode: "lake", Name: "canal-lake", BinEnv: "ANFRA_CANAL_LAKE_BIN"}

// Command is what runs a service: a binary, and its arguments.
type Command struct {
	Path string
	Args []string
}

// Command is what runs s. A release runs its embedded anfra-canal. A build from
// source embeds none, and runs s.BinEnv's binary, a service alone, or else
// ANFRA_CANAL_BIN's anfra-canal, which may run any service.
func (s Service) Command() (Command, error) {
	if len(embedded) > 0 {
		path, err := unpack()
		if err != nil {
			return Command{}, err
		}
		return Command{Path: path, Args: []string{s.Mode}}, nil
	}
	if path := os.Getenv(s.BinEnv); path != "" {
		return Command{Path: path}, nil
	}
	if path := os.Getenv(BinEnv); path != "" {
		return Command{Path: path, Args: []string{s.Mode}}, nil
	}
	return Command{}, fmt.Errorf("this build of anfra doesn't include %s. Set %s to the path of %s's binary, or %s to a %s's",
		Name, BinEnv, Name, s.BinEnv, s.Name)
}

// unpack writes the embedded binary to anfra's folder once per process, though
// several services start at once.
var unpack = sync.OnceValues(func() (string, error) {
	return sidecar.Unpack(Name, embedded)
})
