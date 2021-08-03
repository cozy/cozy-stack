require 'amazing_print'
require 'base64'
require 'date'
require 'digest'
require 'faker'
require 'fileutils'
require 'mini_mime'
require 'json'
require 'open3'
require 'pbkdf2'
require 'pry'
require 'rest-client'
require 'uuid'

AmazingPrint.pry!
Pry.config.history_file = File.expand_path "tmp/.pry_history", __dir__

FileUtils.cd __dir__ do
  Faker::Config.locale = :fr
  FileUtils.mkdir_p "tmp/"
  require_relative "lib/test/timeout.rb" if ENV['CI']
  require_relative "lib/test/cat_logs.rb" if ENV['CI']
  require_relative "lib/model.rb"
  Dir["lib/*"].each do |f|
    require_relative f if File.file? f
  end
  Helpers.setup
  Helpers.cleanup unless ENV['CI']
end
