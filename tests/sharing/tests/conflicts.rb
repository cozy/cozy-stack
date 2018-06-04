require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A sharing" do
  Helpers.scenario "conflicts"
  Helpers.start_mailhog

  it "can handle conflicts" do
    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, dir_id: folder.couch_id
    child2 = Folder.create inst, dir_id: folder.couch_id
    child3 = Folder.create inst, dir_id: folder.couch_id
    child4 = Folder.create inst, dir_id: folder.couch_id
    filename1 = "#{Faker::Internet.slug}.txt"
    filename2 = "#{Faker::Internet.slug}.txt"
    file1 = CozyFile.create inst, name: filename1, dir_id: folder.couch_id
    file2 = CozyFile.create inst, name: filename2, dir_id: folder.couch_id

    # Create the sharing
    contact = Contact.create inst, givenName: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 2
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file1.name}"
    file1_recipient = CozyFile.find_by_path inst_recipient, path
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file2.name}"
    file2_recipient = CozyFile.find_by_path inst_recipient, path
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_recipient = Folder.find_by_path inst_recipient, path
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_recipient = Folder.find_by_path inst_recipient, path
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child3.name}"
    child3_recipient = Folder.find_by_path inst_recipient, path
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child4.name}"
    child4_recipient = Folder.find_by_path inst_recipient, path

    # Generate conflicts with reconciliation

    # Move the file on both sides
    file1.move_to inst, child1.couch_id
    file1_recipient.move_to inst_recipient, child2_recipient.couch_id
    file1.move_to inst, folder.couch_id
    file1_recipient.move_to inst_recipient, child1_recipient.couch_id

    # Remove a file and write it on the other side
    file2.remove inst
    file2_recipient.overwrite inst_recipient

    # Create a file and remove the folder on the other side
    filename = "#{Faker::Internet.slug}.txt"
    file3 = CozyFile.create inst, name: filename, dir_id: child2.couch_id
    child2_recipient.remove inst_recipient

    # Rename file and folder on both sides and write file on one side
    2.times do
      child1.rename inst, Faker::Internet.slug
      file1.rename inst, "#{Faker::Internet.slug}.txt"
      # FIXME: the overwrite does not seem to be properly handled: the file is
      # going to have a 'conflict - xxx' name,  with a 'xxx' different on
      # both sides
      # file1.overwrite inst
      child1_recipient.rename inst_recipient, Faker::Internet.slug
      file1_recipient.rename inst_recipient, "#{Faker::Internet.slug}.txt"
    end

    sleep 20
    # Check the files and diretories are even
    file1 = CozyFile.find inst, file1.couch_id
    file2 = CozyFile.find inst, file2.couch_id
    file3 = CozyFile.find inst, file3.couch_id
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

    # FIXME: child2 and its child file3 are in the trash
    # puts "child2 #{child2.name}"
    # child2 = Folder.find inst, child2.couch_id
    # refute_equal Folder::TRASH_DIR, child2.dir_id
    # refute_equal Folder::TRASH_DIR, file3.dir_id

    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

    # Generate conflicts with no reconciliation

    # Create 2 differents files with same name

    filename = "#{Faker::Internet.slug}.txt"
    file4 = CozyFile.create inst, name: filename, dir_id: child3.couch_id
    CozyFile.create inst_recipient, name: filename, dir_id: child3_recipient.couch_id

    # Create a file and rename an existing one with the newly created name
    # FIXME : file5 and file6 end up (sometimes) with several '- conflict' in their name
    # puts "child 4 : #{child4.name}"
    filename = "#{Faker::Internet.slug}.txt"
    file5 = CozyFile.create inst, name: filename, dir_id: child4.couch_id
    filename = "#{Faker::Internet.slug}.txt"
    file6 = CozyFile.create inst_recipient, name: filename, dir_id: child4_recipient.couch_id
    file5.rename inst, file6.name

    # Write the same file on both sides
    2.times do
      file1.overwrite inst
      file1_recipient.overwrite inst_recipient
    end

    sleep 15
    # Check the conflicted files
    children = Folder.children inst, parent_file.path
    conflict_file = children.find { |c| c.name.include? "conflict" }
    refute_nil conflict_file
    path = CGI.escape "#{parent_file_recipient.path}/#{conflict_file.name}"
    conflict_file_recipient = CozyFile.find_by_path inst_recipient, path
    assert_equal conflict_file_recipient.name, conflict_file.name
    assert_equal conflict_file_recipient.couch_rev, conflict_file.couch_rev
    assert_equal conflict_file_recipient.md5sum, conflict_file.md5sum

    child3 = Folder.find inst, child3.couch_id
    children = Folder.children inst, child3.path
    assert 2, children.length
    children.each { |child| assert child.name.include? file4.name }


    diff = Helpers.fsdiff da, db
    diff.must_be_empty
  end
end
