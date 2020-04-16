#compdef _cozy-stack cozy-stack


function _cozy-stack {
  local -a commands

  _arguments -C \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "apps:Interact with the applications"
      "assets:Show and manage dynamic assets"
      "bug:start a bug report"
      "completion:Output shell completion code for the specified shell"
      "config:Show and manage configuration elements"
      "doc:Print the documentation"
      "features:Manage the feature flags"
      "files:Interact with the cozy filesystem"
      "fixer:A set of tools to fix issues or migrate content for retro-compatibility."
      "help:Help about any command"
      "instances:Manage instances of a stack"
      "jobs:Launch and manage jobs and workers"
      "konnectors:Interact with the konnectors"
      "serve:Starts the stack and listens for HTTP calls"
      "settings:Display and update settings"
      "status:Check if the HTTP server is running"
      "swift:Interact directly with OpenStack Swift object storage"
      "triggers:Interact with the triggers"
      "version:Print the version number"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  apps)
    _cozy-stack_apps
    ;;
  assets)
    _cozy-stack_assets
    ;;
  bug)
    _cozy-stack_bug
    ;;
  completion)
    _cozy-stack_completion
    ;;
  config)
    _cozy-stack_config
    ;;
  doc)
    _cozy-stack_doc
    ;;
  features)
    _cozy-stack_features
    ;;
  files)
    _cozy-stack_files
    ;;
  fixer)
    _cozy-stack_fixer
    ;;
  help)
    _cozy-stack_help
    ;;
  instances)
    _cozy-stack_instances
    ;;
  jobs)
    _cozy-stack_jobs
    ;;
  konnectors)
    _cozy-stack_konnectors
    ;;
  serve)
    _cozy-stack_serve
    ;;
  settings)
    _cozy-stack_settings
    ;;
  status)
    _cozy-stack_status
    ;;
  swift)
    _cozy-stack_swift
    ;;
  triggers)
    _cozy-stack_triggers
    ;;
  version)
    _cozy-stack_version
    ;;
  esac
}


function _cozy-stack_apps {
  local -a commands

  _arguments -C \
    '--all-domains[work on all domains iteratively]' \
    '--domain[specify the domain name of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "install:Install an application with the specified slug name
from the given source URL."
      "ls:List the installed applications."
      "show:Show the application attributes"
      "uninstall:Uninstall the application with the specified slug name."
      "update:Update the application with the specified slug name."
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  install)
    _cozy-stack_apps_install
    ;;
  ls)
    _cozy-stack_apps_ls
    ;;
  show)
    _cozy-stack_apps_show
    ;;
  uninstall)
    _cozy-stack_apps_uninstall
    ;;
  update)
    _cozy-stack_apps_update
    ;;
  esac
}

function _cozy-stack_apps_install {
  _arguments \
    '--ask-permissions[specify that the application should not be activated after installation]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_apps_ls {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_apps_show {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_apps_uninstall {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_apps_update {
  _arguments \
    '--safe[do not upgrade if there are blocking changes]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_assets {
  local -a commands

  _arguments -C \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "add:Insert a dynamic asset"
      "ls:List assets"
      "rm:Removes an asset"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  add)
    _cozy-stack_assets_add
    ;;
  ls)
    _cozy-stack_assets_ls
    ;;
  rm)
    _cozy-stack_assets_rm
    ;;
  esac
}

function _cozy-stack_assets_add {
  _arguments \
    '--context[The context of the asset]:' \
    '--name[The name of the asset]:' \
    '--shasum[The shasum of the asset]:' \
    '--url[The URL of the asset]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_assets_ls {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_assets_rm {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_bug {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_completion {
  _arguments \
    '(-h --help)'{-h,--help}'[help for completion]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    '1: :("bash" "zsh" "fish")'
}


function _cozy-stack_config {
  local -a commands

  _arguments -C \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "decrypt-creds:Decrypt the given credentials cipher text with the specified decryption keyfile."
      "decrypt-data:Decrypt data with the specified decryption keyfile."
      "encrypt-creds:Encrypt the given credentials with the specified decryption keyfile."
      "encrypt-data:Encrypt data with the specified encryption keyfile."
      "gen-keys:Generate an key pair for encryption and decryption of credentials"
      "insert-asset:Inserts an asset"
      "ls-assets:List assets"
      "ls-contexts:List contexts"
      "passwd:Generate an admin passphrase"
      "rm-asset:Removes an asset"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  decrypt-creds)
    _cozy-stack_config_decrypt-creds
    ;;
  decrypt-data)
    _cozy-stack_config_decrypt-data
    ;;
  encrypt-creds)
    _cozy-stack_config_encrypt-creds
    ;;
  encrypt-data)
    _cozy-stack_config_encrypt-data
    ;;
  gen-keys)
    _cozy-stack_config_gen-keys
    ;;
  insert-asset)
    _cozy-stack_config_insert-asset
    ;;
  ls-assets)
    _cozy-stack_config_ls-assets
    ;;
  ls-contexts)
    _cozy-stack_config_ls-contexts
    ;;
  passwd)
    _cozy-stack_config_passwd
    ;;
  rm-asset)
    _cozy-stack_config_rm-asset
    ;;
  esac
}

function _cozy-stack_config_decrypt-creds {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_decrypt-data {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_encrypt-creds {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_encrypt-data {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_gen-keys {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_insert-asset {
  _arguments \
    '--context[The context of the asset]:' \
    '--name[The name of the asset]:' \
    '--shasum[The shasum of the asset]:' \
    '--url[The URL of the asset]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_ls-assets {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_ls-contexts {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_passwd {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_config_rm-asset {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_doc {
  local -a commands

  _arguments -C \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "man:Print the manpages of cozy-stack"
      "markdown:Print the documentation of cozy-stack as markdown"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  man)
    _cozy-stack_doc_man
    ;;
  markdown)
    _cozy-stack_doc_markdown
    ;;
  esac
}

function _cozy-stack_doc_man {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_doc_markdown {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_features {
  local -a commands

  _arguments -C \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "config:Display the feature flags from configuration for a context"
      "defaults:Display and update the default values for feature flags"
      "flags:Display and update the feature flags for an instance"
      "ratio:Display and update the feature flags for a context"
      "sets:Display and update the feature sets for an instance"
      "show:Display the computed feature flags for an instance"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  config)
    _cozy-stack_features_config
    ;;
  defaults)
    _cozy-stack_features_defaults
    ;;
  flags)
    _cozy-stack_features_flags
    ;;
  ratio)
    _cozy-stack_features_ratio
    ;;
  sets)
    _cozy-stack_features_sets
    ;;
  show)
    _cozy-stack_features_show
    ;;
  esac
}

function _cozy-stack_features_config {
  _arguments \
    '--context[The context for the feature flags]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_features_defaults {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_features_flags {
  _arguments \
    '--domain[Specify the domain name of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_features_ratio {
  _arguments \
    '--context[The context for the feature flags]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_features_sets {
  _arguments \
    '--domain[Specify the domain name of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_features_show {
  _arguments \
    '--domain[Specify the domain name of the instance]:' \
    '--source[Show the sources of the feature flags]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_files {
  local -a commands

  _arguments -C \
    '--domain[specify the domain name of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "exec:Execute the given command on the specified domain and leave"
      "import:Import the specified file or directory into cozy"
      "usage:Show the usage and quota for the files of this instance"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  exec)
    _cozy-stack_files_exec
    ;;
  import)
    _cozy-stack_files_import
    ;;
  usage)
    _cozy-stack_files_usage
    ;;
  esac
}

function _cozy-stack_files_exec {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_files_import {
  _arguments \
    '--dry-run[do not actually import the files]' \
    '--from[directory to import from in cozy]:' \
    '--match[pattern that the imported files must match]:' \
    '--to[directory to import to in cozy]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_files_usage {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_fixer {
  local -a commands

  _arguments -C \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "contact-emails:Detect and try to fix invalid emails on contacts"
      "content-mismatch:Fix the content mismatch differences for 64K issue"
      "indexes:Rebuild the CouchDB views and indexes"
      "jobs:Take a look at the consistency of the jobs"
      "md5:Fix missing md5 from contents in the vfs"
      "mime:Fix the class computed from the mime-type"
      "orphan-account:Remove the orphan accounts"
      "redis:Rebuild scheduling data strucutures in redis"
      "thumbnails:Rebuild thumbnails image for images files"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  contact-emails)
    _cozy-stack_fixer_contact-emails
    ;;
  content-mismatch)
    _cozy-stack_fixer_content-mismatch
    ;;
  indexes)
    _cozy-stack_fixer_indexes
    ;;
  jobs)
    _cozy-stack_fixer_jobs
    ;;
  md5)
    _cozy-stack_fixer_md5
    ;;
  mime)
    _cozy-stack_fixer_mime
    ;;
  orphan-account)
    _cozy-stack_fixer_orphan-account
    ;;
  redis)
    _cozy-stack_fixer_redis
    ;;
  thumbnails)
    _cozy-stack_fixer_thumbnails
    ;;
  esac
}

function _cozy-stack_fixer_contact-emails {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_fixer_content-mismatch {
  _arguments \
    '--no-dry-run[Do not dry run]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_fixer_indexes {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_fixer_jobs {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_fixer_md5 {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_fixer_mime {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_fixer_orphan-account {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_fixer_redis {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_fixer_thumbnails {
  _arguments \
    '--dry-run[Dry run]' \
    '--with-metadata[Recalculate images metadata]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_help {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_instances {
  local -a commands

  _arguments -C \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "add:Manage instances of a stack"
      "auth-mode:Set instance auth-mode"
      "client-oauth:Register a new OAuth client"
      "debug:Activate or deactivate debugging of the instance"
      "destroy:Remove instance"
      "export:Export an instance to a tarball"
      "find-oauth-client:Find an OAuth client"
      "fsck:Check a vfs"
      "import:Import a tarball"
      "ls:List instances"
      "modify:Modify the instance properties"
      "refresh-token-oauth:Generate a new OAuth refresh token"
      "set-disk-quota:Change the disk-quota of the instance"
      "set-passphrase:Change the passphrase of the instance"
      "show:Show the instance of the specified domain"
      "show-app-version:Show instances that have a particular app version"
      "show-db-prefix:Show the instance DB prefix of the specified domain"
      "show-swift-prefix:Show the instance swift prefix of the specified domain"
      "token-app:Generate a new application token"
      "token-cli:Generate a new CLI access token (global access)"
      "token-konnector:Generate a new konnector token"
      "token-oauth:Generate a new OAuth access token"
      "update:Start the updates for the specified domain instance."
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  add)
    _cozy-stack_instances_add
    ;;
  auth-mode)
    _cozy-stack_instances_auth-mode
    ;;
  client-oauth)
    _cozy-stack_instances_client-oauth
    ;;
  debug)
    _cozy-stack_instances_debug
    ;;
  destroy)
    _cozy-stack_instances_destroy
    ;;
  export)
    _cozy-stack_instances_export
    ;;
  find-oauth-client)
    _cozy-stack_instances_find-oauth-client
    ;;
  fsck)
    _cozy-stack_instances_fsck
    ;;
  import)
    _cozy-stack_instances_import
    ;;
  ls)
    _cozy-stack_instances_ls
    ;;
  modify)
    _cozy-stack_instances_modify
    ;;
  refresh-token-oauth)
    _cozy-stack_instances_refresh-token-oauth
    ;;
  set-disk-quota)
    _cozy-stack_instances_set-disk-quota
    ;;
  set-passphrase)
    _cozy-stack_instances_set-passphrase
    ;;
  show)
    _cozy-stack_instances_show
    ;;
  show-app-version)
    _cozy-stack_instances_show-app-version
    ;;
  show-db-prefix)
    _cozy-stack_instances_show-db-prefix
    ;;
  show-swift-prefix)
    _cozy-stack_instances_show-swift-prefix
    ;;
  token-app)
    _cozy-stack_instances_token-app
    ;;
  token-cli)
    _cozy-stack_instances_token-cli
    ;;
  token-konnector)
    _cozy-stack_instances_token-konnector
    ;;
  token-oauth)
    _cozy-stack_instances_token-oauth
    ;;
  update)
    _cozy-stack_instances_update
    ;;
  esac
}

function _cozy-stack_instances_add {
  _arguments \
    '*--apps[Apps to be preinstalled]:' \
    '--context-name[Context name of the instance]:' \
    '--dev[To create a development instance (deprecated)]' \
    '--disk-quota[The quota allowed to the instance'\''s VFS]:' \
    '*--domain-aliases[Specify one or more aliases domain for the instance (separated by '\'','\'')]:' \
    '--email[The email of the owner]:' \
    '--locale[Locale of the new cozy instance]:' \
    '--passphrase[Register the instance with this passphrase (useful for tests)]:' \
    '--public-name[The public name of the owner]:' \
    '--settings[A list of settings (eg context:foo,offer:premium)]:' \
    '--swift-layout[Specify the layout to use for Swift (from 0 for layout V1 to 2 for layout V3, -1 means the default)]:' \
    '--tos[The TOS version signed]:' \
    '--tz[The timezone for the user]:' \
    '--uuid[The UUID of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_auth-mode {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_client-oauth {
  _arguments \
    '--allow-login-scope[Allow login scope]' \
    '--json[Output more informations in JSON format]' \
    '--onboarding-app[Specify an OnboardingApp]:' \
    '--onboarding-permissions[Specify an OnboardingPermissions]:' \
    '--onboarding-secret[Specify an OnboardingSecret]:' \
    '--onboarding-state[Specify an OnboardingState]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_debug {
  _arguments \
    '--domain[Specify the domain name of the instance]:' \
    '--ttl[Specify how long the debug mode will last]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_destroy {
  _arguments \
    '--force[Force the deletion without asking for confirmation]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_export {
  _arguments \
    '--domain[Specify the domain name of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_find-oauth-client {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_fsck {
  _arguments \
    '--fail-fast[Stop the FSCK on the first error]' \
    '--files-consistency[Check the files consistency only (between CouchDB and Swift)]' \
    '--index-integrity[Check the index integrity only]' \
    '--json[Output more informations in JSON format]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_import {
  _arguments \
    '--directory[Put the imported files inside this directory]:' \
    '--domain[Specify the domain name of the instance]:' \
    '--increase-quota[Increase the disk quota if needed for importing all the files]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_ls {
  _arguments \
    '--available-fields[List available fields for --fields option]' \
    '*--fields[Arguments shown for each line in the list]:' \
    '--json[Show each line as a json representation of the instance]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_modify {
  _arguments \
    '--blocked[Block the instance]' \
    '--context-name[New context name]:' \
    '--deleting[Set (or remove) the deleting flag]' \
    '--disk-quota[Specify a new disk quota]:' \
    '*--domain-aliases[Specify one or more aliases domain for the instance (separated by '\'','\'')]:' \
    '--email[New email]:' \
    '--locale[New locale]:' \
    '--onboarding-finished[Force the finishing of the onboarding]' \
    '--public-name[New public name]:' \
    '--settings[New list of settings (eg offer:premium)]:' \
    '--tos[Update the TOS version signed]:' \
    '--tos-latest[Update the latest TOS version]:' \
    '--tz[New timezone]:' \
    '--uuid[New UUID]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_refresh-token-oauth {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_set-disk-quota {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_set-passphrase {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_show {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_show-app-version {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_show-db-prefix {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_show-swift-prefix {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_token-app {
  _arguments \
    '--expire[Make the token expires in this amount of time]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_token-cli {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_token-konnector {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_token-oauth {
  _arguments \
    '--expire[Make the token expires in this amount of time, as a duration string, e.g. "1h"]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_instances_update {
  _arguments \
    '--all-domains[Work on all domains iteratively]' \
    '--context-name[Work only on the instances with the given context name]:' \
    '--domain[Specify the domain name of the instance]:' \
    '--force-registry[Force to update all applications sources from git to the registry]' \
    '--only-registry[Only update applications installed from the registry]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_jobs {
  local -a commands

  _arguments -C \
    '--domain[specify the domain name of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "purge-old-jobs:Purge old jobs from an instance"
      "run:"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  purge-old-jobs)
    _cozy-stack_jobs_purge-old-jobs
    ;;
  run)
    _cozy-stack_jobs_run
    ;;
  esac
}

function _cozy-stack_jobs_purge-old-jobs {
  _arguments \
    '--duration[duration to look for (ie. 3D, 2M)]:' \
    '*--workers[worker types to iterate over (all workers by default)]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_jobs_run {
  _arguments \
    '--json[specify the job arguments as raw JSON]:' \
    '--logs[print jobs log in stdout]' \
    '--logs-verbose[verbose logging (with --logs flag)]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_konnectors {
  local -a commands

  _arguments -C \
    '--all-domains[work on all domains iteratively]' \
    '--domain[specify the domain name of the instance]:' \
    '--parameters[override the parameters of the installed konnector]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "install:Install a konnector with the specified slug name
from the given source URL."
      "ls:List the installed konnectors."
      "run:Run a konnector."
      "show:Show the application attributes"
      "uninstall:Uninstall the konnector with the specified slug name."
      "update:Update the konnector with the specified slug name."
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  install)
    _cozy-stack_konnectors_install
    ;;
  ls)
    _cozy-stack_konnectors_ls
    ;;
  run)
    _cozy-stack_konnectors_run
    ;;
  show)
    _cozy-stack_konnectors_show
    ;;
  uninstall)
    _cozy-stack_konnectors_uninstall
    ;;
  update)
    _cozy-stack_konnectors_update
    ;;
  esac
}

function _cozy-stack_konnectors_install {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '--parameters[override the parameters of the installed konnector]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_konnectors_ls {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '--parameters[override the parameters of the installed konnector]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_konnectors_run {
  _arguments \
    '--account-id[specify the account ID to use for running the konnector]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '--parameters[override the parameters of the installed konnector]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_konnectors_show {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '--parameters[override the parameters of the installed konnector]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_konnectors_uninstall {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '--parameters[override the parameters of the installed konnector]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_konnectors_update {
  _arguments \
    '--safe[do not upgrade if there are blocking changes]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '--all-domains[work on all domains iteratively]' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '--parameters[override the parameters of the installed konnector]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_serve {
  _arguments \
    '--allow-root[Allow to start as root (disabled by default)]' \
    '*--appdir[Mount a directory as the '\''app'\'' application]:' \
    '--assets[path to the directory with the assets (use the packed assets by default)]:' \
    '--couchdb-url[CouchDB URL]:' \
    '--csp-whitelist[Whitelisted domains for the default allowed origins of the Content Secury Policy]:' \
    '--dev[Allow to run in dev mode for a prod release (disabled by default)]' \
    '--disable-csp[Disable the Content Security Policy (only available for development)]' \
    '--doctypes[path to the directory with the doctypes (for developing/testing a remote doctype)]:' \
    '--downloads-url[URL for the download secret storage, redis or in-memory]:' \
    '--fs-default-layout[Default layout for Swift (2 for layout v3)]:' \
    '--fs-url[filesystem url]:' \
    '--geodb[define the location of the database for IP -> City lookups]:' \
    '--hooks[define the directory used for hook scripts]:' \
    '--jobs-url[URL for the jobs system synchronization, redis or in-memory]:' \
    '--konnectors-cmd[konnectors command to be executed]:' \
    '--konnectors-oauthstate[URL for the storage of OAuth state for konnectors, redis or in-memory]:' \
    '--lock-url[URL for the locks, redis or in-memory]:' \
    '--log-level[define the log level]:' \
    '--log-syslog[use the local syslog for logging]' \
    '--mail-alert-address[mail address used for alerts (instance deletion failure for example)]:' \
    '--mail-disable-tls[disable smtp over tls]' \
    '--mail-host[mail smtp host]:' \
    '--mail-noreply-address[mail address used for sending mail as a noreply (forgot passwords for example)]:' \
    '--mail-noreply-name[mail name used for sending mail as a noreply (forgot passwords for example)]:' \
    '--mail-password[mail smtp password]:' \
    '--mail-port[mail smtp port]:' \
    '--mail-username[mail smtp username]:' \
    '--mailhog[Alias of --mail-disable-tls --mail-port 1025, useful for MailHog]' \
    '--password-reset-interval[minimal duration between two password reset]:' \
    '--rate-limiting-url[URL for rate-limiting counters, redis or in-memory]:' \
    '--realtime-url[URL for realtime in the browser via webocket, redis or in-memory]:' \
    '--sessions-url[URL for the sessions storage, redis or in-memory]:' \
    '--subdomains[how to structure the subdomains for apps (can be nested or flat)]:' \
    '--vault-decryptor-key[the path to the key used to decrypt credentials]:' \
    '--vault-encryptor-key[the path to the key used to encrypt credentials]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_settings {
  _arguments \
    '--domain[specify the domain name of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_status {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_swift {
  local -a commands

  _arguments -C \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "get:"
      "ls:"
      "ls-layouts:Count layouts by types (v1, v2a, v2b, v3a, v3b)"
      "put:"
      "rm:"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  get)
    _cozy-stack_swift_get
    ;;
  ls)
    _cozy-stack_swift_ls
    ;;
  ls-layouts)
    _cozy-stack_swift_ls-layouts
    ;;
  put)
    _cozy-stack_swift_put
    ;;
  rm)
    _cozy-stack_swift_rm
    ;;
  esac
}

function _cozy-stack_swift_get {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_swift_ls {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_swift_ls-layouts {
  _arguments \
    '--show-domains[Show the domains along the counter]' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_swift_put {
  _arguments \
    '--content-type[Specify a Content-Type for the created object]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_swift_rm {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}


function _cozy-stack_triggers {
  local -a commands

  _arguments -C \
    '--domain[specify the domain name of the instance]:' \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:' \
    "1: :->cmnds" \
    "*::arg:->args"

  case $state in
  cmnds)
    commands=(
      "launch:Creates a job from a specific trigger"
      "ls:List triggers"
      "show-from-app:Show the application triggers"
    )
    _describe "command" commands
    ;;
  esac

  case "$words[1]" in
  launch)
    _cozy-stack_triggers_launch
    ;;
  ls)
    _cozy-stack_triggers_ls
    ;;
  show-from-app)
    _cozy-stack_triggers_show-from-app
    ;;
  esac
}

function _cozy-stack_triggers_launch {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_triggers_ls {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_triggers_show-from-app {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--domain[specify the domain name of the instance]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

function _cozy-stack_version {
  _arguments \
    '--admin-host[administration server host]:' \
    '--admin-port[administration server port]:' \
    '(-c --config)'{-c,--config}'[configuration file (default "$HOME/.cozy.yaml")]:' \
    '--host[server host]:' \
    '(-p --port)'{-p,--port}'[server port]:'
}

