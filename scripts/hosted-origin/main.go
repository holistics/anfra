// hosted-origin validates and canonicalizes CLI build configuration before linking.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/holistics/anfra/internal/hostedorigin"
)

func main() {
	required := flag.Bool("required", false, "require a hosted origin for a release build")
	flag.Parse()
	raw := os.Getenv("ANFRA_HOSTED_ORIGIN")
	if raw == "" && !*required {
		return
	}
	origin, err := hostedorigin.ParseHostedOrigin(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(origin)
}
