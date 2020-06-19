require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def children_by_parent_id(inst, parent_id)
  parent = Folder.find inst, parent_id
  Folder.children inst, parent.path
end

def assert_conflict_children(inst_a, inst_b, parent_id_a, parent_id_b, filename)
  basename = File.basename filename, File.extname(filename)

  _, children_a = children_by_parent_id inst_a, parent_id_a
  assert 2, children_a.length
  children_a.each { |child| assert child.name.include? basename }

  _, children_b = children_by_parent_id inst_b, parent_id_b
  assert 2, children_b.length
  children_b.each { |child| assert child.name.include? basename }

  assert_equal children_a[0].name, children_b[0].name
  assert_equal children_a[1].name, children_b[1].name

  assert_equal children_a[0].md5sum, children_b[0].md5sum
  assert_equal children_a[1].md5sum, children_b[1].md5sum

  assert_equal children_a[0].couch_rev, children_b[0].couch_rev
  assert_equal children_a[1].couch_rev, children_b[1].couch_rev
end

describe 'A sharing' do
  it 'can handle conflicts' do
    Helpers.scenario 'conflicts'
    Helpers.start_mailhog

    recipient_name = 'Bob'

    # Create the instance
    inst = Instance.create name: 'Alice'
    inst_recipient = Instance.create name: recipient_name

    # Create hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, dir_id: folder.couch_id
    child2 = Folder.create inst, dir_id: folder.couch_id
    child3 = Folder.create inst, dir_id: folder.couch_id
    child4 = Folder.create inst, dir_id: folder.couch_id
    child5 = Folder.create inst, dir_id: folder.couch_id
    filename1 = "#{Faker::Internet.slug}.txt"
    filename2 = "#{Faker::Internet.slug}.txt"
    file1 = CozyFile.create inst, name: filename1, dir_id: folder.couch_id
    file2 = CozyFile.create inst, name: filename2, dir_id: folder.couch_id

    # Create the sharing
    contact = Contact.create inst, given_name: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 2
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file1.name}"
    file1_recipient = CozyFile.find_by_path inst_recipient, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file2.name}"
    file2_recipient = CozyFile.find_by_path inst_recipient, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_recipient = Folder.find_by_path inst_recipient, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_recipient = Folder.find_by_path inst_recipient, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child3.name}"
    child3_recipient = Folder.find_by_path inst_recipient, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child4.name}"
    child4_recipient = Folder.find_by_path inst_recipient, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child5.name}"
    child5_recipient = Folder.find_by_path inst_recipient, path

    # Create and trash a file for later use
    filename_trash = "#{Faker::Internet.slug}.txt"
    file_to_trash = CozyFile.create inst, name: filename_trash, dir_id: child5.couch_id
    file_to_trash.remove inst

    # Generate conflicts with reconciliation

    # Move the file on both sides
    file1.move_to inst, child1.couch_id
    file1_recipient.move_to inst_recipient, child2_recipient.couch_id
    file1.move_to inst, folder.couch_id
    file1_recipient.move_to inst_recipient, child1_recipient.couch_id

    # Remove a file and write it on the other side
    file2.remove inst
    file2_recipient.overwrite inst_recipient

    # Rename file and folder on both sides and write file on one side
    2.times do
      child1.rename inst, Faker::Internet.slug
      file1.rename inst, "#{Faker::Internet.slug}.txt"
      file1.overwrite inst, content: Faker::BackToTheFuture.quote
      child1_recipient.rename inst_recipient, Faker::Internet.slug
      file1_recipient.rename inst_recipient, "#{Faker::Internet.slug}.txt"
    end

    sleep 30
    # Check the files and diretories are even
    file1 = CozyFile.find inst, file1.couch_id
    file2 = CozyFile.find inst, file2.couch_id
    parent_file = Folder.find inst, file1.dir_id
    file1_recipient = CozyFile.find inst_recipient, file1_recipient.couch_id
    file2_recipient = CozyFile.find inst_recipient, file2_recipient.couch_id
    parent_file_recipient = Folder.find inst_recipient, file1_recipient.dir_id
    child1 = Folder.find inst, child1.couch_id
    child1_recipient = Folder.find inst_recipient, child1_recipient.couch_id
    assert_equal parent_file.name, parent_file_recipient.name
    assert_equal file1.name, file1_recipient.name
    assert_equal file1.couch_rev, file1_recipient.couch_rev
    assert_equal file1.md5sum, file1_recipient.md5sum
    assert_equal child1.name, child1_recipient.name
    assert_equal child1.couch_rev, child1_recipient.couch_rev
    assert file2.trashed
    assert file2_recipient.trashed

    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    Helpers.fsdiff(da, db).must_be_empty

    # Generate conflicts with no reconciliation

    # Create 2 differents files with same name
    filename = "#{Faker::Internet.slug}.txt"
    file4 = CozyFile.create inst, name: filename, dir_id: child3.couch_id
    CozyFile.create inst_recipient, name: filename, dir_id: child3_recipient.couch_id

    # Create a file and rename an existing one with the newly created name
    filename = "#{Faker::Internet.slug}.txt"
    file5 = CozyFile.create inst, name: filename, dir_id: child4.couch_id
    filename = "#{Faker::Internet.slug}.txt"
    file6 = CozyFile.create inst_recipient, name: filename, dir_id: child4_recipient.couch_id
    file5.rename inst, file6.name

    # Create a file on a side and restore a file with same name on other side
    CozyFile.create inst_recipient, name: filename_trash, dir_id: child5_recipient.couch_id
    file_to_trash.restore inst

    # Write the same file on both sides
    2.times do
      file1.overwrite inst, content: Faker::BackToTheFuture.quote
      file1_recipient.overwrite inst_recipient
    end

    sleep 30
    # Check the conflicted files
    _, files = Folder.children inst, parent_file.path
    conflict_file = files.find { |c| c.name =~ / \(\d+\)/ }
    refute_nil conflict_file
    path = "#{parent_file_recipient.path}/#{conflict_file.name}"
    conflict_file_recipient = CozyFile.find_by_path inst_recipient, path
    assert_equal conflict_file_recipient.name, conflict_file.name
    assert_equal conflict_file_recipient.couch_rev, conflict_file.couch_rev
    assert_equal conflict_file_recipient.md5sum, conflict_file.md5sum

    assert_conflict_children inst, inst_recipient, child3.couch_id, child3_recipient.couch_id, file4.name
    assert_conflict_children inst, inst_recipient, child4.couch_id, child4_recipient.couch_id, file6.name
    assert_conflict_children inst, inst_recipient, child5.couch_id, child5_recipient.couch_id, file_to_trash.name

    Helpers.fsdiff(da, db).must_be_empty

    assert_equal inst.check, []
    assert_equal inst_recipient.check, []

    inst.remove
    inst_recipient.remove
  end

  it 'does not create couchdb conflicts' do
    Helpers.scenario 'couchdb-conflicts'
    Helpers.start_mailhog

    members_name = %w[Alice Bob Charlie]

    # Create the instances
    inst_a = Instance.create name: members_name[0]
    inst_b = Instance.create name: members_name[1]
    inst_c = Instance.create name: members_name[2]

    # Create hierarchy
    folder = Folder.create inst_a, name: 'shared-folder'
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst_a, dir_id: folder.couch_id
    child2 = Folder.create inst_a, dir_id: folder.couch_id
    child1_1 = Folder.create inst_a, dir_id: child1.couch_id
    child1_2 = Folder.create inst_a, dir_id: child1.couch_id
    child1_1_1 = Folder.create inst_a, dir_id: child1_1.couch_id
    Folder.create inst_a, dir_id: child1_1.couch_id
    Folder.create inst_a, dir_id: child1_1.couch_id
    file1 = CozyFile.create inst_a, dir_id: child1_1_1.couch_id
    file2 = CozyFile.create inst_a, dir_id: child1_1_1.couch_id

    # Create the sharing
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst_a
    contact_b = Contact.create inst_a, given_name: members_name[1]
    contact_c = Contact.create inst_a, given_name: members_name[2]
    sharing.members << contact_b
    sharing.members << contact_c
    inst_a.register sharing

    # Accept the sharing
    sleep 1
    inst_b.accept sharing
    sleep 1
    inst_c.accept sharing
    sleep 10

    # Rename directories
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_b = Folder.find_by_path inst_b, path
    child1_c = Folder.find_by_path inst_c, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_b = Folder.find_by_path inst_b, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{child1_1.name}"
    child1_1b = Folder.find_by_path inst_b, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{child1_1.name}/#{child1_1_1.name}"
    child1_1_1b = Folder.find_by_path inst_b, path
    child1_1_1c = Folder.find_by_path inst_c, path

    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{child1_1.name}/#{child1_1_1.name}/#{file1.name}"
    file1_b = CozyFile.find_by_path inst_b, path

    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{child1_1.name}/#{child1_1_1.name}/#{file1.name}"
    file1_c = CozyFile.find_by_path inst_c, path

    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{child1_1.name}/#{child1_1_1.name}/#{file2.name}"
    file2_b = CozyFile.find_by_path inst_b, path

    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{child1_1.name}/#{child1_1_1.name}/#{file2.name}"
    file2_c = CozyFile.find_by_path inst_c, path

    # Try to create CouchDB conflicts on directories
    child1_b.rename inst_b, Faker::Internet.slug
    sleep 12
    child1_b.rename inst_b, Faker::Internet.slug
    sleep 12
    child1_1b.rename inst_b, Faker::Internet.slug
    child1_1_1b.rename inst_b, Faker::Internet.slug

    # Try to create CouchDB conflicts on files
    file1_b.rename inst_b, Faker::Internet.slug
    file1_b.rename inst_b, Faker::Internet.slug
    file1_b.rename inst_b, Faker::Internet.slug
    file1_b.overwrite inst_b, content: Faker::Friends.quote

    file1_b.rename inst_b, Faker::Internet.slug
    file1_b.rename inst_b, Faker::Internet.slug
    file1_c.rename inst_c, Faker::Internet.slug
    file1_c.overwrite inst_c, content: Faker::GameOfThrones.quote

    file2_b.rename inst_b, Faker::Internet.slug
    file1.rename inst_a, Faker::Internet.slug
    file1.rename inst_a, Faker::Internet.slug
    file2.rename inst_a, Faker::Internet.slug

    child1_1_1c.move_to inst_c, child1_c.couch_id
    child1_1_1c.rename inst_c, Faker::Internet.slug
    child1_1_1b.move_to inst_b, child2_b.couch_id

    child1_1_1.move_to inst_a, child1_2.couch_id
    child1_1_1.rename inst_a, Faker::Internet.slug

    file2_b.overwrite inst_b, content: Faker::Friends.quote
    file2_c.rename inst_c, Faker::Internet.slug
    file2_c.overwrite inst_c, content: Faker::GameOfThrones.quote

    file1.overwrite inst_a, content: Faker::SiliconValley.quote
    file1.overwrite inst_a, content: Faker::SiliconValley.quote
    file2.overwrite inst_a, content: Faker::SiliconValley.quote

    sleep 60

    # Check that the files have been synchronized
    da = File.join Helpers.current_dir, inst_a.domain, folder.name
    db = File.join Helpers.current_dir, inst_b.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    dc = File.join Helpers.current_dir, inst_c.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty

    # Check there is no conflict
    [inst_a, inst_b, inst_c].each do |inst|
      assert_equal inst.check, []
      inst.remove
    end
  end
end
