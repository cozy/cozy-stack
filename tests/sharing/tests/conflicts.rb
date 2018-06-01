require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A sharing" do
  Helpers.scenario "conflicts"
  Helpers.start_mailhog

  it "can handle conflicts on the folder" do
    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create hierarchy
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, dir_id: folder.couch_id
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
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_recipient = Folder.find_by_path inst_recipient, path
    assert_equal child1_recipient.name, child1.name

    2.times do
      child1.rename inst, Faker::Internet.slug
      child1_recipient.rename inst_recipient, Faker::Internet.slug
    end

    sleep 12
    # Check the names are equal in db and disk
    child1 = Folder.find inst, child1.couch_id
    child1_recipient = Folder.find inst_recipient, child1_recipient.couch_id
    assert_equal child1.name, child1_recipient.name
    assert_equal child1.couch_rev, child1_recipient.couch_rev

    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, folder.name
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

    # Generate conflicts with no reconciliation
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    file_recipient = CozyFile.find_by_path inst_recipient, path
    assert_equal file_recipient.name, file.name

    2.times do
      file.overwrite inst
      file_recipient.overwrite inst_recipient
    end

    sleep 12
    # TODO assert

  end
end
