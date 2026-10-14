// cloud-url validates the optional cloud URL before embedding it in a CLI build.
package main

import (
	"fmt"
	"os"

	"github.com/holistics/anfra/internal/cloudapi"
)

func main() {
	raw := os.Getenv("ANFRA_CLOUD_URL")
	if raw == "" {
		return
	}
	origin, err := cloudapi.ParseOrigin(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ANFRA_CLOUD_URL:", err)
		os.Exit(1)
	}
	fmt.Println(origin)
}
