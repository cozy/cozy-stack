require_relative '../boot'
require 'minitest/autorun'
require 'faye/websocket'
require 'eventmachine'
require 'pry-rescue/minitest' unless ENV['CI']

def assert_same_thumbs(base_path_a, id_a, base_path_b, id_b)
  thumbs_a = Dir["#{base_path_a}/*"]
  thumbs_a.each do |thumb_a|
    suffix = thumb_a.split(id_a).last
    thumb_b = File.join base_path_b, "#{id_b}#{suffix}"
    assert_empty Helpers.fsdiff(thumb_a, thumb_b)
  end
end

describe "A folder" do
  it "can be shared to a recipient in push mode" do
    Helpers.scenario "push_folder"
    Helpers.start_mailhog

    recipient_name = "Bob"

    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name
    cozy = [{ url: inst_recipient.url, primary: true }]
    contact = Contact.create inst, given_name: recipient_name, emails: [], cozy: cozy

    # Create the folder with a photo, and checks that the photo has its
    # thumbnails generated
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    file = nil
    formats = []
    EM.run do
      ws = Faye::WebSocket::Client.new("ws://#{inst.domain}/realtime/")

      ws.on :open do
        ws.send({ method: "AUTH", payload: inst.token_for("io.cozy.files") }.to_json)
        ws.send({ method: "SUBSCRIBE", payload: { type: "io.cozy.files.thumbnails" } }.to_json)
      end

      ws.on :message do |event|
        msg = JSON.parse(event.data)
        formats << msg.dig("payload", "doc", "format")
        ws.close if formats.size == 3
      end

      ws.on :close do
        EM.stop
      end

      EM::Timer.new(30) do
        EM.stop
      end

      EM::Timer.new(1) do
        file_path = "../fixtures/wet-cozy_20160910__M4Dz.jpg"
        opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
        file = CozyFile.create inst, opts
      end
    end
    assert_equal formats, %w[large medium small]

    # Add a note in the folder
    note = Note.create inst, dir_id: folder.couch_id

    # Create the sharing
    sharing = Sharing.new
    sharing.rules << Rule.push(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing

    # Check the recipient's folder is the same as the sender's
    sleep 7
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
    folder_recipient = Folder.find_by_path inst_recipient, path
    assert_equal folder_recipient.name, folder.name

    # Check that the files are the same on disk
    unless ENV['COZY_SWIFTTEST']
      da = File.join Helpers.current_dir, inst.domain, folder.name
      db = File.join Helpers.current_dir, inst_recipient.domain,
                     Helpers::SHARED_WITH_ME, sharing.rules.first.title
      diff = Helpers.fsdiff da, db
      diff.must_be_empty
    end

    # Check the metadata are the same for the photo
    file = CozyFile.find inst, file.couch_id
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    file_recipient = CozyFile.find_by_path inst_recipient, file_path
    assert_equal file.md5sum, file_recipient.md5sum
    assert_equal file.mime, file_recipient.mime
    assert_equal file.class, file_recipient.class
    assert_equal file.size, file_recipient.size
    assert_equal file.executable, file_recipient.executable
    refute_nil file.metadata
    refute_nil file_recipient.metadata
    assert_equal file.metadata, file_recipient.metadata

    short_id_a = file.couch_id.slice(0..3)
    short_id_b = file_recipient.couch_id.slice(0..3)
    base_path_a = File.join Helpers.current_dir, inst.domain, ".thumbs", short_id_a
    base_path_b = File.join Helpers.current_dir, inst_recipient.domain, ".thumbs", short_id_b
    assert_same_thumbs base_path_a, file.couch_id, base_path_b, file_recipient.couch_id

    # Check that the recipient can open the note
    note_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{note.file.name}"
    note_recipient = CozyFile.find_by_path inst_recipient, note_path
    parameters = Note.open inst_recipient, note_recipient.couch_id
    assert_equal note.file.couch_id, parameters["note_id"]
    assert %w[flat nested].include? parameters["subdomain"]
    assert_equal inst.domain, parameters["instance"]
    refute_nil parameters["sharecode"]
    assert_equal recipient_name, parameters["public_name"]

    # Create a "one-shot" sharing
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    file_path = "../fixtures/wet-cozy_20160910__M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file_path, dir_id: folder.couch_id)
    CozyFile.create inst, opts
    oneshot = Sharing.new
    oneshot.rules << Rule.none(folder)
    oneshot.members << inst << contact
    inst.register oneshot

    # Accept the oneshot
    sleep 1
    inst_recipient.accept oneshot

    # Check the recipient's folder is the same as the sender's
    sleep 7
    path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}"
    folder_recipient = Folder.find_by_path inst_recipient, path
    assert_equal folder_recipient.name, folder.name

    # Check that the files are the same on disk
    unless ENV['COZY_SWIFTTEST']
      da = File.join Helpers.current_dir, inst.domain, folder.name
      db = File.join Helpers.current_dir, inst_recipient.domain,
                     Helpers::SHARED_WITH_ME, oneshot.rules.first.title
      diff = Helpers.fsdiff da, db
      diff.must_be_empty
    end

    assert_equal inst.fsck, ""
    assert_equal inst_recipient.fsck, ""

    inst.remove
    inst_recipient.remove
  end
end
