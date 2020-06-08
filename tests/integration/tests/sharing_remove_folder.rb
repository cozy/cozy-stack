require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A shared folder" do
  it "can be removed and end up in the trash" do
    Helpers.scenario "remove_folder"
    Helpers.start_mailhog

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
    file_path = "../fixtures/wet-cozy_20160910__M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file_path, dir_id: child1.couch_id)
    f1 = CozyFile.create inst, opts
    opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
    f2 = CozyFile.create inst, opts
    opts = CozyFile.options_from_fixture(file_path, dir_id: child2.couch_id)
    f3 = CozyFile.create inst, opts

    # Create the sharing
    contact = Contact.create inst, given_name: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 7

    # Get id for all dir/files to retrieve after delete
    child2_path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_recipient_id = Folder.get_id_from_path inst_recipient, child2_path

    child3_path = "#{child2_path}/#{child3.name}"
    child3_recipient_id = Folder.get_id_from_path inst_recipient, child3_path

    f1_path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}/#{f1.name}"
    f1_recipient_id = CozyFile.get_id_from_path inst_recipient, f1_path

    f2_path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{f2.name}"
    f2_recipient_id = CozyFile.get_id_from_path inst_recipient, f2_path

    f3_path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}/#{f3.name}"
    f3_recipient_id = CozyFile.get_id_from_path inst_recipient, f3_path

    # Remove a single file
    f2.remove inst

    # Move a dir out of the shared folder and update it
    child2.move_to inst, Folder::ROOT_DIR
    child2.rename inst, Faker::Internet.slug

    # Remove a directory containing a binary
    child1.remove inst

    sleep 12

    f2_recipient = CozyFile.find inst_recipient, f2_recipient_id
    assert_equal true, f2_recipient.trashed

    child2_recipient = Folder.find inst_recipient, child2_recipient_id
    child3_recipient = Folder.find inst_recipient, child3_recipient_id
    assert child2_recipient.trashed
    assert child3_recipient.trashed
    assert_equal "/#{Helpers::SHARED_WITH_ME}/#{folder.name}", child2_recipient.restore_path
    refute_equal child2.name, child2_recipient.name

    f1_recipient = CozyFile.find inst_recipient, f1_recipient_id
    assert f1_recipient.trashed

    # Check that when a folder is moved out of a sharing, the retroaction
    # doesn't trash the files inside it
    sleep 12
    f3_recipient = CozyFile.find inst_recipient, f3_recipient_id
    assert f3_recipient.trashed
    f3_sharer = CozyFile.find inst, f3.couch_id
    refute f3_sharer.trashed

    assert_equal inst.check, []
    assert_equal inst_recipient.check, []

    inst.remove
    inst_recipient.remove
  end
end
