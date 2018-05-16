#!/usr/bin/env ruby

require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def assert_not_found(db, id)
  assert_raises RestClient::NotFound do
    Helpers.couch.get_doc db, id
  end
end

describe "A sharing" do
  Helpers.scenario "revoke_sharing"
  Helpers.start_mailhog

  it "can be revoked by the sharer" do
    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create the folder
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    file = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file, dir_id: folder.couch_id)
    file = CozyFile.create inst, opts

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

    # Get the clients id and triggers id
    db = Helpers.db_name inst.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    client_id = doc.dig "credentials", 0, "local_client_id"
    track_id = doc.dig "triggers", "track_id"
    replicate_id = doc.dig "triggers", "replicate_id"
    upload_id = doc.dig "triggers", "upload_id"
    refute_empty(client_id)
    refute_empty(track_id)
    refute_empty(replicate_id)
    refute_empty(upload_id)

    db = Helpers.db_name inst_recipient.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    client_id_recipient = doc.dig "credentials", 0, "local_client_id"
    track_id_recipient = doc.dig "triggers", "track_id"
    replicate_id_recipient = doc.dig "triggers", "replicate_id"
    upload_id_recipient = doc.dig "triggers", "upload_id"
    refute_empty(client_id_recipient)
    refute_empty(track_id_recipient)
    refute_empty(replicate_id_recipient)
    refute_empty(upload_id_recipient)

    # Revoke the sharing
    code = sharing.revoke_by_sharer(inst, Folder.doctype)
    assert_equal 204, code

    # Check the sharing on the sharer
    db = Helpers.db_name inst.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    assert_nil doc["active"]
    assert_empty doc["triggers"]
    assert_equal("revoked", doc.dig("members", 1, "status"))
    assert_empty doc.dig("credentials", 0)
    shared_docs = Sharing.get_shared_docs(inst, sharing.couch_id, Folder.doctype)
    assert_nil shared_docs

    # Check the oauth client and triggers are deleted
    db_oauth = Helpers.db_name inst.domain, "io.cozy.oauth"
    db_triggers = Helpers.db_name inst.domain, "io.cozy.triggers"
    assert_not_found db_oauth, client_id
    assert_not_found db_triggers, track_id
    assert_not_found db_triggers, replicate_id
    assert_not_found db_triggers, upload_id

    # Check the sharing on the recipient
    db = Helpers.db_name(inst_recipient.domain, Sharing.doctype)
    doc = Helpers.couch.get_doc db, sharing.couch_id
    assert_nil doc["active"]
    assert_empty doc["triggers"]
    assert_nil doc.dig("credentials")
    shared_docs = Sharing.get_shared_docs(inst_recipient, sharing.couch_id, Folder.doctype)
    assert_nil shared_docs

    # Check the oauth client and triggers are deleted
    db_oauth = Helpers.db_name(inst_recipient.domain, "io.cozy.oauth")
    db_triggers = Helpers.db_name(inst_recipient.domain, "io.cozy.triggers")
    assert_not_found db_oauth, client_id_recipient
    assert_not_found db_triggers, track_id_recipient
    assert_not_found db_triggers, replicate_id_recipient
    assert_not_found db_triggers, upload_id_recipient

    # Make an update: it should not be propagated
    old_name = file.name
    file.rename inst, Faker::Internet.slug
    sleep 3
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{old_name}"
    file_recipient = Folder.find_by_path inst_recipient, file_path
    assert(file_recipient.name != file.name)
  end
end
