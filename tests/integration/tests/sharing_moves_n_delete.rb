require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A shared directory" do
  it "can have its children moved out of it and then be deleted" do
    Helpers.scenario "moves_n_delete"
    Helpers.start_mailhog

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_bob = Instance.create name: "Bob"
    inst_charlie = Instance.create name: "Charlie"

    # Create hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    subdir = Folder.create inst, dir_id: folder.couch_id
    child1 = Folder.create inst, dir_id: subdir.couch_id
    child2 = Folder.create inst, dir_id: subdir.couch_id
    child3 = Folder.create inst, dir_id: subdir.couch_id
    filename1 = "#{Faker::Internet.slug}1.txt"
    filename2 = "#{Faker::Internet.slug}2.txt"
    filename3 = "#{Faker::Internet.slug}3.txt"
    filename4 = "#{Faker::Internet.slug}4.txt"
    file1 = CozyFile.create inst, name: filename1, dir_id: child1.couch_id
    file2 = CozyFile.create inst, name: filename2, dir_id: child2.couch_id
    file3 = CozyFile.create inst, name: filename3, dir_id: child3.couch_id
    file4 = CozyFile.create inst, name: filename4, dir_id: subdir.couch_id
    other = Folder.create inst
    subother = Folder.create inst, dir_id: other.couch_id
    filename5 = "#{Faker::Internet.slug}5.txt"
    filename6 = "#{Faker::Internet.slug}6.txt"
    file5 = CozyFile.create inst, name: filename5, dir_id: other.couch_id
    file6 = CozyFile.create inst, name: filename6, dir_id: subother.couch_id
    child4 = Folder.create inst, dir_id: folder.couch_id
    filename7 = "#{Faker::Internet.slug}7.txt"
    file7 = CozyFile.create inst, name: filename7, dir_id: child4.couch_id

    # Create the sharing
    contact_bob = Contact.create inst, given_name: "Bob"
    contact_charlie = Contact.create inst, given_name: "Charlie"
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact_bob << contact_charlie
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    inst_charlie.accept sharing
    sleep 12

    # Check that the files have been synchronized
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_bob.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    dc = File.join Helpers.current_dir, inst_charlie.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty

    # Move what is in subdir out of it...
    file4.move_to inst, folder.couch_id
    child1.move_to inst, folder.couch_id
    child2.move_to inst, folder.couch_id
    child3.move_to inst, other.couch_id

    # And Bob moves the child4 dir outside of the sharing
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child4.name}"
    child4_bob = Folder.find_by_path inst_bob, path
    child4_bob.move_to inst_bob, Folder::ROOT_DIR

    # ...and delete it
    subdir.remove inst
    sleep 12

    # Check that no children have been lost
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_bob = Folder.find_by_path inst_bob, path
    refute child1_bob.trashed
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_bob = Folder.find_by_path inst_bob, path
    refute child2_bob.trashed
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{file1.name}"
    file1_bob = CozyFile.find_by_path inst_bob, path
    refute file1_bob.trashed
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}/#{file2.name}"
    file2_bob = CozyFile.find_by_path inst_bob, path
    refute file2_bob.trashed
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file4.name}"
    file4_bob = CozyFile.find_by_path inst_bob, path
    refute file4_bob.trashed

    # Move the other directory inside the shared folder
    other.move_to inst, folder.couch_id
    sleep 12
    # Debug.visualize_tree [inst, inst_bob, inst_charlie], sharing

    # Check that the files have been added on the bob
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{other.name}/#{child3.name}/#{file3.name}"
    file3_bob = Folder.find_by_path inst_bob, path
    refute file3_bob.trashed
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{other.name}/#{file5.name}"
    file5_bob = Folder.find_by_path inst_bob, path
    refute file5_bob.trashed
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{other.name}/#{subother.name}/#{file6.name}"
    file6_bob = Folder.find_by_path inst_bob, path
    refute file6_bob.trashed

    # Check that we have no surprise
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_bob.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    dc = File.join Helpers.current_dir, inst_charlie.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty

    assert_equal inst.check, []
    assert_equal inst_bob.check, []
    assert_equal inst_charlie.check, []

    inst.remove
    inst_bob.remove
    inst_charlie.remove
  end
end
