require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def file_has_album_reference(file, album_id)
  return false if file.referenced_by.nil?
  file.referenced_by.any? do |ref|
     ref["type"] == Album.doctype && ref["id"] == album_id
  end
end

describe "A photo" do
  Helpers.scenario "referenced_by"
  Helpers.start_mailhog

  recipient_name = "Bob"

  it "can be shared in a folder or an album" do
    # Create the instance
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create the folder with a photo
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    file_name = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(file_name, dir_id: folder.couch_id)
    file = CozyFile.create inst, opts

    # Create an album with this file
    album = Album.create inst
    album.add inst, file
    file = CozyFile.find inst, file.couch_id
    assert file_has_album_reference(file, album.couch_id)

    # Create the sharing
    contact = Contact.create inst, given_name: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 2

    # Check there is no reference on the recipient's file
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{folder.name}/#{file.name}"
    file_recipient = CozyFile.find_by_path inst_recipient, file_path
    assert_equal file.name, file_recipient.name
    assert_nil file_recipient.referenced_by

    # Create an album with the file by the recipient
    album_recipient = Album.create inst_recipient
    album_recipient.add inst_recipient, file_recipient
    file_recipient = CozyFile.find inst_recipient, file_recipient.couch_id
    assert file_has_album_reference(file_recipient, album_recipient.couch_id)

    # Make an update on the recipient's file and check on the sharer
    file_recipient.rename inst_recipient, "#{Faker::Internet.slug}.jpg"
    sleep 5
    file = CozyFile.find inst, file.couch_id
    assert_equal file_recipient.name, file.name
    refute file_has_album_reference(file, album_recipient.couch_id)

    # Remove the sharer's file
    file.remove inst
    sleep 5
    file = CozyFile.find inst, file.couch_id
    assert file.trashed

    # The recipient still has the file, but in a special folder
    file_recipient = CozyFile.find inst_recipient, file_recipient.couch_id
    refute file_recipient.trashed
    assert_equal Folder::NO_LONGER_SHARED_DIR, file_recipient.dir_id
    assert file_has_album_reference(file_recipient, album_recipient.couch_id)

    # Create a picture
    file_name = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture file_name
    file = CozyFile.create inst, opts

    # Create an album with the picture
    album = Album.create inst
    album.add inst, file
    file = CozyFile.find inst, file.couch_id
    assert file_has_album_reference(file, album.couch_id)

    # Create the sharing
    contact = Contact.create inst, given_name: recipient_name
    sharing = Sharing.new
    sharing.rules = Rule.create_from_album(album, "sync")
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing
    sleep 2

    # Check the recipient has the shared album and photo
    album_recipient = Album.find inst_recipient, album.couch_id
    assert_equal album.name, album_recipient.name
    file_path = CGI.escape "/#{Helpers::SHARED_WITH_ME}/#{album.name}/#{file.name}"
    file_recipient = CozyFile.find_by_path inst_recipient, file_path
    assert_equal file.name, file_recipient.name
    assert file_has_album_reference(file_recipient, album.couch_id)

    # Remove the photo from the recipient's album
    album_recipient.remove_photo inst_recipient, file_recipient
    sleep 5

    # The photo should still exist, but not in the sharer's album anymore
    file = CozyFile.find inst, file.couch_id
    refute file_has_album_reference(file, album.couch_id)
    refute file.trashed
  end
end
