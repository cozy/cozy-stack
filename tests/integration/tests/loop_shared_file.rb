require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A sharing" do
  it "is resilient to a lot of changes, like a loop from cozy-desktop" do
    Helpers.scenario "loop_shared_file"
    Helpers.start_mailhog

    # Create the instances
    inst_alice = Instance.create name: "Alice"
    inst_bob = Instance.create name: "Bob"
    contact_bob = Contact.create inst_alice, given_name: "Bob"

    # Create the folder
    folder = Folder.create inst_alice
    folder.couch_id.wont_be_empty
    filename = "#{Faker::Internet.slug}.txt"
    file = CozyFile.create inst_alice, name: filename, dir_id: folder.couch_id

    # Create the sharing
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst_alice << contact_bob
    inst_alice.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    sleep 1
    file = CozyFile.find inst_alice, file.couch_id
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    file_bob = CozyFile.find_by_path inst_bob, path
    assert_equal file.md5sum, file_bob.md5sum

    # Simulate a bug of cozy-desktop where the file is renamed in a loop
    250.times do |i|
      name = "#{i}-#{filename}"
      file.rename inst_alice, name
    end
    file.overwrite inst_alice, content: Faker::BackToTheFuture.quote

    # Check that the changes are applied on Bob's instance
    sleep 30
    file = CozyFile.find inst_alice, file.couch_id
    file_bob = CozyFile.find inst_bob, file_bob.couch_id
    assert_equal file.name, file_bob.name
    assert_equal file.md5sum, file_bob.md5sum

    inst_alice.remove
    inst_bob.remove
  end
end
