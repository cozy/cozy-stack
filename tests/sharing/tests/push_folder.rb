#!/usr/bin/env ruby

require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest'

describe "A folder" do
  Helpers.scenario "push_folder"
  Helpers.start_mailhog

  it "can be shared to a recipient in push mode" do
    # Create the folder
    inst = Instance.create name: "Alice"
    folder = inst.create_doc Folder.new
    folder.couch_id.wont_be_empty

    # Create the sharing
    name = "Bob"
    contact = inst.create_doc Contact.new givenName: name
    sharing = Sharing.new
    sharing.rules << Rule.push(folder)
    sharing.members << inst << contact
    inst.register sharing

    # Accept the sharing
    sleep 1
    recipient = Instance.create name: name
    recipient.accept sharing
  end
end
