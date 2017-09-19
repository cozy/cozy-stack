// Cozy Cloud is a personal platform as a service with a focus on data.
// Cozy Cloud can be seen as 4 layers, from inside to outside:
//
// 1. A place to keep your personal data
//
// 2. A core API to handle the data
//
// 3. Your web apps, and also the mobile & desktop clients
//
// 4. A coherent User Experience
//
// It's also a set of values: Simple, Versatile, Yours. These values mean a lot
// for Cozy Cloud in all aspects. From an architectural point, it declines to:
//
// - Simple to deploy and understand, not built as a galaxy of optimized
// microservices managed by kubernetes that only experts can debug.
//
// - Versatile, can be hosted on a Raspberry Pi for geeks to massive scale on
// multiple servers by specialized hosting. Users can install apps.
//
// - Yours, you own your data and you control it. If you want to take back your
// data to go elsewhere, you can.
package main

import (
	"fmt"
	"os"

	"github.com/cozy/cozy-stack/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		if err != cmd.ErrUsage {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error()) // #nosec
			os.Exit(1)
		}
	}
}
