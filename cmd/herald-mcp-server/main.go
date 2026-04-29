// herald-mcp-server is a compatibility wrapper for `herald mcp`.
package main

import (
	"log"
	"os"

	"mail-processor/internal/mcpserver"
)

func main() {
	if err := mcpserver.Run("herald-mcp-server", os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
