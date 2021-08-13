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

    # Creates
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

    login = {
      type: Bitwarden::Types::LOGIN,
      favorite: true,
      name: "My login",
      notes: nil,
      login: {
        username: Faker::Internet.email,
        password: Faker::Internet.password,
        passwordRevisionDate: nil,
        totp: Faker::Internet.password,
        uris: [
          { uri: Faker::Internet.url, match: nil }
        ]
      },
      fields: [
        { type: 0, name: Faker::Internet.slug, value: Faker::Internet.password },
        { type: 1, name: Faker::Internet.slug, value: Faker::Internet.password }
      ]
    }
    bw.create_item login

    bw2.sync
    assert_equal bw.fingerprint, bw2.fingerprint
    items = bw2.items
    assert_equal items.length, 4
    [card, note, identity, login].each do |expected|
      item = items.find { |i| i[:type] == expected[:type] }
      refute_nil item.delete(:id)
      refute_nil item.delete(:revisionDate)
      assert_nil item.delete(:folderId) unless expected.key? :folderId
      assert_nil item.delete(:organizationId)
      assert_nil item.delete(:collectionIds)
      assert_equal item.delete(:object), "item"
      assert_equal item, expected
    end

    # Updates
    name = Faker::DrWho.catch_phrase
    bw.edit_folder folder_id, name

    items = bw.items
    item = items.find { |i| i[:type] == login[:type] }
    login[:login][:uris].push(uri: Faker::Internet.url, match: 3)
    login[:login][:password] = Faker::Internet.password
    bw.edit_item item[:id], login

    note = items.find { |i| i[:type] == note[:type] }
    bw.delete_item note[:id]

    item = items.find { |i| i[:type] == identity[:type] }
    bw.share item[:id], org_id, coll_id

    bw2.sync
    folders = bw2.folders
    assert_equal folders.length, 2
    assert_equal folders[0][:name], name
    assert_equal folders[0][:id], folder_id
    assert_equal folders[1][:name], "No Folder"
    assert_nil folders[1][:id]

    items = bw2.items
    assert_equal items.length, 3
    item = items.find { |i| i[:type] == Bitwarden::Types::IDENTITY }
    assert_equal item.delete(:organizationId), org_id
    assert_equal item.delete(:collectionIds), [coll_id]
    [card, identity, login].each do |expected|
      item = items.find { |i| i[:type] == expected[:type] }
      item[:login][:passwordRevisionDate] = nil if item[:type] == Bitwarden::Types::LOGIN
      refute_nil item.delete(:id)
      refute_nil item.delete(:revisionDate)
      assert_nil item.delete(:folderId) unless expected.key? :folderId
      assert_nil item.delete(:organizationId)
      assert_nil item.delete(:collectionIds)
      assert_equal item.delete(:object), "item"
      assert_equal item, expected
    end

    # Create an organization and a collection
    org = Bitwarden::Organization.create inst, "Family"
    assert_equal bw.sync, "Syncing complete."
    bw.sync
    orgs = bw.organizations
    assert_equal orgs.length, 2
    names = orgs.map { |o| o[:name] }.sort
    assert_equal names, %w[Cozy Family]
    colls = bw.collections
    assert_equal colls.length, 2
    names = colls.map { |c| c[:name] }.sort
    assert_equal names, ["Cozy Connectors", "Family"]

    coll_id = colls.find { |c| c[:organizationId] == org.id }[:id]
    shared_item = {
      type: Bitwarden::Types::CARD,
      favorite: false,
      name: "Family card",
      notes: "for serious stuff",
      card: bw.template('item.card'),
      organizationId: org.id,
      collectionIds: [coll_id]
    }
    bw.create_item shared_item, org.id

    # Create a sharing
    inst_recipient = Instance.create name: "Bob"
    contact = Contact.create inst, given_name: "Bob"
    sharing = Sharing.new
    sharing.rules = Rule.create_from_organization(org.id, "sync")
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing

    # Check the users inside the organization
    users = Bitwarden::User.list inst, org.id
    alice_user = users.find { |u| u.status == 2 } # Confirmed
    assert_equal alice_user.type, 0 # Owner
    assert_equal alice_user.name, "Alice"
    assert_equal alice_user.email, inst.email
    refute_empty alice_user.id
    bob_user = users.find { |u| u.status == 1 } # Accepted
    assert_equal bob_user.type, 2 # User
    assert_equal bob_user.name, contact.fullname
    assert_equal bob_user.email, contact.primary_email
    refute_empty bob_user.id

    # Confirm the sharing
    public_key = bob_user.fetch_public_key(inst)
    encrypted_key = Bitwarden::Organization.encrypt_key public_key, org.key
    bob_user.confirm inst, org.id, encrypted_key
    sleep 6

    # Check that Bob can access the shared credentials
    bw3 = Bitwarden.new inst_recipient
    bw3.login
    assert_equal bw3.sync, "Syncing complete."
    orgs = bw3.organizations
    assert_equal orgs.length, 2
    names = orgs.map { |o| o[:name] }.sort
    assert_equal names, %w[Cozy Family]
    colls = bw3.collections
    assert_equal colls.length, 2
    names = colls.map { |c| c[:name] }.sort
    assert_equal names, ["Cozy Connectors", "Family"]
    items = bw3.items
    assert_equal items.length, 1
    assert_equal items.first[:name], "Family card"
    item_id = items.first[:id]

    # Update an item
    shared_item[:name] = "Updated card"
    bw.edit_item item_id, shared_item

    # Check that the item is updated on Bob's instance
    sleep 6
    bw3.sync
    items = bw3.items
    assert_equal items.length, 1
    assert_equal items.first[:name], "Updated card"

    inst.remove
  end
end
