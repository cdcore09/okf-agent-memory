// Command derivekey prints the hex vault key derived from OKF_HUB_PASSWORD and
// OKF_HUB_SECRET_KEY, for piping straight into a secret store. It never takes
// secrets as arguments and prints nothing else.
package main

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/okf-memory/okf-agent-memory/pkg/vault"
)

func main() {
	key, err := vault.DeriveVaultKey(os.Getenv("OKF_HUB_PASSWORD"), os.Getenv("OKF_HUB_SECRET_KEY"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "derivekey:", err)
		os.Exit(1)
	}
	fmt.Print(hex.EncodeToString(key))
}
