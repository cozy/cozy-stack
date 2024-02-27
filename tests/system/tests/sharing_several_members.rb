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
    inst_fred = Instance.create name: "Fred"

    # Create the hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    sub = Folder.create inst, dir_id: folder.couch_id
    content_path = "../fixtures/wet-cozy_20160910__M4Dz.jpg"
    opts = CozyFile.options_from_fixture(content_path, dir_id: sub.couch_id)
    fsub = CozyFile.create inst, opts
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
    sleep 6

    # Check that we can add a group from the owner
    g1 = Group.create inst, name: Faker::Kpop.girl_groups
    contact_gaby = Contact.create inst, given_name: "Gaby", groups: [g1.couch_id]
    sleep 1
    sharing.add_group inst, g1
    sleep 3
    info = Sharing.get_sharing_info inst, sharing.couch_id, Folder.doctype
    members = [contact_bob, contact_charlie, contact_dave, contact_emily, contact_gaby]
    revoked = []
    check_sharing_has_groups_and_members info, [g1], members, revoked

    # Check that we can add a group from a recipient
    g2 = Group.create inst_bob, name: Faker::Kpop.boy_bands
    contact_hugo = Contact.create inst_bob, given_name: "Hugo", groups: [g2.couch_id]
    sleep 1
    sharing.add_group inst_bob, g2
    sleep 3
    info = Sharing.get_sharing_info inst, sharing.couch_id, Folder.doctype
    members = [contact_bob, contact_charlie, contact_dave, contact_emily, contact_gaby, contact_hugo]
    revoked = []
    check_sharing_has_groups_and_members info, [g1, g2], members, revoked

    # Check that we can remove a member of a group
    contact_hugo.delete inst_bob
    sleep 4
    info = Sharing.get_sharing_info inst, sharing.couch_id, Folder.doctype
    members = [contact_bob, contact_charlie, contact_dave, contact_emily, contact_gaby, contact_hugo]
    revoked = [6]
    check_sharing_has_groups_and_members info, [g1, g2], members, revoked

    # Check that we can remove a group
    sharing.remove_group inst, 0
    sleep 4
    info = Sharing.get_sharing_info inst, sharing.couch_id, Folder.doctype
    members = [contact_bob, contact_charlie, contact_dave, contact_emily, contact_gaby, contact_hugo]
    revoked = [5]
    assert info.dig("attributes", "groups", 0, "removed")
    check_sharing_has_groups_and_members info, [g1, g2], members, revoked

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

    # Create another sharing
    folder = Folder.create inst, name: "Second"
    folder.couch_id.wont_be_empty
    contact_fred = Contact.create inst, given_name: "Fred"
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact_bob << contact_charlie << contact_fred
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    inst_charlie.accept sharing
    inst_fred.accept sharing
    sleep 12

    # Move the sub folder from one sharing to another
    sub.move_to inst, Folder::ROOT_DIR
    sleep 1
    sub.rename inst, "sub2"
    sleep 1
    fsub.rename inst, "fsub2"
    sleep 1
    opts = CozyFile.options_from_fixture(content_path, dir_id: sub.couch_id)
    other = CozyFile.create inst, opts
    other.rename inst, "other"
    sleep 1
    sub.move_to inst, folder.couch_id
    sleep 12

    sub.rename inst, "sub3"
    sleep 12

    # Check that the files are the same on disk
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_bob.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    dc = File.join Helpers.current_dir, inst_charlie.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    df = File.join Helpers.current_dir, inst_fred.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty
    Helpers.fsdiff(da, df).must_be_empty

    assert_equal inst.check, []
    assert_equal inst_bob.check, []
    assert_equal inst_charlie.check, []
    assert_equal inst_dave.check, []
    assert_equal inst_emily.check, []
    assert_equal inst_fred.check, []

    inst.remove
    inst_bob.remove
    inst_charlie.remove
    inst_dave.remove
    inst_emily.remove
    inst_fred.remove
  end
end

def check_sharing_has_groups_and_members(info, groups, contacts, revoked)
  grps = info.dig("attributes", "groups") || []
  assert_equal grps.length, groups.length
  groups.each_with_index do |g, i|
    assert_equal grps[i]["name"], g.name
  end

  members = info.dig "attributes", "members"
  # We have the owner in members but not in contacts
  assert_equal members.length, contacts.length + 1
  contacts.each_with_index do |contact, i|
    assert_equal members[i+1]["name"], contact.fullname
  end

  revoked.each do |i|
    assert_equal members[i]["status"], "revoked"
  end
end
