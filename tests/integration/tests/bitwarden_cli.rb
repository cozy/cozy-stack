require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "The bitwarden API of the stack" do
  it "is usable with the official bitwarden CLI" do
    Helpers.scenario "bitwarden_cli"
    Helpers.start_mailhog

    inst = Instance.create name: "Alice"

    bw = Bitwarden.new inst
    bw.login
    assert_equal bw.sync, "Syncing complete."

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

    # ...with a connectors collection
    colls = bw.collections
    assert_equal colls.length, 1
    # TODO why the name is missing?
    ap colls
    # assert_equal colls[0][:name], "Cozy Connectors"

    # %w[item item.field item.login item.login.uri item.card item.identity item.securenote collection item-collections].each do |object|
    #   ap object
    #   ap bw.template object
    #   ap '---'
    # end

    name = Faker::Internet.slug
    bw.create_folder name
    folders = bw.folders
    assert_equal folders.length, 2
    assert_equal folders[0][:name], name
    refute_nil folders[0][:id]
    assert_equal folders[1][:name], "No Folder"
    assert_nil folders[1][:id]

    assert_equal bw.sync, "Syncing complete."
    assert_equal bw.logout, "You have logged out."
  end
end
