require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A shared directory" do
  it "can have its children moved out of it and then be deleted" do
    Helpers.scenario "moves_n_delete"
    Helpers.start_mailhog

    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    subdir = Folder.create inst, dir_id: folder.couch_id
    child1 = Folder.create inst, dir_id: subdir.couch_id
    child2 = Folder.create inst, dir_id: subdir.couch_id
    child3 = Folder.create inst, dir_id: subdir.couch_id
    filename1 = "#{Faker::Internet.slug}.txt"
    filename2 = "#{Faker::Internet.slug}.txt"
    filename3 = "#{Faker::Internet.slug}.txt"
    file1 = CozyFile.create inst, name: filename1, dir_id: child1.couch_id
    file2 = CozyFile.create inst, name: filename2, dir_id: child2.couch_id
    file3 = CozyFile.create inst, name: filename3, dir_id: subdir.couch_id

    # Create the sharing
    contact = Contact.create inst, given_name: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 12

    # Check that the files have been synchronized
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

    # Move what is in subdir out of it...
    file3.move_to inst, folder.couch_id
    # TODO remove this sleep. Files move/rename are currently managed from the
    # worker share_upload, not with the directories in share_replicate. It is
    # something that should be changed in the future. The triggers for the 2
    # workers have different debounce values, which is why the sleep was added
    # as a temporary workaround.
    sleep 6
    child1.move_to inst, folder.couch_id
    child2.move_to inst, folder.couch_id
    child3.move_to inst, folder.couch_id

    # ...and delete it
    subdir.remove inst
    sleep 12
    # Debug.visualize_tree [inst, inst_recipient], sharing

    # Check that no children have been lost
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_recipient = Folder.find_by_path inst_recipient, path
    refute child1_recipient.trashed
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_recipient = Folder.find_by_path inst_recipient, path
    refute child2_recipient.trashed
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child3.name}"
    child3_recipient = Folder.find_by_path inst_recipient, path
    refute child3_recipient.trashed
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{file1.name}"
    file1_recipient = CozyFile.find_by_path inst_recipient, path
    refute file1_recipient.trashed
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}/#{file2.name}"
    file2_recipient = CozyFile.find_by_path inst_recipient, path
    refute file2_recipient.trashed
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file3.name}"
    file3_recipient = CozyFile.find_by_path inst_recipient, path
    refute file3_recipient.trashed

    # Check that we have no surprise
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

    assert_equal inst.check, []
    assert_equal inst_recipient.check, []

    inst.remove
    inst_recipient.remove
  end
end
