require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A shared folder" do
  Helpers.scenario "remove_folder"
  Helpers.start_mailhog

  it "can be removed and end up in the trash" do
    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create the hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, dir_id: folder.couch_id
    child2 = Folder.create inst, dir_id: folder.couch_id
    child3 = Folder.create inst, dir_id: child2.couch_id
    file_path = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file_path, dir_id: child1.couch_id)
    f1 = CozyFile.create inst, opts
    opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
    f2 = CozyFile.create inst, opts

    # Create the sharing
    contact = Contact.create inst, given_name: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.push(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 7

    # Get id for all dir/files to retrieve after delete
    child2_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_recipient_id = Folder.get_id_from_path inst_recipient, child2_path

    child3_path = "#{child2_path}/#{child3.name}"
    child3_recipient_id = Folder.get_id_from_path inst_recipient, child3_path

    f1_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{f1.name}"
    f1_recipient_id = CozyFile.get_id_from_path inst_recipient, f1_path

    f2_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{f2.name}"
    f2_recipient_id = CozyFile.get_id_from_path inst_recipient, f2_path

    # Remove a single file
    f2.remove inst

    # Move a dir out of the shared folder and update it
    child2.move_to inst, Folder::ROOT_DIR
    child2.rename inst, Faker::Internet.slug

    # Remove a directory containing a binary
    child1.remove inst

    sleep 7

    f2_recipient = CozyFile.find inst_recipient, f2_recipient_id
    assert_equal true, f2_recipient.trashed

    child2_recipient = Folder.find inst_recipient, child2_recipient_id
    child3_recipient = Folder.find inst_recipient, child3_recipient_id
    assert_equal "/.cozy_trash/#{child2_recipient.name}", child2_recipient.path
    assert_equal "/.cozy_trash/#{child2_recipient.name}/#{child3_recipient.name}", child3_recipient.path
    assert_equal "/#{Helpers::SHARED_WITH_ME}/#{folder.name}", child2_recipient.restore_path
    assert_equal "#{Folder::TRASH_DIR}", child2_recipient.dir_id
    assert_equal "#{child2_recipient_id}", child3_recipient.dir_id
    assert(child2.name != child2_recipient.name)

    f1_recipient = CozyFile.find inst_recipient, f1_recipient_id
    assert_equal true, f1_recipient.trashed
  end
end
