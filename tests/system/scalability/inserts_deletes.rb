require_relative './base_tests.rb'

N_FILES_INIT = 1000
N_INSERTS_OR_DELETES = 1000

# This test creates a sharing with many inserts and deletes

# Create the instances
inst_names = ["Alice", "Bob"]
insts = create_instances inst_names
inst_a = insts[0]
inst_b = insts[1]

# Create the hierarchy to share
folder = Folder.create inst_a
dirs, files = create_hierarchy inst_a, folder, N_FILES_INIT

# Create the sharing
create_sharing insts, folder

# Check everything went well
da = File.join Helpers.current_dir, inst_a.domain, folder.name
db = File.join Helpers.current_dir, inst_b.domain, "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
poll_for_diff da, db

# Randomly add or remove folder/files in the hierarchy
N_INSERTS_OR_DELETES.times do
  is_creation = (dirs.length + files.length) < 2 ? true : [true, false].sample
  if is_creation
    create_dir_or_file inst_a, dirs, files
  else
    remove_folder_or_file inst_a, dirs, files
  end
end
poll_for_diff da, db
