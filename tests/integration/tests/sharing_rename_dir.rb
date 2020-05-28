require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A directory in a sharing" do
  it "can be renamed" do
    Helpers.scenario "rename_dir"
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

    # Rename the directory
    subdir.rename inst, "9.1_#{subdir.name}"
    child3.move_to inst, child2.couch_id
    sleep 12

    # Check that no children have been lost
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{subdir.name}"
    subdir_recipient = Folder.find_by_path inst_recipient, path
    refute subdir_recipient.trashed
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{subdir.name}/#{child1.name}"
    child1_recipient = Folder.find_by_path inst_recipient, path
    refute child1_recipient.trashed
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{subdir.name}/#{child2.name}"
    child2_recipient = Folder.find_by_path inst_recipient, path
    refute child2_recipient.trashed
    path = CGI.escape "/#{child2_recipient.path}/#{child3.name}"
    child3_recipient = Folder.find_by_path inst_recipient, path
    refute child3_recipient.trashed

    # Check that we have no surprise
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

    assert_equal inst.check, []
    assert_equal inst_recipient.check, []

    inst.remove
    inst_recipient.remove
  end
end
