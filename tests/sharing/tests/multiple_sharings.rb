#!/usr/bin/env ruby

require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest'

describe "A folder" do
  Helpers.scenario "multiple_sharings"
  Helpers.start_mailhog

  it "can be shared to several recipients" do
    bob = "Bob"
    charlie = "Charlie"

    # Create the instances
    inst_alice = Instance.create name: "Alice"
    inst_bob = Instance.create name: bob
    inst_charlie = Instance.create name: charlie

    # Create the contacts
    contact_bob = Contact.create inst_alice, givenName: bob
    contact_charlie = Contact.create inst_alice, givenName: charlie

    # Share a folder with bob and charlie, in the same sharing
    folder = Folder.create inst_alice
    folder.couch_id.wont_be_empty
    file_path = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
    file = CozyFile.create inst_alice, opts

    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst_alice << contact_bob
    sharing.members << contact_charlie
    inst_alice.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    inst_charlie.accept sharing

    sleep 5

    # Check the folders are the same
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
    folder_bob = Folder.find_by_path inst_bob, path
    assert_equal folder_bob.name, folder.name
    folder_charlie = Folder.find_by_path inst_charlie, path
    assert_equal folder_charlie.name, folder.name

    #  Share a folder with Bob and Charlie, in 2 different sharings
    folder = Folder.create inst_alice
    child1 = Folder.create inst_alice, {dir_id: folder.couch_id}
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst_alice << contact_bob
    inst_alice.register sharing
    sleep 1
    inst_bob.accept sharing
    sharing.members = []
    sharing.members << inst_alice << contact_bob
    inst_alice.register sharing
    sleep 1
    inst_charlie.accept sharing

    sleep 7

    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_bob = Folder.find_by_path inst_bob, path
    assert_equal child1_bob.name, child1.name
    child1_charlie = Folder.find_by_path inst_charlie, path
    assert_equal child1_charlie.name, child1.name

    # Propagate a change (rename dir + add file) from Alice's side
    new_name = Faker::Internet.slug
    child1.rename inst_alice, new_name
    opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
    file = CozyFile.create inst_alice, opts

    sleep 12
    child1_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{new_name}"
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    puts "file path : #{file_path}"
    child1_bob = Folder.find_by_path inst_bob, child1_path
    file_bob = CozyFile.find_by_path inst_bob, file_path
    puts "file bob : #{file_bob.name}"

    file_charlie = CozyFile.find_by_path inst_charlie, file_path
    child1_charlie = Folder.find_by_path inst_charlie, child1_path
    assert_equal child1_bob.name, new_name
    assert_equal child1_charlie.name, new_name
    assert_equal file_bob.name, file.name
    assert_equal file_charlie.name, file.name

    # Propagate a change (rename file) from Bob's side
    new_name = Faker::Internet.slug
    child1_bob.rename inst_bob, new_name
    sleep 10
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{new_name}"
    child1_alice = Folder.find inst_alice, child1.couch_id
    assert_equal child1_alice.name, new_name
    child1_charlie = Folder.find_by_path inst_charlie, path
    assert_equal child1_charlie.name, new_name

    # Check that the files are the same on disk
    da = File.join Helpers.current_dir, inst_alice.domain, folder.name
    db = File.join Helpers.current_dir, inst_bob.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    dc = File.join Helpers.current_dir, inst_charlie.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    diff = Helpers.fsdiff da, db
    diff.must_be_empty
    diff = Helpers.fsdiff da, dc
    diff.must_be_empty

  end

end
