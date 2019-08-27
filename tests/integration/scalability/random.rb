require_relative './base_tests.rb'

# This test creates random sharing scenarios

N_SHARINGS = 10
MAX_RECIPIENTS = 10
MAX_FILES_INIT = 100
MAX_UPDATES = 1000 # Includes CUD operations
FILE_MAX_SIZE = 1000 # In KB

# Create the instances
names = Array.new(MAX_RECIPIENTS + 1) { Faker::Internet.domain_word }
insts = create_instances names
inst_a = insts[0]
recipients = insts.drop 1

sids = Array.new(N_SHARINGS)
folders_a = Array.new(N_SHARINGS)
members = {}
dirs = {}
files = {}

N_SHARINGS.times do |i|
  # Create the hierarchy to share
  folders_a[i] = Folder.create inst_a, name: Faker::Internet.unique.slug
  n_files = Random.rand 1..MAX_FILES_INIT
  dirs_a, files_a = create_hierarchy inst_a, folders_a[i], n_files, FILE_MAX_SIZE

  # Randomly select the recipients
  n_rec = Random.rand 1..MAX_RECIPIENTS
  recs = recipients.sample(n_rec)
  s_members = [inst_a] + recs

  # Share it
  sharing = create_sharing s_members, folders_a[i]
  sid = sharing.couch_id
  sids[i] = sid
  dirs[sid] = { inst_a.name => dirs_a }
  files[sid] = { inst_a.name => files_a }
  members[sid] = s_members

  # Check everything went well for each recipient of this sharing
  da = File.join Helpers.current_dir, inst_a.domain, folders_a[i].name
  path_folder = "/#{Helpers::SHARED_WITH_ME}/#{folders_a[i].name}"

  recs.each do |rec|
    drec = File.join Helpers.current_dir, rec.domain, path_folder
    poll_for_diff da, drec
    folder_rec = Folder.find_by_path rec, CGI.escape(path_folder)
    dirs[sid][rec.name], files[sid][rec.name] = get_hierarchy rec, folder_rec
  end
end

puts "#{N_SHARINGS} sharings have been successfully created"

# Generate random writes on random sharings on random instances
n_updates = Random.rand MAX_UPDATES

n_updates.times do
  sid = pick_random_element sids
  inst = pick_random_element members[sid]
  dirs_inst = dirs[sid][inst.name]
  files_inst = files[sid][inst.name]

  case Random.rand(3)
  # Insert
  when 0
    create_dir_or_file inst, dirs_inst, files_inst
  # Update
  when 1
    update_dir_or_file inst, dirs, files
  # Delete
  when 2
    remove_folder_or_file inst, dirs_inst, files_inst
  end
end

start = Time.now
sids.each_with_index do |sid, i|
  da = File.join Helpers.current_dir, inst_a.domain, folders_a[i].name
  path_folder = "/#{Helpers::SHARED_WITH_ME}/#{folders_a[i].name}"
  recs = members[sid].drop 1
  recs.each do |rec|
    drec = File.join Helpers.current_dir, rec.domain, path_folder
    poll_for_diff da, drec
  end
end
finish = Time.now
diff = finish - start
puts "All sharings have converged in #{diff}s after #{n_updates} updates"
