#!/usr/bin/env ruby

require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest'

describe "A folder" do
  Helpers.scenario "sync_folder"
  Helpers.start_mailhog

  it "can be shared to a recipient in sync mode" do
    recipient_name = "Bob"

    # Create the instances
    inst = Instance.create name: "Alice"
    inst_recipient = Instance.create name: recipient_name

    # Create the folders
    folder = Folder.create inst
    folder.couch_id.wont_be_empty
    child1 = Folder.create inst, {dir_id: folder.couch_id}

    # Create the sharing
    contact = Contact.create inst, givenName: recipient_name
    sharing = Sharing.new
    sharing.rules << Rule.sync(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    inst_recipient.accept sharing

    # Check the folders are the same
    sharing_info = Sharing.get_sharing_info inst_recipient, sharing.couch_id, folder.doctype
    child1_id_recipient = sharing_info["relationships"]["shared_docs"]["data"][0]["id"]
    folder_id_recipient = sharing_info["attributes"]["rules"][0]["values"][0]
    f = Folder.find inst_recipient, child1_id_recipient
    assert_equal f.name, child1.name

    # Check the update sync sharer -> recipient
    new_name = Faker::Internet.slug
    child1.rename inst, new_name
    sleep 7
    child1_recipient = Folder.find inst_recipient, child1_id_recipient
    assert_equal child1_recipient.name, new_name

    # Check the update sync recipient -> sharer
    new_name = Faker::Internet.slug
    child1_recipient.rename inst_recipient, new_name
    sleep 7
    child1 = Folder.find inst, child1.couch_id
    assert_equal child1.name, new_name
  end

end
