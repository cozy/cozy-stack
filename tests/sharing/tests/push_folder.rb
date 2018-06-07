require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def assert_same_thumbs(base_path_a, id_a, base_path_b, id_b)
  thumbs_a = Dir["#{base_path_a}/*"]
  thumbs_a.each do |thumb_a|
    suffix = thumb_a.split(id_a).last
    thumb_b = File.join base_path_b, "#{id_b}#{suffix}"
    assert_empty Helpers.fsdiff(thumb_a, thumb_b)
  end
end

describe "A folder" do
  Helpers.scenario "push_folder"
  Helpers.start_mailhog

  it "can be shared to a recipient in push mode" do
    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name
    contact = Contact.create inst, given_name: recipient_name

    # Create the folder with a file
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    file_path = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
    file = CozyFile.create inst, opts

    # Create the sharing
    sharing = Sharing.new
    sharing.rules << Rule.push(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing

    # Check the recipient's folder is the same as the sender's
    sleep 7
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
    folder_recipient = Folder.find_by_path inst_recipient, path
    assert_equal folder_recipient.name, folder.name

    # Check that the files are the same on disk
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

    # Check the metadata are the same
    file = CozyFile.find inst, file.couch_id
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    file_recipient = CozyFile.find_by_path inst_recipient, file_path
    assert_equal file.md5sum, file_recipient.md5sum
    assert_equal file.mime, file_recipient.mime
    assert_equal file.class, file_recipient.class
    assert_equal file.size, file_recipient.size
    assert_equal file.executable, file_recipient.executable
    refute_nil file.metadata
    refute_nil file_recipient.metadata
    assert_equal file.metadata, file_recipient.metadata

    short_id_a = file.couch_id.slice(0..3)
    short_id_b = file_recipient.couch_id.slice(0..3)
    base_path_a = File.join Helpers.current_dir, inst.domain, ".thumbs", short_id_a
    base_path_b = File.join Helpers.current_dir, inst_recipient.domain, ".thumbs", short_id_b
    assert_same_thumbs base_path_a, file.couch_id, base_path_b, file_recipient.couch_id

    # Create a "one-shot" sharing
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    file_path = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
    CozyFile.create inst, opts
    oneshot = Sharing.new
    oneshot.rules << Rule.none(folder)
    oneshot.members << inst << contact
    inst.register oneshot

    # Accept the oneshot
    sleep 1
    inst_recipient.accept oneshot

    # Check the recipient's folder is the same as the sender's
    sleep 7
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
    folder_recipient = Folder.find_by_path inst_recipient, path
    assert_equal folder_recipient.name, folder.name

    # Check that the files are the same on disk
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, oneshot.rules.first.title
    diff = Helpers.fsdiff da, db
    diff.must_be_empty
  end
end
