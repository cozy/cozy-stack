
require_relative './base_tests.rb'

N_SHARINGS = 100
MAX_RECIPIENTS = 5
N_FILES_INIT = 100

# This test creates several sharings with several recipients

# Create the instances
names = Array.new(MAX_RECIPIENTS + 1) { Faker::Internet.domain_word }

insts = create_instances names
inst_a = insts[0]
recipients = insts.drop 1

N_SHARINGS.times do |i|
  # Create the hierarchy to share
  folder = Folder.create inst_a, name: Faker::Internet.unique.slug
  create_hierarchy inst_a, folder, N_FILES_INIT

  # Randomly select the recipients
  n_rec = Random.rand(MAX_RECIPIENTS) + 1
  recs = recipients.sample(n_rec)
  members = [inst_a]
  members.concat recs

  # Share it
  create_sharing members, folder

  # Check everything went well for each recipient of this sharing
  da = File.join Helpers.current_dir, inst_a.domain, folder.name
  rec_path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
  recs.each do |recipient|
    drec = File.join Helpers.current_dir, recipient.domain, rec_path
    poll_for_diff da, drec
  end
  puts "OK for sharing #{i + 1}"
end
