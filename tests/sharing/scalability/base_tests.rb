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

def create_file_with_size(inst, dir_id, size)
  file_path = "tmp/#{Faker::Internet.unique.slug}.txt"
  buffer = "a" * (1024 * 1024)
  File.open(file_path, 'w') do |f|
    size.to_i.times { f.write(buffer) }
  end
  opts = CozyFile.options_from_fixture(file_path, dir_id: dir_id)
  CozyFile.create inst, opts
end

def create_files(inst, n_files, dir_id)
  print "Create #{n_files} files... "
  files = Array.new(n_files)
  n_files.times do |i|
    file_name = "#{Faker::Internet.unique.slug}.txt"
    files[i] = CozyFile.create inst, dir_id: dir_id, name: file_name
  end
  print "Done.\n"
  files
end

def rename_or_rewrite(inst, file)
  i = Random.rand 2
  if i.zero?
    file.rename inst, Faker::Internet.unique.slug
  else
    file.overwrite inst
  end
  file
end

# Randomly generate updates
def generate_updates(inst, n_updates, files)
  n_updates.times do
    i_file = Random.rand files.length
    rename_or_rewrite inst, files[i_file]
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
