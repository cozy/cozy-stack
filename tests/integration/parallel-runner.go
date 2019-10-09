package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var shuffle bool
var failFast bool
var nb int

func main() {
	flag.BoolVar(&shuffle, "shuffle", false, "Randomize the order of the tests")
	flag.BoolVar(&failFast, "fail-fast", false, "Stop on the first test that fails")
	flag.IntVar(&nb, "n", 4, "Number of tests to run in parallel")
	flag.Parse()

	tests, err := listTests()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	if err := runTests(tests); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func listTests() ([]string, error) {
	tests, err := filepath.Glob("tests/*.rb")
	if err != nil {
		return nil, err
	}
	if shuffle {
		rand.Shuffle(len(tests), func(i, j int) {
			tests[i], tests[j] = tests[j], tests[i]
		})
	}
	return tests, nil
}

type result struct {
	Test string
	Err  error
	Out  []byte
}

func runTests(tests []string) error {
	results := make(chan result, len(tests))
	tokens := make(chan struct{}, nb)
	for i := 0; i < nb; i++ {
		tokens <- struct{}{}
	}

	for i, test := range tests {
		go func(i int, test string) {
			k := <-tokens
			cmd := exec.Command("bundle", "exec", "ruby", test)
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("COZY_BASE_PORT=%d", 8081+10*i),
				"CI=true")
			out, err := cmd.CombinedOutput()
			results <- result{test, err, out}
			tokens <- k
		}(i, test)

		// Starting all the tests at the same time is not a good idea, the
		// stack can create conflicts on the global databases.
		time.Sleep(3 * time.Second)
	}

	var err error
	for range tests {
		res := <-results
		fmt.Printf("\n==== Run %s ====\n%s\n", res.Test, res.Out)
		if res.Err != nil {
			err = res.Err
			if failFast {
				return err
			}
		}
	}

	return err
}
