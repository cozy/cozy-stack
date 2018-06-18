require_relative './base_tests.rb'

N_FILES_INIT = 1
N_UPDATES = 1000

# This test produces updates during and after the sharing process

# Create the instances
inst_names = ["Alice", "Bob"]
insts = create_instances inst_names
inst_a = insts[0]
inst_b = insts[1]

# Create the files to share
folder = Folder.create inst_a
files_a = create_files inst_a, N_FILES_INIT, folder.couch_id

# Create the sharing and generate updates
create_sharing insts, folder
generate_updates [inst_a], N_UPDATES, files_a

# Make sure everything went well
da = File.join Helpers.current_dir, inst_a.domain, folder.name
path_folder_b = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
db = File.join Helpers.current_dir, inst_b.domain, path_folder_b
poll_for_diff da, db

# Generate updates from both sides
_, files_b = Folder.children inst_b, CGI.escape(path_folder_b)

generate_updates insts, N_UPDATES, files_a, files_b
poll_for_diff da, db
