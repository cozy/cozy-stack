# Tools for testing sharings

## Install

```sh
apt install ruby
cd $(go env GOPATH)/src/github.com/cozy/cozy-stack
go install
cd tests/sharing
bundle install
```

Optional: install `MailHog`


## Interactive mode

It's possible to do manual tests and to use the tools in an interactive mode
to setup the sharing, create and update documents, etc.

```sh
bundle exec ./console.rb
```


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
