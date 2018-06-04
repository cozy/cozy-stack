require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A sharing" do
  Helpers.scenario "conflicts"
  Helpers.start_mailhog

  it "can handle conflicts" do
    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, dir_id: folder.couch_id
    child2 = Folder.create inst, dir_id: folder.couch_id
    file_name = "#{Faker::Internet.slug}.txt"
    file = CozyFile.create inst, name: file_name, dir_id: folder.couch_id

    # Create the sharing
    contact = Contact.create inst, givenName: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 2

    # Generate conflicts with reconciliation
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    file_recipient = CozyFile.find_by_path inst_recipient, path
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_recipient = Folder.find_by_path inst_recipient, path
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_recipient = CozyFile.find_by_path inst_recipient, path

    file.move_to inst, child1.couch_id
    file_recipient.move_to inst_recipient, child2_recipient.couch_id
    file.move_to inst, folder.couch_id
    file_recipient.move_to inst_recipient, child1_recipient.couch_id

    2.times do
      child1.rename inst, Faker::Internet.slug
      child1_recipient.rename inst_recipient, Faker::Internet.slug
    end

    sleep 15
    # Check the files and diretories are even
    file = CozyFile.find inst, file.couch_id
    parent_file = Folder.find inst, file.dir_id
    file_recipient = CozyFile.find inst_recipient, file_recipient.couch_id
    parent_file_recipient = Folder.find inst_recipient, file_recipient.dir_id
    child1 = Folder.find inst, child1.couch_id
    child1_recipient = Folder.find inst_recipient, child1_recipient.couch_id
    assert_equal parent_file.name, parent_file_recipient.name
    assert_equal file.couch_rev, file_recipient.couch_rev
    assert_equal child1.name, child1_recipient.name
    assert_equal child1.couch_rev, child1_recipient.couch_rev

    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

    # Generate conflicts with no reconciliation
    4.times do
      file.overwrite inst
      file_recipient.overwrite inst_recipient
    end

    sleep 15
    # Check the conflicted files
    children = Folder.children inst, parent_file.path
    conflict_file = children.find { |c| c.name.include? "conflict" }
    refute_nil conflict_file
    path = CGI.escape "#{parent_file_recipient.path}/#{conflict_file.name}"
    conflict_file_recipient = CozyFile.find_by_path inst_recipient, path
    assert_equal conflict_file_recipient.name, conflict_file.name
    assert_equal conflict_file_recipient.couch_rev, conflict_file.couch_rev
    assert_equal conflict_file_recipient.md5sum, conflict_file.md5sum

    diff = Helpers.fsdiff da, db
    diff.must_be_empty
  end
end
