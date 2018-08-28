class Bootstrap
  attr_reader :sharing, :owner, :recipients, :objects

  def initialize(owner, recipients, rules)
    @owner = owner
    @recipients = recipients
    @objects = rules.map(&:object)
    @sharing = Sharing.new
    @sharing.rules = rules
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

  def self.sync_folder
    owner = Instance.create name: "Alice"
    object = Folder.create owner
    dir = Folder.create owner, dir_id: object.couch_id
    f = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(f, dir_id: object.couch_id)
    file = CozyFile.create owner, opts
    object.children << dir << file
    recipient = Instance.create name: "Bob"
    [owner, recipient].map { |i| i.install_app "home" }
    rule = Rule.sync object
    Bootstrap.new owner, [recipient], [rule]
  end

  def self.push_folder
    owner = Instance.create name: "Alice"
    object = Folder.create owner
    dir = Folder.create owner, dir_id: object.couch_id
    f = "../fixtures/wet-cozy_20160910__©M4Dz.jpg"
    opts = CozyFile.options_from_fixture(f, dir_id: object.couch_id)
    file = CozyFile.create owner, opts
    object.children << dir << file
    recipient = Instance.create name: "Bob"
    [owner, recipient].map { |i| i.install_app "home" }
    rule = Rule.push object
    Bootstrap.new owner, [recipient], [rule]
  end

  def self.photos_album
    owner = Instance.create name: "Alice"
    CozyFile.ensure_photos_in_cache
    album = Album.create owner
    dir = Folder.create owner
    photos = CozyFile.create_photos owner, dir_id: dir.couch_id
    photos.each { |p| album.add owner, p }
    recipient = Instance.create name: "Bob"
    [owner, recipient].map { |i| i.install_app "photos" }
    rules = []
    rules << Rule.new(doctype: album.doctype,
                      title: album.name,
                      values: [album.couch_id])
    rules << Rule.new(doctype: photos.first.doctype,
                      title: "photos",
                      selector: "referenced_by",
                      values: ["#{album.doctype}/#{album.couch_id}"],
                      add: :push,
                      update: :push,
                      remove: :push)
    Bootstrap.new owner, [recipient], rules
  end
end
