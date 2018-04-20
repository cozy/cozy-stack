class Bootstrap
  attr_reader :sharing, :owner, :recipients, :objects

  def initialize(owner, recipients, objects)
    @owner = owner
    @recipients = recipients
    @objects = objects
    @sharing = Sharing.new
    objects.each do |o|
      @sharing.rules << Rule.push(o)
    end
    @sharing.members << owner
    recipients.each do |r|
      contact = Contact.create owner, given_name: r.name
      @sharing.members << contact
    end
    owner.register @sharing
  end

  def open
    @owner.open @objects.first
  end

  def accept(recipient = nil)
    recipient ||= @recipients.first
    recipient.accept @sharing
  end

  def self.push_folder
    owner = Instance.create name: "Alice"
    object = Folder.create owner
    recipient = Instance.create name: "Bob"
    [owner, recipient].map { |i| i.install_app "drive" }
    Bootstrap.new owner, [recipient], [object]
  end
end
