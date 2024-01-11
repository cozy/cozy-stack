require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A sharing with several members" do
  it "is correctly synchronized" do
    Helpers.scenario "several_members"
    Helpers.start_mailhog

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_bob = Instance.create name: "Bob"
    inst_charlie = Instance.create name: "Charlie"
    inst_dave = Instance.create name: "Dave"
    inst_emily = Instance.create name: "Emily"

    # Create the hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    content_path = "../fixtures/wet-cozy_20160910__M4Dz.jpg"
    opts = CozyFile.options_from_fixture(content_path, dir_id: folder.couch_id)
    f1 = CozyFile.create inst, opts

    # Create the sharing
    contact_bob = Contact.create inst, given_name: "Bob"
    contact_charlie = Contact.create inst, given_name: "Charlie"
    contact_dave = Contact.create inst, given_name: "Dave"
    contact_emily = Contact.create inst, given_name: "Emily"
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact_bob << contact_charlie << contact_dave << contact_emily
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    inst_charlie.accept sharing
    inst_dave.accept sharing
    inst_emily.accept sharing
    sleep 12

    dir_path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
    dir_bob = Folder.find_by_path inst_bob, dir_path
    opts = CozyFile.options_from_fixture(content_path, dir_id: dir_bob.couch_id)
    f2_bob = CozyFile.create inst_bob, opts
    sleep 17

    f1_path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{f1.name}"
    f1_bob = CozyFile.find_by_path inst_bob, f1_path
    f1_bob.overwrite inst_bob

    dir_charlie = Folder.find_by_path inst_charlie, dir_path
    opts = CozyFile.options_from_fixture(content_path, dir_id: dir_charlie.couch_id, name: "conflict.txt")
    CozyFile.create inst_charlie, opts
    dir_dave = Folder.find_by_path inst_dave, dir_path
    opts = CozyFile.options_from_fixture(content_path, dir_id: dir_dave.couch_id, name: "conflict.txt")
    CozyFile.create inst_dave, opts
    sleep 6

    f2_bob.remove inst_bob
    sleep 21

    # Check that the files are the same on disk
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_bob.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    dc = File.join Helpers.current_dir, inst_charlie.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    dd = File.join Helpers.current_dir, inst_dave.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    de = File.join Helpers.current_dir, inst_emily.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty
    Helpers.fsdiff(da, dd).must_be_empty
    Helpers.fsdiff(da, de).must_be_empty

    assert_equal inst.check, []
    assert_equal inst_bob.check, []
    assert_equal inst_charlie.check, []
    assert_equal inst_dave.check, []
    assert_equal inst_emily.check, []

    inst.remove
    inst_bob.remove
    inst_charlie.remove
    inst_dave.remove
    inst_emily.remove
  end
end
