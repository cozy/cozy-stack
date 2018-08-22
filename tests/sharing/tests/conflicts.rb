require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def children_by_parent_id(inst, parent_id)
  parent = Folder.find inst, parent_id
  Folder.children inst, CGI.escape(parent.path)
end

def assert_conflict_children(inst_a, inst_b, parent_id_a, parent_id_b, filename)
  _, children_a = children_by_parent_id inst_a, parent_id_a
  assert 2, children_a.length
  children_a.each { |child| assert child.name.include? filename }

  _, children_b = children_by_parent_id inst_b, parent_id_b
  assert 2, children_b.length
  children_b.each { |child| assert child.name.include? filename }

  assert_equal children_a[0].name, children_b[0].name
  assert_equal children_a[1].name, children_b[1].name

  assert_equal children_a[0].md5sum, children_b[0].md5sum
  assert_equal children_a[1].md5sum, children_b[1].md5sum

  assert_equal children_a[0].couch_rev, children_b[0].couch_rev
  assert_equal children_a[1].couch_rev, children_b[1].couch_rev
end

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
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child5.name}"
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

    sleep 15
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
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

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

    sleep 20
    # Check the conflicted files
    _, files = Folder.children inst, parent_file.path
    conflict_file = files.find { |c| c.name.include? "conflict" }
    refute_nil conflict_file
    path = CGI.escape "#{parent_file_recipient.path}/#{conflict_file.name}"
    conflict_file_recipient = CozyFile.find_by_path inst_recipient, path
    assert_equal conflict_file_recipient.name, conflict_file.name
    assert_equal conflict_file_recipient.couch_rev, conflict_file.couch_rev
    assert_equal conflict_file_recipient.md5sum, conflict_file.md5sum

    assert_conflict_children inst, inst_recipient, child3.couch_id, child3_recipient.couch_id, file4.name
    assert_conflict_children inst, inst_recipient, child4.couch_id, child4_recipient.couch_id, file6.name
    assert_conflict_children inst, inst_recipient, child5.couch_id, child5_recipient.couch_id, file_to_trash.name

    diff = Helpers.fsdiff da, db
    diff.must_be_empty
  end
end
