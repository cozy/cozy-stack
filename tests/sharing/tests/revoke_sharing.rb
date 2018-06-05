require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def assert_not_found(db, id)
  assert_raises RestClient::NotFound do
    Helpers.couch.get_doc db, id
  end
end

def assert_oauth_client_not_empty(client_id)
  refute_empty client_id
end

def assert_triggers_not_empty(triggers_ids)
  triggers_ids.each { |id| refute_empty id }
end

def assert_no_triggers(inst, triggers_ids)
  db_triggers = Helpers.db_name inst.domain, "io.cozy.triggers"
  triggers_ids.each { |id| assert_not_found db_triggers, id }
end

def assert_no_oauth_client(inst, client_id)
  db_oauth = Helpers.db_name inst.domain, "io.cozy.oauth"
  assert_not_found db_oauth, client_id
end

def triggers_ids(sharing)
  track_id = sharing.dig "triggers", "track_id"
  replicate_id = sharing.dig "triggers", "replicate_id"
  upload_id = sharing.dig "triggers", "upload_id"
  [track_id, replicate_id, upload_id]
end

def inbound_client_id(sharing, index)
  sharing.dig "credentials", index, "inbound_client_id"
end

def assert_sharing_revoked(inst, sharing_id, is_sharer)
  db = Helpers.db_name inst.domain, Sharing.doctype
  doc = Helpers.couch.get_doc db, sharing_id
  assert_nil doc["active"]
  assert_empty doc["triggers"]
  if is_sharer
    doc["credentials"].each { |cred| assert_empty cred }
    doc["members"].each_with_index do |m, i|
      assert_equal "revoked", m["status"] if i != 0
    end
  else
    assert_nil doc["credentials"]
  end
  shared_docs = Sharing.get_shared_docs(inst, sharing_id, Folder.doctype)
  assert_nil shared_docs
end

def assert_recipient_revoked(inst, sharing_id, index)
  db = Helpers.db_name inst.domain, Sharing.doctype
  doc = Helpers.couch.get_doc db, sharing_id
  assert doc["active"]
  refute_empty doc["triggers"]
  assert_empty doc.dig("credentials", index - 1)
  assert_equal("revoked", doc.dig("members", index, "status"))
  shared_docs = Sharing.get_shared_docs(inst, sharing_id, Folder.doctype)
  refute_empty shared_docs
end

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
    contact_bob = Contact.create inst_alice, givenName: bob
    contact_charlie = Contact.create inst_alice, givenName: charlie

    # Create the folder
    folder = Folder.create inst_alice
    folder.couch_id.wont_be_empty
    file = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file, dir_id: folder.couch_id)
    file = CozyFile.create inst_alice, opts

    # Create the sharing
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst_alice << contact_bob
    inst_alice.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    sleep 2

    # Get the clients id and triggers id
    db = Helpers.db_name inst_alice.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    client_id = inbound_client_id doc, 0
    tri_ids = triggers_ids doc
    assert_oauth_client_not_empty client_id
    assert_triggers_not_empty tri_ids

    db = Helpers.db_name inst_bob.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    client_id_recipient = inbound_client_id doc, 0
    tri_ids_recipient = triggers_ids doc
    assert_oauth_client_not_empty client_id_recipient
    assert_triggers_not_empty tri_ids_recipient

    # Revoke the sharing
    code = sharing.revoke_by_sharer(inst_alice, Folder.doctype)
    assert_equal 204, code

    # Check the sharing on the sharer
    assert_sharing_revoked inst_alice, sharing.couch_id, true
    assert_no_triggers inst_alice, tri_ids
    assert_no_oauth_client inst_alice, client_id

    # Check the sharing on the recipient
    assert_sharing_revoked inst_bob, sharing.couch_id, false
    assert_no_oauth_client inst_bob, client_id_recipient
    assert_no_triggers inst_bob, tri_ids_recipient

    # Make an update: it should not be propagated
    old_name = file.name
    file.rename inst_alice, Faker::Internet.slug
    sleep 3
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{old_name}"
    file_recipient = Folder.find_by_path inst_bob, file_path
    refute_equal file_recipient.name, file.name

    # Create a new sharing with a new folder
    folder = Folder.create inst_alice
    file = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file, dir_id: folder.couch_id)
    file = CozyFile.create inst_alice, opts
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst_alice << contact_bob
    inst_alice.register sharing

    # Accept the sharing
    sleep 1
    inst_bob.accept sharing
    sleep 2

    # Add Charlie to the sharing
    code = sharing.add_members inst_alice, [contact_charlie], Folder.doctype
    assert_equal 200, code

    # Accept the sharing
    sleep 1
    inst_charlie.accept sharing
    sleep 2

    # Get the clients id and triggers id on alice side
    db = Helpers.db_name inst_alice.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    inb_bob_client_id = inbound_client_id doc, 0
    inb_charlie_client_id = inbound_client_id doc, 1
    tri_ids = triggers_ids doc
    assert_oauth_client_not_empty inb_bob_client_id
    assert_oauth_client_not_empty inb_charlie_client_id
    assert_triggers_not_empty tri_ids

    # Get the clients id and triggers id on bob side
    db = Helpers.db_name inst_bob.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    client_id_bob = inbound_client_id doc, 0
    tri_ids_bob = triggers_ids doc
    assert_oauth_client_not_empty client_id_bob
    assert_triggers_not_empty tri_ids_bob

    # Get the clients id and triggers id on charlie side
    db = Helpers.db_name inst_charlie.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    client_id_charlie = inbound_client_id doc, 0
    tri_ids_charlie = triggers_ids doc
    assert_oauth_client_not_empty client_id_charlie
    assert_triggers_not_empty tri_ids_charlie

    # Revoke charlie by alice
    code = sharing.revoke_recipient_by_sharer inst_alice, Folder.doctype, 2
    assert_equal 204, code

    # Check the sharing on alice
    assert_recipient_revoked inst_alice, sharing.couch_id, 2
    assert_oauth_client_not_empty inb_bob_client_id
    assert_no_oauth_client inst_alice, inb_charlie_client_id
    assert_triggers_not_empty tri_ids

    # Check the sharing on charlie
    assert_sharing_revoked inst_charlie, sharing.couch_id, false
    assert_no_triggers inst_charlie, tri_ids_charlie
    assert_no_oauth_client inst_charlie, client_id_charlie

    # Revoke bob by himself
    code = sharing.revoke_recipient_by_itself inst_bob, Folder.doctype
    assert_equal 204, code

    # Check the sharing on alice
    assert_sharing_revoked inst_alice, sharing.couch_id, true
    assert_no_triggers inst_alice, tri_ids
    assert_no_oauth_client inst_alice, inb_bob_client_id
    assert_no_oauth_client inst_alice, inb_charlie_client_id

    # Check the sharing on bob
    assert_sharing_revoked inst_bob, sharing.couch_id, false
    assert_no_triggers inst_bob, tri_ids_bob
    assert_no_oauth_client inst_bob, client_id_bob

    # Make an update: it should not be propagated
    old_name = file.name
    file.rename inst_alice, Faker::Internet.slug
    sleep 3
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{old_name}"
    file_bob = Folder.find_by_path inst_bob, file_path
    file_charlie = Folder.find_by_path inst_charlie, file_path
    refute_equal file_bob.name, file.name
    refute_equal file_charlie.name, file.name
  end
end
