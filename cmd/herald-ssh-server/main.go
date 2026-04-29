// herald-ssh-server is a compatibility wrapper for `herald ssh`.
package main

import (
	"log"
	"os"

	"mail-processor/internal/sshserver"
)

func main() {
	if err := sshserver.Run("herald-ssh-server", os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
