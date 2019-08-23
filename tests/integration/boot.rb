require 'awesome_print'
require 'base64'
require 'date'
require 'digest'
require 'faker'
require 'fileutils'
require 'mimemagic'
require 'json'
require 'pbkdf2'
require 'pry'
require 'rest-client'

AwesomePrint.pry!
Pry.config.history.file = File.expand_path "../tmp/.pry_history", __FILE__

base = File.expand_path "..", __FILE__
FileUtils.cd base do
  Faker::Config.locale = :fr
  FileUtils.mkdir_p "tmp/"
  require_relative "lib/test/timeout.rb" if ENV['CI']
  require_relative "lib/model.rb"
  Dir["lib/*"].each do |f|
    require_relative f if File.file? f
  end
  Helpers.setup
  Helpers.cleanup
end
