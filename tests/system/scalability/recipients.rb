require_relative './base_tests.rb'

MAX_RECIPIENTS = 50
N_FILES_INIT = 10

# This test creates a sharing with many recipients

# Create the instances
names = Array.new(MAX_RECIPIENTS + 1) { Faker::Internet.domain_word }

insts = create_instances names
inst_a = insts[0]

# Create the hierarchy to share
folder = Folder.create inst_a
create_hierarchy inst_a, folder, N_FILES_INIT

# Create the sharing
create_sharing insts, folder

# Check everything went well for each recipient
da = File.join Helpers.current_dir, inst_a.domain, folder.name
recipients = insts.drop 1
rec_path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
recipients.each do |recipient|
  drec = File.join Helpers.current_dir, recipient.domain, rec_path
  poll_for_diff da, drec
end
