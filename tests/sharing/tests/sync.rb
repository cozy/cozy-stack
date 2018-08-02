require_relative '../boot'
require 'minitest/autorun'
require 'faye/websocket'
require 'eventmachine'
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

describe "A folder" do
  Helpers.scenario "sync_folder"
  Helpers.start_mailhog

  it "can be shared to a recipient in sync mode" do
    recipient_name = "Bob"

    # Create the instances
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create the folders
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, dir_id: folder.couch_id
    file = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file, dir_id: folder.couch_id)
    file = CozyFile.create inst, opts
    zip  = "../fixtures/logos.zip"
    opts = CozyFile.options_from_fixture(zip, dir_id: child1.couch_id)
    CozyFile.create inst, opts

    # Create the sharing of a folder
    contact = Contact.create inst, given_name: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Manually set the xor_key
    doc = Helpers.couch.get_doc inst.domain, Sharing.doctype, sharing.couch_id
    key = make_xor_key
    doc["credentials"][0]["xor_key"] = key
    Helpers.couch.update_doc inst.domain, Sharing.doctype, doc

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    doc = Helpers.couch.get_doc inst_recipient.domain, Sharing.doctype, sharing.couch_id
    assert_equal 2, doc["initial_number_of_files_to_sync"]

    # Check the realtime events
    EM.run do
      ws = Faye::WebSocket::Client.new("ws://#{inst_recipient.domain}/realtime/")

      ws.on :open do
        ws.send({
          method: "AUTH",
          payload: inst_recipient.token_for("io.cozy.files")
        }.to_json)
        ws.send({
          method: "SUBSCRIBE",
          payload: { type: "io.cozy.sharings.initial-sync", id: sharing.couch_id }
        }.to_json)
      end

      ws.on :message do |event|
        msg = JSON.parse(event.data)
        if msg["event"] == "DELETED"
          ws.close
        else
          assert_equal 1, msg.dig("payload", "doc", "count")
        end
      end

      ws.on :close do
        EM.stop
      end
    end
    doc = Helpers.couch.get_doc inst_recipient.domain, Sharing.doctype, sharing.couch_id
    assert_nil doc["initial_number_of_files_to_sync"]

    # Check the folders are the same
    child1_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child1.name}"
    child1_recipient = Folder.find_by_path inst_recipient, child1_path
    child1_id_recipient = child1_recipient.couch_id
    folder_id_recipient = child1_recipient.dir_id
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    file_recipient = CozyFile.find_by_path inst_recipient, file_path
    file_id_recipient = file_recipient.couch_id
    assert_equal child1.name, child1_recipient.name
    assert_equal file.name, file_recipient.name

    # Check the sync (create + update) sharer -> recipient
    child1.rename inst, Faker::Internet.slug
    child2 = Folder.create inst, dir_id: folder.couch_id
    child1.move_to inst, child2.couch_id
    file.overwrite inst, mime: 'text/plain'
    file.rename inst, "#{Faker::Internet.slug}.txt"
    sleep 7

    child1_recipient = Folder.find inst_recipient, child1_id_recipient
    child2_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{child2.name}"
    child2_recipient = Folder.find_by_path inst_recipient, child2_path
    file = CozyFile.find inst, file.couch_id
    file_recipient = CozyFile.find inst_recipient, file_id_recipient
    assert_equal child1.name, child1_recipient.name
    assert_equal child2.name, child2_recipient.name
    assert_equal child1_recipient.dir_id, child2_recipient.couch_id
    assert_equal file.name, file_recipient.name
    assert_equal file.md5sum, file_recipient.md5sum
    assert_equal file.couch_rev, file_recipient.couch_rev

    # Check the sync (create + update) recipient -> sharer
    child1_recipient.rename inst_recipient, Faker::Internet.slug
    child3_recipient = Folder.create inst_recipient, dir_id: folder_id_recipient
    child1_recipient.move_to inst_recipient, child3_recipient.couch_id
    file_recipient.rename inst_recipient, "#{Faker::Internet.slug}.txt"
    file_recipient.overwrite inst_recipient, content: "New content from recipient"

    sleep 7
    child1 = Folder.find inst, child1.couch_id
    child3_path = CGI.escape "/#{folder.name}/#{child3_recipient.name}"
    child3 = Folder.find_by_path inst, child3_path
    file = CozyFile.find inst, file.couch_id
    assert_equal child1_recipient.name, child1.name
    assert_equal child3_recipient.name, child3.name
    assert_equal child1.dir_id, child3.couch_id
    assert_equal file_recipient.name, file.name
    assert_equal file_recipient.md5sum, file.md5sum
    assert_equal file_recipient.couch_rev, file.couch_rev

    # Check that the files are the same on disk
    da = File.join Helpers.current_dir, inst.domain, folder.name
    db = File.join Helpers.current_dir, inst_recipient.domain,
                   Helpers::SHARED_WITH_ME, sharing.rules.first.title
    diff = Helpers.fsdiff da, db
    diff.must_be_empty

    # Create a new folder
    child2 = Folder.create inst, dir_id: folder.couch_id

    # Create a folder on the recipient side, with a fixed id being the
    # xor_id of the child2 folder
    doc = {
      type: "directory",
      name: name,
      dir_id: Folder::ROOT_DIR,
      path: "/#{Faker::Internet.slug}",
      created_at: "2018-05-11T12:18:37.558389182+02:00",
      updated_at: "2018-05-11T12:18:37.558389182+02:00"
    }
    id = xor_id(child2.couch_id, key)
    Helpers.couch.create_named_doc inst_recipient.domain, Folder.doctype, id, doc

    # Make an update
    child2.rename inst, Faker::Internet.slug
    sleep 4

    # The child1 folder shouldn't be part of the sharing as its id exists
    # on the recipient side
    child2_recipient = Folder.find inst_recipient, id
    assert(child2.name != child2_recipient.name)
    path = File.join Helpers.current_dir, inst_recipient.domain,
                     Helpers::SHARED_WITH_ME, sharing.rules.first.title,
                     child2_recipient.name
    assert !Helpers.file_exists_in_fs(path)

    # Create the sharing of a file
    file = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file, dir_id: Folder::ROOT_DIR)
    file = CozyFile.create inst, opts
    sharing = Sharing.new
    sharing.rules << Rule.sync(file)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 7

    # Check the files are the same
    file = CozyFile.find inst, file.couch_id
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{file.name}"
    file_recipient = CozyFile.find_by_path inst_recipient, file_path
    file_id_recipient = file_recipient.couch_id
    assert_equal file.name, file_recipient.name
    assert_equal file.couch_rev, file_recipient.couch_rev

    # Check the sync sharer -> recipient
    file.overwrite inst, mime: 'text/plain'
    file.rename inst, "#{Faker::Internet.slug}.txt"
    sleep 7
    file = CozyFile.find inst, file.couch_id
    file_recipient = CozyFile.find inst_recipient, file_id_recipient
    assert_equal file.name, file_recipient.name
    assert_equal file.md5sum, file_recipient.md5sum
    assert_equal file.couch_rev, file_recipient.couch_rev

    # Check the sync recipient -> sharer
    file_recipient.rename inst_recipient, "#{Faker::Internet.slug}.txt"
    file_recipient.overwrite inst_recipient, content: "New content from recipient"
    sleep 7
    file = CozyFile.find inst, file.couch_id
    file_recipient = CozyFile.find inst_recipient, file_id_recipient
    assert_equal file.name, file_recipient.name
    assert_equal file.md5sum, file_recipient.md5sum
    assert_equal file.couch_rev, file_recipient.couch_rev
  end
end
