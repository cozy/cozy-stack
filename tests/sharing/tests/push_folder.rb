#!/usr/bin/env ruby

require_relative '../boot'
require 'minitest/autorun'

describe "When an instance has a folder" do
  before do
    @inst = Instance.create name: 'alice'
    folder = @inst.create_doc Folder.new
    contact = @inst.create_doc Contact.new
    @sharing = Sharing.new
    @sharing.rules << Rule.push(folder)
    @sharing.members << @inst << contact
  end

  it "the folder can be shared (push) to a contact" do
    @inst.register_sharing @sharing
  end
end
