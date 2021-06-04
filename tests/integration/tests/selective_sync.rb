require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "An OAuth client" do
  it "can synchronize only some directories" do
    Helpers.scenario "selective_sync"
    Helpers.start_mailhog

    # Create the instance
    inst = Instance.create

    # Create hierarchy
    folder1 = Folder.create inst
    child1 = Folder.create inst, dir_id: folder1.couch_id
    file1 = CozyFile.create inst, dir_id: child1.couch_id
    child2 = Folder.create inst, dir_id: folder1.couch_id
    file2 = CozyFile.create inst, dir_id: child2.couch_id
    child3 = Folder.create inst, dir_id: folder1.couch_id
    folder2 = Folder.create inst
    folder3 = Folder.create inst
    folder4 = Folder.create inst

    changes = CozyFile.changes(inst)
    seq = changes["last_seq"]
    [folder1, child1, child2, child3, folder2, folder3, folder4].each do |dir|
      result = changes["results"].detect { |r| r["id"] == dir.couch_id }
      result.wont_be_empty
      assert_equal result["doc"]["type"], "directory"
    end
    [file1, file2].each do |f|
      result = changes["results"].detect { |r| r["id"] == f.couch_id }
      result.wont_be_empty
      assert_equal result["doc"]["type"], "file"
    end

    # Mark two directories to not synchronize
    client_id = inst.stack.oauth_client_id
    folder1.not_synchronized_on inst, client_id
    folder2.not_synchronized_on inst, client_id

    changes = CozyFile.changes(inst, seq)
    seq = changes["last_seq"]
    assert_equal changes["results"].length, 2
    assert_equal changes.dig("results", 0, "id"), folder1.couch_id
    assert changes.dig("results", 0, "deleted")
    assert_equal changes.dig("results", 0, "doc").keys, %w[_deleted _id _rev]
    assert_equal changes.dig("results", 0, "doc", "_id"), folder1.couch_id
    assert_equal changes.dig("results", 0, "doc", "_deleted"), true
    assert_equal changes.dig("results", 1, "id"), folder2.couch_id
    assert changes.dig("results", 1, "deleted")
    assert_equal changes.dig("results", 1, "doc").keys, %w[_deleted _id _rev]

    # Make some operations
    child1.rename inst, Faker::Internet.slug
    file1.rename inst, Faker::Internet.slug
    folder2.rename inst, Faker::Internet.slug
    changes = CozyFile.changes(inst, seq)
    seq = changes["last_seq"]
    assert_equal changes["results"].length, 3
    assert_equal changes.dig("results", 0, "id"), child1.couch_id
    assert changes.dig("results", 0, "deleted")
    assert_equal changes.dig("results", 1, "id"), file1.couch_id
    assert changes.dig("results", 1, "deleted")
    assert_equal changes.dig("results", 2, "id"), folder2.couch_id
    assert changes.dig("results", 2, "deleted")

    child1.move_to inst, folder4.couch_id
    child2.move_to inst, folder2.couch_id
    child3.move_to inst, folder3.couch_id
    changes = CozyFile.changes(inst, seq)
    seq = changes["last_seq"]
    assert_equal changes["results"].length, 3
    assert_equal changes.dig("results", 0, "id"), child1.couch_id
    refute changes.dig("results", 0, "deleted")
    assert_equal changes.dig("results", 1, "id"), child2.couch_id
    assert changes.dig("results", 1, "deleted")
    assert_equal changes.dig("results", 2, "id"), child3.couch_id
    refute changes.dig("results", 2, "deleted")

    folder4.remove inst
    Folder.clear_trash inst
    changes = CozyFile.changes(inst, seq)
    seq = changes["last_seq"]
    assert_equal changes["results"].length, 3 # folder4, child1, and child4
    assert changes.dig("results", 0, "deleted")
    assert changes.dig("results", 1, "deleted")
    assert changes.dig("results", 2, "deleted")

    # Synchronize again folder2
    folder2.synchronized_on inst, client_id
    changes = CozyFile.changes(inst, seq)
    assert_equal changes["results"].length, 1
    assert_equal changes.dig("results", 0, "id"), folder2.couch_id
    refute changes.dig("results", 0, "deleted")

    # Done
    assert_equal inst.check, []
    inst.remove
  end
end
