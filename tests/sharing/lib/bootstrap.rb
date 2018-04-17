class Bootstrap
  attr_reader :sharing, :owner, :recipients, :object

  def initialize(owner, recipients, objects)
    @owner = owner
    @recipients = recipients
    @object = object
    @sharing = Sharing.new
    @sharing.rules << Rule.push(objects)
    @sharing.members << owner
    recipients.each do |r|
      contact = owner.create_doc Contact.new given_name: r.name
      @sharing.members << contact
    end
    owner.register @sharing
  end

  def open
    @owner.open @object
  end

  def accept(recipient = nil)
    recipient ||= @recipients.first
    recipient.accept @sharing
  end

  def self.push_folder
    owner = Instance.create name: "Alice"
    object = owner.create_doc Folder.new
    recipient = Instance.create name: "Bob"
    [owner, recipient].map { |i| i.install_app "drive" }
    Bootstrap.new owner, [recipient], [object]
  end
end
