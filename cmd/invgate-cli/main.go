// Command invgate-cli is the runtime OpenAPI/Swagger CLI.
// main.go is a tiny shim: it builds the CLI via internal/cli.New
// and exits with the code returned by Execute.
package main

import (
	"os"

	"github.com/wdelcant/invgate-cli/internal/cli"
)

func main() {
	c := cli.New(os.Args[1:])
	os.Exit(c.Execute(os.Args[1:]))
}