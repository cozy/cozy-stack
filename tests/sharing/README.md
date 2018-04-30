# Tools for testing sharings

## Install

```sh
apt install ruby ruby-dev
gem install bundle
cd $(go env GOPATH)/src/github.com/cozy/cozy-stack
go install
cd tests/sharing
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


## Logs

The log files for the stack are kept inside the `tmp/` directory. You can use
[lnav](http://lnav.org/) tool to view them. The log format of cozy-stack can
installed with:

```sh
lnav -i $(go env
GOPATH)/src/github.com/cozy/cozy-stack/scripts/lnav_cozy_log.json
```
