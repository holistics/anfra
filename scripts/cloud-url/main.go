// cloud-url validates the optional cloud servers before embedding them in a CLI build, and
// prints them in their canonical form. See cloudapi.ParseCloudServers for the format.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/holistics/anfra/internal/cloudapi"
)

func main() {
	servers, err := cloudapi.ParseCloudServers(os.Getenv("ANFRA_CLOUD_URL"))
	if err != nil {
		fail(err.Error())
	}
	for _, server := range servers {
		// The build passes the value to the linker in single quotes, which can't hold one.
		if strings.Contains(server.Label, "'") {
			fail(fmt.Sprintf("the label %q has a \"'\", which the build can't embed. Remove it from the label", server.Label))
		}
	}
	fmt.Println(cloudapi.FormatCloudServers(servers))
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "ANFRA_CLOUD_URL:", message)
	os.Exit(1)
}
