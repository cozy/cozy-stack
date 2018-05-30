require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A folder" do
  Helpers.scenario "sync_folder"
  Helpers.start_mailhog

  it "can be shared to a recipient in sync mode" do
    recipient_name = "Bob"

    # Create the instances
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create the folders
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, dir_id: folder.couch_id
    file = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file, dir_id: folder.couch_id)
    file = CozyFile.create inst, opts

    # Create the sharing
    contact = Contact.create inst, givenName: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing

    sleep 7
    # Check the folders are the same
    child1_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_recipient = Folder.find_by_path inst_recipient, child1_path
    child1_id_recipient = child1_recipient.couch_id
    folder_id_recipient = child1_recipient.dir_id
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    file_recipient = Folder.find_by_path inst_recipient, file_path
    file_id_recipient = file_recipient.couch_id
    assert_equal child1.name, child1_recipient.name
    assert_equal file.name, file_recipient.name

    # Check the sync (create + update) sharer -> recipient
    child1.rename inst, Faker::Internet.slug
    child2 = Folder.create inst, dir_id: folder.couch_id
    child1.move_to inst, child2.couch_id
    file.overwrite inst, mime: 'text/plain'
    file.rename inst, "#{Faker::Internet.slug}.txt"
    sleep 7

    child1_recipient = Folder.find inst_recipient, child1_id_recipient
    child2_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_recipient = Folder.find_by_path inst_recipient, child2_path
    file = CozyFile.find inst, file.couch_id
    file_recipient = CozyFile.find inst_recipient, file_id_recipient
    assert_equal child1.name, child1_recipient.name
    assert_equal child2.name, child2_recipient.name
    assert_equal child1_recipient.dir_id, child2_recipient.couch_id
    assert_equal file.name, file_recipient.name
    assert_equal file.md5sum, file_recipient.md5sum

    # Check the sync (create + update) recipient -> sharer
    child1_recipient.rename inst_recipient, Faker::Internet.slug
    child3_recipient = Folder.create inst_recipient, dir_id: folder_id_recipient
    child1_recipient.move_to inst_recipient, child3_recipient.couch_id
    file_recipient.rename inst_recipient, "#{Faker::Internet.slug}.txt"
    file_recipient.overwrite inst_recipient, content: "New content from recipient"

    sleep 7
    child1 = Folder.find inst, child1.couch_id
    child3_path = CGI.escape "/#{folder.name}/#{child3_recipient.name}"
    child3 = Folder.find_by_path inst, child3_path
    file = CozyFile.find inst, file.couch_id
    assert_equal child1_recipient.name, child1.name
    assert_equal child3_recipient.name, child3.name
    assert_equal child1.dir_id, child3.couch_id
    assert_equal file_recipient.name, file.name
    assert_equal file_recipient.md5sum, file.md5sum

    # Check that the files are the same on disk
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    diff = Helpers.fsdiff da, db
    diff.must_be_empty
  end
end
