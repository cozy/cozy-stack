require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def assert_equal_metadata(meta_a, meta_b)
  meta_a.each do |key, value|
    assert_equal value, meta_b[key]
  end
end

def assert_diff_empty(file_a, file_b)
  diff = Helpers.fsdiff file_a, file_b
  assert_empty diff
end

def assert_same_thumbs(base_path_a, id_a, base_path_b, id_b)
  thumbs_a = Dir["#{base_path_a}/*"]
  thumbs_a.each do |thumb_a|
    suffix = thumb_a.split(id_a).last
    thumb_b = File.join base_path_b, "#{id_b}#{suffix}"
    assert_diff_empty thumb_a, thumb_b
  end
end

describe "A shared file" do
  Helpers.scenario "metadata"
  Helpers.start_mailhog

  it "has its metadata replicated" do
    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create the folder with a file
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    file_path = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
    file = CozyFile.create inst, opts

    # Create the sharing
    contact = Contact.create inst, givenName: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.push(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 2

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
    assert_equal_metadata file.metadata, file_recipient.metadata

    short_id_a = file.couch_id.slice(0..3)
    short_id_b = file_recipient.couch_id.slice(0..3)
    base_path_a = File.join Helpers.current_dir, inst.domain, ".thumbs", short_id_a
    base_path_b = File.join Helpers.current_dir, inst_recipient.domain, ".thumbs", short_id_b
    assert_same_thumbs base_path_a, file.couch_id, base_path_b, file_recipient.couch_id
  end
end
