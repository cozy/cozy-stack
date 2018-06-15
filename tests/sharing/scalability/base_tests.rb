require_relative '../boot'

def create_instances(names)
  insts = Array.new(names.length)
  names.each_with_index do |name, i|
    insts[i] = Instance.create name: name
  end
  insts
end

def create_sharing(insts, obj)
  inst_sharer = insts[0]
  sharing = Sharing.new
  sharing.rules << Rule.sync(obj)
  sharing.members << inst_sharer
  recipients = insts.drop 1
  recipients.each do |inst|
    contact = Contact.create inst_sharer, given_name: inst.name
    sharing.members << contact
  end
  inst_sharer.register sharing
  sleep 1
  recipients.each do |inst|
    inst.accept sharing
    sleep 2
  end
  sharing
end

# Create a file with a fixed size, in KB
def create_file_with_size(inst, dir_id, size)
  file_path = "tmp/#{Faker::Internet.unique.slug}.txt"
  buffer = "a" * 1024
  File.open(file_path, 'w') do |f|
    size.to_i.times { f.write(buffer) }
  end
  opts = CozyFile.options_from_fixture file_path, dir_id: dir_id
  CozyFile.create inst, opts
end

def create_files(inst, n_files, dir_id)
  print "Create #{n_files} files... "
  files = Array.new(n_files)
  n_files.times do |i|
    files[i] = create_file inst, dir_id, size
  end
  print "Done.\n"
  files
end

def create_hierarchy(inst, root, n_elements, max_filesize = nil)
  files = []
  dirs = [root]
  n_elements.times do
    create_dir_or_file inst, dirs, files, max_filesize
  end
  [dirs, files]
end

def get_hierarchy(inst, root)
  dirs, files = Folder.children inst, CGI.escape(root.path)

  if dirs.length > 0
    dirs.each do |dir|
      sdirs, sfiles = get_hierarchy inst, dir
      dirs += sdirs
      files += sfiles
    end
  end
  dirs.unshift root
  [dirs, files]
end

def create_file(inst, dir_id, filesize = nil)
  if filesize.nil?
    file_name = "#{Faker::Internet.unique.slug}.txt"
    CozyFile.create inst, dir_id: dir_id, name: file_name
  else
    create_file_with_size inst, dir_id, filesize
  end
end

def create_dir(inst, dir_id)
  dir_name = Faker::Internet.unique.slug
  parent = Folder.find inst, dir_id
  path = "#{parent.path}/#{dir_name}"
  Folder.create inst, dir_id: dir_id, name: dir_name, path: path
end

def create_dir_or_file(inst, dirs, files, max_filesize = nil)
  dir_id = pick_random_element(dirs).couch_id
  create_folder = [true, false].sample

  if create_folder
    dir = create_dir inst, dir_id
    dirs << dir
  else
    filesize = Random.rand(1..max_filesize) unless max_filesize.nil?
    file = create_file inst, dir_id, filesize
    files << file
  end
end

def remove_folder(inst, dirs, files)
  return unless dirs.length > 2

  removable_dirs = dirs - [dirs[0]] # do not remove the root folder
  dir = pick_random_element removable_dirs
  dir.remove inst
  remove_folder_in_hierarchy dirs, files, dir
end

def remove_file(inst, files)
  return unless files.length > 0

  file = pick_random_element files
  file.remove inst
  files.delete file
end

def remove_folder_or_file(inst, dirs, files)
  is_folder = [true, false].sample
  if is_folder
    remove_folder inst, dirs, files
  else
    remove_file inst, files
  end
end

# Returns true if the element is part of the root's hierarchy
def child?(root, dirs, el)
  return true unless el.dir_id != root.couch_id

  parent = dirs.find { |dir| dir.couch_id == el.dir_id }
  return false if parent.nil?
  child? root, dirs, parent
end

# Remove all the descendants of the given folder
def remove_folder_in_hierarchy(dirs, files, folder)
  dirs_to_del = dirs.map { |dir| dir if child? folder, dirs, dir }.compact
  dirs_to_del << folder
  files_to_del = files.map { |file| file if child? folder, dirs, file }.compact

  dirs_to_del.each { |dir| dirs.delete dir }
  files_to_del.each { |file| files.delete file }
end

def rename_dir_or_file(inst, dirs, files)
  is_folder = [true, false].sample
  if is_folder
    dir = pick_random_element dirs
    dir.rename inst, Faker::Internet.unique.slug unless dir.nil?
  else
    file = pick_random_element files
    file.rename inst, "#{Faker::Internet.unique.slug}.txt" unless file.nil?
  end
end

def rewrite_file(inst, files)
  file = pick_random_element files
  file.overwrite inst unless file.nil?
end

def move_dir_or_file(inst, dirs, files)
  is_folder = [true, false].sample
  parent = pick_random_element dirs

  if is_folder
    dir = pick_random_element dirs
    dir.move_to inst, parent.couch_id unless dir.nil?
  else
    file = pick_random_element files
    file.move_to inst, parent.couch_id unless file.nil?
  end
end

def update_dir_or_file(inst, dirs, files)
  case Random.rand(3)
  when 0
    rename_dir_or_file inst, dirs, files
  when 1
    move_dir_or_file inst, dirs, files
  when 2
    rewrite_file inst, files
  end
end

# Randomly generate updates on instances
def generate_updates(insts, n_updates, *files)
  return unless insts.length == files.length

  n_updates.times do
    i_inst = Random.rand insts.length
    i_file = Random.rand files[i_inst].length
    rename_or_rewrite insts[i_inst], files[i_inst][i_file]
  end
end

def rename_or_rewrite(inst, file)
  rename = [true, false].sample
  if rename
    file.rename inst, "#{Faker::Internet.unique.slug}.txt"
  else
    file.overwrite inst
  end
end

# Run a diff on folders until they are even
def poll_for_diff(da, db)
  printf "Waiting for shared files to be consistent in file system... "
  start = Time.now
  loop do
    begin
      diff = Helpers.fsdiff da, db
      if diff.empty?
        finish = Time.now
        diff = finish - start
        printf "Done in #{diff}s.\n"
        break
      end
    rescue
    end
    sleep 2
  end
end

def pick_random_element(array)
  return nil if array.empty?
  array.length == 1 ? array[0] : array[Random.rand array.length]
end
