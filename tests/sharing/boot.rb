require 'awesome_print'
require 'faker'
require 'fileutils'
require 'rest-client'

base = File.expand_path "..", __FILE__
puts "dir = #{__dir__}"
FileUtils.cd base do
  FileUtils.mkdir_p "tmp/"
  Dir["tmp/*"].each do |f|
    ap "rm -r #{f}"
    FileUtils.rm_r f
  end

  Dir["lib/*"].each do |f|
    ap "require #{f}"
    require_relative f
  end
end
