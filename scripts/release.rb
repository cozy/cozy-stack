#!/usr/bin/env ruby
require 'git'
require 'octokit'
require 'filemagic'

REPO = 'cozy/cozy-stack'
filemagic = FileMagic.new FileMagic::MAGIC_MIME
root_dir = File.dirname File.dirname File.expand_path __FILE__

git = Git.open root_dir
tag = git.describe '--tags'

token = ENV.fetch 'GITHUB_RELEASE_TOKEN'
github = Octokit::Client.new access_token: token
user = github.user
user.login

assets = Dir["cozy-stack*-#{tag}*"]

puts "Create release #{tag}"
release = github.create_release REPO, tag, name: tag
url = release.url
assets.each do |asset|
  puts "Upload #{asset}"
  mime = filemagic.file asset
  name = File.basename(asset).sub "-#{tag}", ''
  github.upload_asset url, asset, content_type: mime, name: name
end
