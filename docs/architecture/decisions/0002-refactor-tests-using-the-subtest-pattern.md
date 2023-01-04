# 2. Refactor the tests using the subtest pattern

Date: 2023-01-04

## Status

Accepted

## Context

The current way of writing integration tests use the `TestMain` function as a way of initializing the 
tests. This bring several issues:

  - As explained in [#3622](https://github.com/cozy/cozy-stack/issues/3622) the only way to return an error
  catch in a main is by using `os.Exit`. This method as the desavantage of skipping the defer functions and
  so the test cleanup is not done properly.
  - You can have only one `TestMain` function by package, this force the use of the same instance for every
  tests.
  - We can't have access to the type `testing.T` which could be very usefull for many reasons.

## Decision

We move to the [Subtests](https://pkg.go.dev/testing#hdr-Subtests_and_Sub_benchmarks) pattern.

### Good

- It will scope the data inside the tests, making them easier to read.
- It will allow to clean the code correctly by applying the defer and using t.cleanup (exitAfterDefer issue #3622).
- It will allow to use the logger from the tests and printing only the log in error.
- It will allow you to use t.Tempdir which is automatically cleaned.
- It will allow you to use the test name (`t.Name`) for the instance naming, ensuring the name uniqueness.
- It will allow you to setup the `--short` flag.
- It will allow you to remove a lot of global variables and avoid possible dataraces.
- It could allow you to run the test in parallel soon.

### Bad

- All the lines indentation will be changed, making a lot of noise in the git history.

## Consequences

TODO
