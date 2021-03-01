require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "Moving a Cozy" do
  it "will move the data and the sharings" do
    Helpers.scenario "moving_a_cozy"
    Helpers.start_mailhog

    alices = Instance.create name: "alicesource", locale: "en"
    alicet = Instance.create name: "alicetarget", locale: "en"
    bobs = Instance.create name: "bobsource"
    bobt = Instance.create name: "bobtarget"
    charlie = Instance.create name: "charlie"

    # Create a few directories and files
    dira = Folder.create alices
    filename1 = "#{Faker::Internet.slug}.txt"
    CozyFile.create alices, name: filename1, dir_id: dira.couch_id
    CozyFile.create alices, dir_id: dira.couch_id
    CozyFile.create alices, dir_id: dira.couch_id
    dirt = Folder.create alicet, name: dira.name
    CozyFile.create alicet, name: filename1, dir_id: dirt.couch_id
    dirb = Folder.create bobs
    CozyFile.create bobs, dir_id: dirb.couch_id
    dirc = Folder.create charlie
    CozyFile.create charlie, dir_id: dirc.couch_id

    # And share them
    contacta1 = Contact.create alices, given_name: "Bob"
    contacta2 = Contact.create alices, given_name: "Charlie"
    sharinga = Sharing.new
    sharinga.rules << Rule.sync(dira)
    sharinga.members << alices << contacta1 << contacta2
    alices.register sharinga

    contactb1 = Contact.create bobs, given_name: "Alice"
    contactb2 = Contact.create bobs, given_name: "Charlie"
    sharingb = Sharing.new
    sharingb.rules << Rule.sync(dirb)
    sharingb.members << bobs << contactb1 << contactb2
    bobs.register sharingb

    contactc1 = Contact.create charlie, given_name: "Alice"
    contactc2 = Contact.create charlie, given_name: "Bob"
    sharingc = Sharing.new
    sharingc.rules << Rule.sync(dirc)
    sharingc.members << charlie << contactc1 << contactc2
    charlie.register sharingc

    # Accept the sharing
    sleep 1
    alices.accept sharingc
    alices.accept sharingb
    bobs.accept sharinga
    bobs.accept sharingc
    charlie.accept sharingb
    charlie.accept sharinga
    sleep 15

    # Check that the files have been synchronized
    da = File.join Helpers.current_dir, alices.domain, dira.name
    db = File.join Helpers.current_dir, bobs.domain, Helpers::SHARED_WITH_ME, dira.name
    dc = File.join Helpers.current_dir, charlie.domain, Helpers::SHARED_WITH_ME, dira.name
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty

    da = File.join Helpers.current_dir, alices.domain, Helpers::SHARED_WITH_ME, dirb.name
    db = File.join Helpers.current_dir, bobs.domain, dirb.name
    dc = File.join Helpers.current_dir, charlie.domain, Helpers::SHARED_WITH_ME, dirb.name
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty

    da = File.join Helpers.current_dir, alices.domain, Helpers::SHARED_WITH_ME, dirc.name
    db = File.join Helpers.current_dir, bobs.domain, Helpers::SHARED_WITH_ME, dirc.name
    dc = File.join Helpers.current_dir, charlie.domain, dirc.name
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty

    # Move both instances at the same time
    sleep 1
    movea = Move.new(alices, alicet)
    movea.get_initialize_token
    movea.get_target_token
    moveb = Move.new(bobs, bobt)
    moveb.get_source_token
    moveb.get_target_token
    movea.run
    moveb.run
    movea.confirm
    moveb.confirm
    movea.wait_done
    moveb.wait_done

    alicet.stack.reset_tokens
    bobt.stack.reset_tokens

    # Check that everything has been moved
    ds = File.join Helpers.current_dir, alices.domain, dira.name
    dt = File.join Helpers.current_dir, alicet.domain, dira.name
    Helpers.fsdiff(ds, dt).must_be_empty
    ds = File.join Helpers.current_dir, alices.domain, Helpers::SHARED_WITH_ME
    dt = File.join Helpers.current_dir, alicet.domain, Helpers::SHARED_WITH_ME
    Helpers.fsdiff(ds, dt).must_be_empty

    ds = File.join Helpers.current_dir, bobs.domain, Helpers::SHARED_WITH_ME
    dt = File.join Helpers.current_dir, bobt.domain, Helpers::SHARED_WITH_ME
    Helpers.fsdiff(ds, dt).must_be_empty
    ds = File.join Helpers.current_dir, bobs.domain, dirb.name
    dt = File.join Helpers.current_dir, bobt.domain, dirb.name
    Helpers.fsdiff(ds, dt).must_be_empty

    # Add some files to the sharings
    CozyFile.create alicet, filename: "from_alice_t", dir_id: dira.couch_id
    CozyFile.create bobt, filename: "from_bob_t", dir_id: dirb.couch_id
    CozyFile.create charlie, filename: "charlie", dir_id: dirc.couch_id
    sleep 15

    # Check that the sharings still work
    da = File.join Helpers.current_dir, alicet.domain, dira.name
    db = File.join Helpers.current_dir, bobt.domain, Helpers::SHARED_WITH_ME, dira.name
    dc = File.join Helpers.current_dir, charlie.domain, Helpers::SHARED_WITH_ME, dira.name
    Helpers.fsdiff(da, db).must_be_empty
    Helpers.fsdiff(da, dc).must_be_empty

    da = File.join Helpers.current_dir, alicet.domain, Helpers::SHARED_WITH_ME, dirb.name
    db = File.join Helpers.current_dir, bobt.domain, dirb.name
    dc = File.join Helpers.current_dir, charlie.domain, Helpers::SHARED_WITH_ME, dirb.name
    Helpers.fsdiff(db, da).must_be_empty
    Helpers.fsdiff(db, dc).must_be_empty

    da = File.join Helpers.current_dir, alicet.domain, Helpers::SHARED_WITH_ME, dirc.name
    db = File.join Helpers.current_dir, bobt.domain, Helpers::SHARED_WITH_ME, dirc.name
    dc = File.join Helpers.current_dir, charlie.domain, dirc.name
    Helpers.fsdiff(dc, da).must_be_empty
    Helpers.fsdiff(dc, db).must_be_empty

    # It is the end
    assert_equal alices.check, []
    assert_equal alicet.check, []
    assert_equal bobs.check, []
    assert_equal bobt.check, []
    assert_equal charlie.check, []

    alices.remove
    alicet.remove
    bobs.remove
    bobt.remove
    charlie.remove
  end
end
