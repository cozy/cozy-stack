#!/usr/bin/env ruby

require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def make_xor_key
  random = Random.new.bytes(8)
  res = []
  random.each_byte do |c|
    res << (c & 0xf)
    res << (c >> 4)
  end
  res
end

def xor_id(id, key)
  l = key.length
  buf = id.bytes.to_a
  buf.each_with_index do |c, i|
    if 48 <= c && c <= 57
      c = (c - 48) ^ key[i%l].ord
    elsif 97 <= c && c <= 102
      c = (c -  87) ^ key[i%l].ord
    elsif 65 <= c && c <= 70
      c = (c - 55) ^ key[i%l].ord
    else
      next
    end
    if c < 10
      buf[i] = c + 48
    else
      buf[i] = (c - 10) + 97
    end
  end
  buf.pack('c*')
end

describe "A sharing" do
  Helpers.scenario "integrity"
  Helpers.start_mailhog

  it "cannot reveal information on existing files" do
    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create the folder
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, dir_id: folder.couch_id
    file = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file, dir_id: folder.couch_id)
    CozyFile.create inst, opts

    # Create the sharing
    contact = Contact.create inst, givenName: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Manually set the xor_key
    db = Helpers.db_name inst.domain, Sharing.doctype
    doc = Helpers.couch.get_doc db, sharing.couch_id
    key = make_xor_key
    doc["credentials"][0]["xor_key"] = key
    Helpers.couch.update_doc db, doc

    # Create a folder on the recipient side, with a fixed id being the
    # xor_id of the child1 folder
    doc = {
      type: "directory",
      name: name,
      dir_id: Folder::ROOT_DIR,
      path: "/#{Faker::Internet.slug}",
      created_at: "2018-05-11T12:18:37.558389182+02:00",
      updated_at: "2018-05-11T12:18:37.558389182+02:00"
    }
    db = Helpers.db_name inst_recipient.domain, Folder.doctype
    id = xor_id(child1.couch_id, key)
    Helpers.couch.create_named_doc db, id, doc

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 2

    # Make an update
    child1.rename inst, Faker::Internet.slug
    sleep 4

    # The child1 folder shouldn't be part of the sharing as its id exists
    # on the recipient side
    child1_recipient = Folder.find inst_recipient, id
    assert(child1.name != child1_recipient.name)
    path = File.join Helpers.current_dir, inst_recipient.domain,
                     Helpers::SHARED_WITH_ME, sharing.rules.first.title,
                     child1_recipient.name
    assert !Helpers.file_exists_in_fs(path)
  end
end
