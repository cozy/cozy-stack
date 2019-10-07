require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "The VFS" do
  it "is able to deal with special cases on filepaths (encoding, case sensitivity)" do
    Helpers.scenario "filepath_operations"
    Helpers.start_mailhog

    # Create the instance
    inst = Instance.create name: "Alice"

    # Create a folder
    dirname = "this"
    folder = Folder.create inst, name: dirname
    folder.couch_id.wont_be_empty
    sub = Folder.create inst, dir_id: folder.couch_id
    3.times do
      CozyFile.create inst, dir_id: sub.couch_id
      CozyFile.create inst, dir_id: folder.couch_id
    end
    # Create a lot of folders to force pagination
    500.times do |i|
      Folder.create inst, name: "foo-#{i}", dir_id: sub.couch_id
    end

    # Create a folder with the same name, but not the same case
    other = Folder.create inst, name: dirname.upcase
    other.couch_id.wont_be_empty
    4.times do
      CozyFile.create inst, dir_id: other.couch_id
    end
    other.remove inst
    assert_equal inst.fsck, ""

    # Trying stupids tricks
    before = File.join Helpers.current_dir, inst.domain
    # Renaming to the same name
    folder.rename inst, "that" rescue nil
    # Moving inside its-self
    folder.move_to inst, folder.couch_id rescue nil
    # Moving inside a sub-directory
    folder.move_to inst, sub.couch_id rescue nil
    after = File.join Helpers.current_dir, inst.domain
    diff = Helpers.fsdiff before, after
    diff.must_be_empty
    assert_equal inst.fsck, ""

    # Play with NFC and NFD unicode normalization
    ["Pièces pour ampèremètre", "Diplômes", "Reçus"].each do |name|
      nfc = name.unicode_normalize(:nfc)
      nfd = name.unicode_normalize(:nfd)
      folder.rename inst, nfc
      assert_equal inst.fsck, ""
      folder.rename inst, nfd
      assert_equal inst.fsck, ""
      folder.rename inst, nfc
      assert_equal inst.fsck, ""
    end

    folder.remove inst
    Folder.clear_trash inst
    sleep 5
    assert_equal inst.fsck, ""

    inst.remove
  end
end
