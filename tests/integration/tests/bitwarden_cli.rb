require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "The bitwarden API of the stack" do
  it "is usable with the official bitwarden CLI" do
    Helpers.scenario "bitwarden_cli"
    Helpers.start_mailhog

    inst = Instance.create name: "Alice"

    # Create two bitwarden clients
    bw = Bitwarden.new inst
    bw.login
    assert_equal bw.sync, "Syncing complete."

    bw2 = Bitwarden.new inst
    bw2.login

    assert_empty bw.items
    # bw CLI has by default a "No Folder" folder
    folders = bw.folders
    assert_equal folders.length, 1
    assert_equal folders[0][:name], "No Folder"
    assert_nil folders[0][:id]

    # The stack has automatically created a cozy organization...
    orgs = bw.organizations
    assert_equal orgs.length, 1
    assert_equal orgs[0][:name], "Cozy"
    refute_nil orgs[0][:id]
    org_id = orgs[0][:id]

    # ...with a connectors collection
    colls = bw.collections
    assert_equal colls.length, 1
    assert_equal colls[0][:name], "Cozy Connectors"
    assert_equal colls[0][:organizationId], org_id
    refute_nil colls[0][:id]
    coll_id = colls[0][:id]

    name = Faker::Internet.slug
    bw.create_folder name
    folders = bw.folders
    assert_equal folders.length, 2
    assert_equal folders[0][:name], name
    refute_nil folders[0][:id]
    folder_id = folders[0][:id]
    assert_equal folders[1][:name], "No Folder"
    assert_nil folders[1][:id]

    card = {
      type: Bitwarden::Types::CARD,
      favorite: false,
      name: "My card",
      notes: "for leisure only",
      card: bw.template('item.card')
    }
    bw.create_item card

    note = {
      type: Bitwarden::Types::SECURENOTE,
      folderId: folder_id,
      favorite: false,
      name: "My note",
      notes: Faker::DrWho.quote,
      secureNote: bw.template('item.securenote')
    }
    bw.create_item note

    identity = {
      type: Bitwarden::Types::IDENTITY,
      favorite: true,
      name: "My identity",
      notes: nil,
      identity: bw.template('item.identity')
    }
    bw.create_item identity

    bw2.sync
    items = bw2.items
    assert_equal items.length, 3
    [card, note, identity].each do |expected|
      item = items.find { |i| i[:type] == expected[:type] }
      refute_nil item.delete(:id)
      refute_nil item.delete(:revisionDate)
      assert_nil item.delete(:folderId) unless expected.key? :folderId
      assert_nil item.delete(:organizationId)
      assert_equal item.delete(:object), "item"
      assert_equal item, expected
    end

    assert_equal bw.sync, "Syncing complete."
    assert_equal bw.logout, "You have logged out."
    assert_equal bw2.logout, "You have logged out."
  end
end
