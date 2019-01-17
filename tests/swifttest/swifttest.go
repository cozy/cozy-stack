// This script can be used to start a Swift-like server that keeps in memory
// its files. It can be stared with `go run ./tests/swifttest`. The username
// and API key to use are both 'swiftest'.

package main

import (
	"fmt"
	"net/url"
	"os"
	"os/signal"

	"github.com/ncw/swift/swifttest"
)

func main() {
	srv, err := swifttest.NewSwiftServer("localhost")
	if err != nil {
		panic(err)
	}
	defer srv.Close()

	u, err := url.Parse(srv.AuthURL)
	if err != nil {
		panic(err)
	}
	fmt.Printf("cozy-stack serve '--fs-url=swift://%s%s?UserName=swifttest&Password=swifttest&AuthURL=%s'\n",
		u.Host, u.Path, srv.AuthURL)

	// Wait for CTRL-C
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
