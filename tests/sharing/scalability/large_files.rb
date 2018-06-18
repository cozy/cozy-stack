require_relative './base_tests.rb'

N_FILES_INIT = 100
FILE_MAX_SIZE = 1_000_000 # In KB

# This test creates a sharing with large files

# Create the instances
inst_names = ["Alice", "Bob"]
insts = create_instances inst_names
inst_a = insts[0]
inst_b = insts[1]

# Create the folder to share
folder = Folder.create inst_a

# Create files with size between 1 MB and FILE_MAX_SIZE KB
N_FILES_INIT.times do
  size = Random.rand FILE_MAX_SIZE
  create_file_with_size inst_a, folder.couch_id, size
end

# Create the sharing
create_sharing insts, folder

# Make sure everything went well
da = File.join Helpers.current_dir, inst_a.domain, folder.name
db = File.join Helpers.current_dir, inst_b.domain, "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
poll_for_diff da, db
