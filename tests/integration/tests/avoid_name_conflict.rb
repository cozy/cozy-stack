require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A file or folder" do
  it "can be shared in read-only mode" do
    Helpers.scenario "read_only"
    Helpers.start_mailhog

    # Create the instances
    inst_alice = Instance.create name: "Alice"
    inst_bob = Instance.create name: "Bob"
    contact_bob = Contact.create inst_alice, given_name: "Bob"

    # Create the folder
    folder = Folder.create inst_alice
    folder.couch_id.wont_be_empty

    # Create the sharing
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst_alice << contact_bob
    inst_alice.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    sleep 1

    # Create two directories and check they are synchronized
    one = Folder.create inst_alice, name: "foo", dir_id: folder.couch_id
    two = Folder.create inst_alice, name: "bar", dir_id: folder.couch_id
    sleep 12
    one_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{one.name}"
    one_bob = Folder.find_by_path inst_bob, one_path
    two_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{two.name}"
    two_bob = Folder.find_by_path inst_bob, two_path
    assert_equal one.name, one_bob.name
    assert_equal two.name, two_bob.name

    # Rename the directories and check that we have no conflict
    two.rename inst_alice, "baz"
    one.rename inst_alice, "bar"
    sleep 12
    one_bob = Folder.find inst_bob, one_bob.couch_id
    two_bob = Folder.find inst_bob, two_bob.couch_id
    assert_equal "bar", one_bob.name
    assert_equal "baz", two_bob.name

    assert_equal inst_alice.fsck, ""
    assert_equal inst_bob.fsck, ""

    inst_alice.remove
    inst_bob.remove
  end
end
