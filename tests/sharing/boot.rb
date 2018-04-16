require 'awesome_print'
require 'faker'
require 'fileutils'
require 'json'
require 'rest-client'

base = File.expand_path "..", __FILE__
FileUtils.cd base do
  Faker::Config.locale = :fr
  FileUtils.mkdir_p "tmp/"
  Dir["lib/*"].each do |f|
    require_relative f
  end
  Helpers.setup
  Helpers.cleanup
end
