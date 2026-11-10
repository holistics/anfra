// Package anfracanal is the anfra-canal binary: one binary from holistics/canal that runs each canal service anfra
// needs, by its first argument (anfra-canal query). The services stay sidecars of their own, with their own settings,
// supervisors and clients (canalquery); this package only finds the binary they run.
package anfracanal

import (
	"fmt"
	"os"

	"github.com/holistics/anfra/internal/sidecar"
)

// Name is the binary's name, and its unpacked file's (<home>/sidecars/anfra-canal-<hash>).
const Name = "anfra-canal"

// BinEnv names the anfra-canal binary in a build that embeds none.
const BinEnv = "ANFRA_CANAL_BIN"

// Command is the binary and arguments that run a canal service, e.g. canal-query, in mode, e.g. query:
//   - a release: the embedded anfra-canal, with mode;
//   - a build from source: the service's own binary if serviceBinEnv (e.g. ANFRA_CANAL_QUERY_BIN) names one, a
//     standalone build of the service, run bare; else the anfra-canal BinEnv names, with mode.
func Command(service, serviceBinEnv, mode string) (path string, args []string, err error) {
	if len(embedded) > 0 {
		path, err = sidecar.Binary(Name, BinEnv, embedded)
		return path, []string{mode}, err
	}
	if path := os.Getenv(serviceBinEnv); path != "" {
		return path, nil, nil
	}
	if path := os.Getenv(BinEnv); path != "" {
		return path, []string{mode}, nil
	}
	return "", nil, fmt.Errorf("this build of anfra doesn't include %s. Set %s to the path of an anfra-canal binary, or %s to a %s one",
		service, BinEnv, serviceBinEnv, service)
}
