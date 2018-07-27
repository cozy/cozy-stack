require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A sharing" do
  Helpers.scenario "revoke_sharing"
  Helpers.start_mailhog

  it "can be revoked" do
    bob = "Bob"
    charlie = "Charlie"

    # Create the instances
    inst_alice = Instance.create name: "Alice"
    inst_bob = Instance.create name: bob
    inst_charlie = Instance.create name: charlie

    # Create the contacts
    contact_bob = Contact.create inst_alice, given_name: bob
    contact_charlie = Contact.create inst_alice, given_name: charlie

    # Create the folder
    folder = Folder.create inst_alice
    folder.couch_id.wont_be_empty
    filename1 = "#{Faker::Internet.slug}.txt"
    filename2 = "#{Faker::Internet.slug}.txt"
    file1 = CozyFile.create inst_alice, name: filename1, dir_id: folder.couch_id
    file2 = CozyFile.create inst_alice, name: filename2, dir_id: folder.couch_id

    # Create the sharing
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst_alice << contact_bob << contact_charlie
    inst_alice.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    sleep 1
    inst_charlie.accept sharing
    sleep 1
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file1.name}"
    file1_charlie = CozyFile.find_by_path inst_charlie, path
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file2.name}"
    file2_charlie = CozyFile.find_by_path inst_charlie, path

    # Rename a file and downgrade Charlie to read-only
    name1b = "#{Faker::Hobbit.thorins_company}.txt"
    file1_charlie.rename inst_charlie, name1b
    code = sharing.read_only inst_bob, 2
    assert_equal 204, code
    sleep 7
    name2b = "#{Faker::HitchhikersGuideToTheGalaxy.marvin_quote}.txt"
    file2_charlie.rename inst_charlie, name2b
    sleep 7

    # Check that the replicate and upload trigger have been removed
    doc = Helpers.couch.get_doc inst_charlie.domain, Sharing.doctype, sharing.couch_id
    replicate_id = doc.dig "triggers", "replicate_id"
    assert_nil replicate_id
    upload_id = doc.dig "triggers", "upload_id"
    assert_nil upload_id

    # File1 should have been sync to Alice, but not file2
    file1 = CozyFile.find inst_alice, file1.couch_id
    assert_equal name1b, file1.name
    file2 = CozyFile.find inst_alice, file2.couch_id
    refute_equal name2b, file2.name

    # Upgrade Charlie to read-write
    code = sharing.read_write inst_bob, 2
    assert_equal 204, code
    sleep 1
    name1c = "#{Faker::DrWho.quote}.txt"
    file1_charlie.rename inst_charlie, name1c
    sleep 6
    file1 = CozyFile.find inst_alice, file1.couch_id
    assert_equal name1c, file1.name
  end
end
