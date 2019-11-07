# Tools for integration tests

## Install

```sh
apt install ruby ruby-dev
gem install bundle
cd cozy-stack
go install
cd tests/integration
bundle install
```

Optional: install `MailHog`


## Automated tests

To launch an automated scenario of tests:

```sh
bundle exec ruby tests/push_folder.rb -v
```

The tests clean the logs and databases from the previous runs when they start,
but does no cleaning on exit. You can inspect them to find what can have gone
wrong for example. If you just want to clean those because you have finish
your testing sessions, you can run this command:

```sh
bundle exec ruby clean.rb
```

You can also run the tests in parallel with:

```sh
go run parallel-runner.go -n 3 -fail-fast
```


## Interactive mode

It's possible to do manual tests and to use the tools in an interactive mode
to setup the sharing, create and update documents, etc.

```sh
bundle exec ./console.rb
```

Example of session:

```ruby
b = Bootstrap.push_folder
ap b.sharing
b.open
b.accept
b.recipients.first.open
```

## Swift

It's complicated to launch the tests with Swift, but it's possible to simulate
it with swifttest:

```sh
$ go run ./tests/swifttest
$ cd tests/integration
$ export COZY_SWIFTTEST=1
$ bundle exec ruby tests/sharing_push_folder.rb
```

## Logs

The log files for the stack are kept inside the `tmp/` directory. You can use
[lnav](http://lnav.org/) tool to view them. The log format of cozy-stack can
installed with:

```sh
lnav -i cozy-stack/scripts/lnav_cozy_log.json
```
