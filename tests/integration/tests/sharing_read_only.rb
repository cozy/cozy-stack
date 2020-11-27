require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A file or folder" do
  it "can be shared in read-only mode" do
    Helpers.scenario "read_only"
    Helpers.start_mailhog

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
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file1.name}"
    file1_charlie = CozyFile.find_by_path inst_charlie, path
    path = "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file2.name}"
    file2_charlie = CozyFile.find_by_path inst_charlie, path

    # Rename a file and downgrade Charlie to read-only
    name1b = "#{Faker::Hobbit.thorins_company}.txt"
    file1_charlie.rename inst_charlie, name1b
    code = sharing.read_only inst_bob, 2
    assert_equal 204, code
    sleep 12
    name2b = "#{Faker::DrWho.villian}.txt"
    file2_charlie.rename inst_charlie, name2b
    sleep 8

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
    name1c = "#{Faker::DrWho.specie}.txt"
    file1_charlie.rename inst_charlie, name1c
    sleep 12
    file1 = CozyFile.find inst_alice, file1.couch_id
    assert_equal name1c, file1.name

    # Create a note and share it
    note = Note.create inst_alice
    sharing = Sharing.new
    sharing.rules << Rule.sync(note.file)
    sharing.members << inst_alice << contact_bob
    inst_alice.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing

    # Check that the recipient can open the note
    sleep 12
    note_path = "/#{Helpers::SHARED_WITH_ME}/#{note.file.name}"
    note_bob = CozyFile.find_by_path inst_bob, note_path
    assert_equal note_bob.referenced_by.length, 1
    assert_equal note_bob.referenced_by[0]["type"], Sharing.doctype
    assert_equal note_bob.referenced_by[0]["id"], sharing.couch_id
    parameters = Note.open inst_bob, note_bob.couch_id
    assert_equal note.file.couch_id, parameters["note_id"]
    assert %w[flat nested].include? parameters["subdomain"]
    assert %w[http https].include? parameters["protocol"]
    assert_equal inst_alice.domain, parameters["instance"]
    refute_nil parameters["sharecode"]
    assert_equal bob, parameters["public_name"]

    assert_equal inst_alice.check, []
    assert_equal inst_bob.check, []
    assert_equal inst_charlie.check, []

    inst_alice.remove
    inst_bob.remove
    inst_charlie.remove
  end
end
