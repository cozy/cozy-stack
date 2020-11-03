require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "Export and import" do
  it "can be used to move data from a Cozy to another" do
    Helpers.scenario "export_import"
    Helpers.start_mailhog

    source = Instance.create name: "source"
    dest = Instance.create name: "dest"
    source.install_app "photos"

    # Create a file with an old version
    folder = Folder.create source
    folder.couch_id.wont_be_empty
    file = CozyFile.create source, dir_id: folder.couch_id
    file.overwrite source, mime: 'text/plain'

    # Create an album with some photos
    CozyFile.ensure_photos_in_cache
    folder = Folder.create source
    folder.couch_id.wont_be_empty
    album = Album.create source
    photos = CozyFile.create_photos source, dir_id: folder.couch_id
    photos.each { |p| album.add source, p }

    # Export the data from one Cozy and import them and the other
    sleep 1
    export = Export.new(source)
    export.run
    link = export.get_link
    import = Import.new(dest, link)
    import.precheck
    import.run
    import.wait_done

    dest.stack.reset_tokens

    # Check that everything has been moved
    moved = Album.find dest, album.couch_id
    assert_equal moved.name, album.name
    triggers = Trigger.all dest
    refute_nil(triggers.detect do |t|
      t.attributes.dig("message", "name") == "onPhotoUpload"
    end) # It's a service for the photos app

    dir = Folder.find dest, folder.couch_id
    %i[couch_id name dir_id path].each do |prop|
      assert_equal dir.send(prop), folder.send(prop)
    end
    photos.each do |p|
      photo = CozyFile.find dest, p.couch_id
      %i[couch_id name dir_id mime].each do |prop|
        assert_equal photo.send(prop), p.send(prop)
      end
    end
    f = CozyFile.find dest, file.couch_id
    assert_equal f.old_versions.length, 1

    # Check that the email from the destination was kept
    contacts = Contact.all dest
    me = contacts.detect(&:me)
    assert_equal me.emails[0]["address"], dest.email
    settings = Settings.instance dest
    assert_equal settings["email"], dest.email

    # It is the end
    assert_equal source.check, []
    assert_equal dest.check, []

    source.remove
    dest.remove
  end
end
