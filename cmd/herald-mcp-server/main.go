// herald-mcp-server is a compatibility wrapper for `herald mcp`.
package main

import (
	"log"
	"os"

	"github.com/herald-email/herald-mail-app/internal/mcpserver"
)

func main() {
	if err := mcpserver.Run("herald-mcp-server", os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
